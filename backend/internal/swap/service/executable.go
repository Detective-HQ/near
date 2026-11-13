package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// simulationAddress stands in for addresses on a simulation record that the
// user chose not to provide. It is never sent to any chain or provider.
const simulationAddress = "simulation-only"

// CreateSwapParams defines the parameters for creating a swap from a quote.
type CreateSwapParams struct {
	QuoteID        string `json:"quoteId"`
	IdempotencyKey string `json:"idempotencyKey"`
	SessionID      string `json:"sessionId,omitempty"`
	Recipient      string `json:"recipient,omitempty"`
	RefundTo       string `json:"refundTo,omitempty"`
}

// CreateSwapResult contains the created or replayed swap and its executable quote.
type CreateSwapResult struct {
	Swap            domain.Swap  `json:"swap"`
	Replayed        bool         `json:"replayed"`
	ExecutableQuote domain.Quote `json:"executableQuote"`
}

// CreateSwap turns a reviewed quote into a swap record.
//
// Idempotency is resolved first: a replay of the same key and payload returns
// the existing swap without calling the provider again (an executable quote
// allocates a fresh deposit address, so a blind retry would otherwise strand
// funds sent to the first address). A replay with a different payload returns
// IDEMPOTENCY_CONFLICT.
//
// Live swaps require ENABLE_LIVE_EXECUTION, a registry route on both chains,
// and use the recipient and refund address validated at quote time. The swap
// starts in AWAITING_USER_DEPOSIT. Simulation quotes create a simulated record
// that is never polled and can never become a real settlement.
func (s *Service) CreateSwap(ctx context.Context, params CreateSwapParams) (CreateSwapResult, error) {
	if err := ValidateIdempotencyKey(params.IdempotencyKey); err != nil {
		return CreateSwapResult{}, err
	}
	params.IdempotencyKey = strings.TrimSpace(params.IdempotencyKey)
	params.QuoteID = strings.TrimSpace(params.QuoteID)
	params.Recipient = strings.TrimSpace(params.Recipient)
	params.RefundTo = strings.TrimSpace(params.RefundTo)
	if params.QuoteID == "" {
		return CreateSwapResult{}, domain.New(domain.CodeValidationFailed, "Quote ID is required.")
	}

	payloadHash, err := CanonicalPayloadHash(params)
	if err != nil {
		return CreateSwapResult{}, err
	}

	if existing, lookupErr := s.store.GetSwapByIdempotencyKey(ctx, params.IdempotencyKey); lookupErr == nil {
		if !strings.EqualFold(existing.IdempotencyPayloadHash, payloadHash) {
			return CreateSwapResult{}, domain.New(domain.CodeIdempotencyConflict,
				"The idempotency key was already used with a different request payload.")
		}
		quote, _ := s.store.GetQuote(ctx, existing.QuoteID)
		return CreateSwapResult{Swap: existing, Replayed: true, ExecutableQuote: quote}, nil
	} else if !domain.IsCode(lookupErr, domain.CodeSwapNotFound) && !domain.IsCode(lookupErr, domain.CodeNotFound) {
		return CreateSwapResult{}, lookupErr
	}

	baseQuote, err := s.store.GetQuote(ctx, params.QuoteID)
	if err != nil {
		return CreateSwapResult{}, err
	}
	if baseQuote.SessionID != "" && !strings.EqualFold(baseQuote.SessionID, params.SessionID) {
		return CreateSwapResult{}, domain.New(domain.CodeForbidden, "You do not own this quote.")
	}

	now := s.now()
	if !baseQuote.ExpiresAt.IsZero() && !now.Before(baseQuote.ExpiresAt) {
		return CreateSwapResult{}, domain.New(domain.CodeQuoteExpired, "The quote has expired; request a fresh quote.")
	}

	switch baseQuote.Mode {
	case domain.QuoteModeSimulation:
		return s.createSimulatedSwap(ctx, baseQuote, params, payloadHash, now)
	case domain.QuoteModeProviderDry:
	default:
		return CreateSwapResult{}, domain.New(domain.CodeInvalidRequest,
			"Only a reviewed dry quote can be turned into a swap.")
	}

	if !s.enableLiveExecution {
		return CreateSwapResult{}, domain.New(domain.CodeLiveExecutionDisabled,
			"Live execution is not enabled on this server.")
	}

	originAsset, err := s.resolveAsset(ctx, baseQuote.OriginAssetID)
	if err != nil {
		return CreateSwapResult{}, err
	}
	destAsset, err := s.resolveAsset(ctx, baseQuote.DestinationAssetID)
	if err != nil {
		return CreateSwapResult{}, err
	}

	originChain, found := s.registry.Get(originAsset.Chain)
	if !found {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Origin chain %q is not registered.", originAsset.Chain)
	}
	if !originChain.Features.SwapEnabled || !originChain.Features.DepositSupported {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Swaps/deposits are not enabled for chain %q.", originChain.Key)
	}
	destChain, found := s.registry.Get(destAsset.Chain)
	if !found {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Destination chain %q is not registered.", destAsset.Chain)
	}
	if !destChain.Features.SwapEnabled {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Swaps are not enabled for destination chain %q.", destChain.Key)
	}

	// Never invent an address: funds sent to a guessed recipient are lost.
	recipient, refundTo, err := s.reviewedAddresses(baseQuote, params)
	if err != nil {
		return CreateSwapResult{}, err
	}
	normalizedRecipient, err := ValidateChainAddress(s.registry, destChain.Key, recipient)
	if err != nil {
		return CreateSwapResult{}, err
	}
	normalizedRefund, err := ValidateChainAddress(s.registry, originChain.Key, refundTo)
	if err != nil {
		return CreateSwapResult{}, domain.Wrap(domain.CodeInvalidRefund, "Invalid refund address.", err)
	}

	depositMode := "SIMPLE"
	if originChain.Features.MemoRequired {
		depositMode = "MEMO"
	}

	deadline := now.Add(s.quoteDeadline)
	quoteRes, err := s.provider.GetExecutableQuote(ctx, providers.ExecutableQuoteRequest{
		DryQuoteRequest: providers.DryQuoteRequest{
			OriginAsset:      originAsset,
			DestinationAsset: destAsset,
			AmountAtomic:     baseQuote.AmountInAtomic,
			Recipient:        normalizedRecipient,
			RefundTo:         normalizedRefund,
			SlippageBPS:      baseQuote.SlippageBPS,
		},
		Deadline:    deadline,
		DepositMode: depositMode,
	})
	s.observe(err)
	if err != nil {
		return CreateSwapResult{}, err
	}
	if strings.TrimSpace(quoteRes.Quote.DepositAddress) == "" {
		return CreateSwapResult{}, domain.New(domain.CodeProviderInvalidQuote,
			"The provider did not return deposit instructions.")
	}
	if originChain.Features.MemoRequired && strings.TrimSpace(quoteRes.Quote.DepositMemo) == "" {
		return CreateSwapResult{}, domain.New(domain.CodeProviderInvalidQuote,
			"The provider did not return the deposit memo this chain requires.")
	}

	execQuote := quoteRes.Quote
	execQuote.ID = "qte_exec_" + uuid.NewString()
	execQuote.SessionID = params.SessionID
	execQuote.Mode = domain.QuoteModeProviderExecutable
	execQuote.OriginAssetID = originAsset.ID
	execQuote.DestinationAssetID = destAsset.ID
	execQuote.AmountInAtomic = baseQuote.AmountInAtomic
	execQuote.AmountInDecimal = baseQuote.AmountInDecimal
	execQuote.SlippageBPS = baseQuote.SlippageBPS
	execQuote.Recipient = normalizedRecipient
	execQuote.RefundTo = normalizedRefund
	if execQuote.ProviderName == "" {
		execQuote.ProviderName = s.provider.Name()
	}
	if execQuote.ExpiresAt.IsZero() {
		execQuote.ExpiresAt = deadline
	}
	execQuote.CreatedAt = now
	execQuote.UpdatedAt = now

	savedExecQuote, err := s.store.SaveQuote(ctx, execQuote)
	if err != nil {
		return CreateSwapResult{}, err
	}

	swap := domain.Swap{
		ID:                     "swp_" + uuid.NewString(),
		QuoteID:                savedExecQuote.ID,
		IdempotencyPayloadHash: payloadHash,
		SessionID:              params.SessionID,
		State:                  domain.StateAwaitingUserDeposit,
		ProviderStatus:         domain.ProviderStatusPendingDeposit,
		OriginChain:            originChain.Key,
		DestinationChain:       destChain.Key,
		DepositAddress:         savedExecQuote.DepositAddress,
		DepositMemo:            savedExecQuote.DepositMemo,
		RecipientAddress:       normalizedRecipient,
		RefundAddress:          normalizedRefund,
		ExpiresAt:              savedExecQuote.ExpiresAt,
		NextPollAt:             now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	created, replayed, err := s.store.CreateSwap(ctx, swap, params.IdempotencyKey)
	if err != nil {
		return CreateSwapResult{}, err
	}
	return CreateSwapResult{Swap: created, Replayed: replayed, ExecutableQuote: savedExecQuote}, nil
}

// reviewedAddresses returns the recipient and refund address for an
// executable swap. The values reviewed in the dry quote win; a caller may
// repeat them but may not change them.
func (s *Service) reviewedAddresses(quote domain.Quote, params CreateSwapParams) (string, string, error) {
	recipient, refundTo := quote.Recipient, quote.RefundTo
	if recipient == "" {
		recipient = params.Recipient
	} else if params.Recipient != "" && !strings.EqualFold(params.Recipient, recipient) {
		return "", "", domain.New(domain.CodeInvalidRecipient,
			"The recipient differs from the reviewed quote; request a fresh quote.")
	}
	if refundTo == "" {
		refundTo = params.RefundTo
	} else if params.RefundTo != "" && !strings.EqualFold(params.RefundTo, refundTo) {
		return "", "", domain.New(domain.CodeInvalidRefund,
			"The refund address differs from the reviewed quote; request a fresh quote.")
	}
	if recipient == "" {
		return "", "", domain.New(domain.CodeRecipientRequired, "Recipient address is required.")
	}
	if refundTo == "" {
		return "", "", domain.New(domain.CodeRefundRequired, "Refund address is required.")
	}
	return recipient, refundTo, nil
}

func (s *Service) createSimulatedSwap(
	ctx context.Context,
	quote domain.Quote,
	params CreateSwapParams,
	payloadHash string,
	now time.Time,
) (CreateSwapResult, error) {
	originChain, destChain := "simulation", "simulation"
	if asset, ok := s.LookupAsset(ctx, quote.OriginAssetID); ok {
		originChain = asset.Chain
	}
	if asset, ok := s.LookupAsset(ctx, quote.DestinationAssetID); ok {
		destChain = asset.Chain
	}
	recipient := firstNonEmpty(params.Recipient, quote.Recipient, simulationAddress)
	refundTo := firstNonEmpty(params.RefundTo, quote.RefundTo, simulationAddress)

	swap := domain.Swap{
		ID:                     "swp_sim_" + uuid.NewString(),
		QuoteID:                quote.ID,
		IdempotencyPayloadHash: payloadHash,
		SessionID:              params.SessionID,
		State:                  domain.StateAwaitingUserDeposit,
		ProviderStatus:         domain.ProviderStatusPendingDeposit,
		Simulated:              true,
		OriginChain:            originChain,
		DestinationChain:       destChain,
		RecipientAddress:       recipient,
		RefundAddress:          refundTo,
		ExpiresAt:              now.Add(24 * time.Hour),
		NextPollAt:             now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	created, replayed, err := s.store.CreateSwap(ctx, swap, params.IdempotencyKey)
	if err != nil {
		return CreateSwapResult{}, err
	}
	return CreateSwapResult{Swap: created, Replayed: replayed, ExecutableQuote: quote}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
