package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

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

// CreateSwap executes the executable quoting and swap creation pipeline.
//
// In live mode, it requires the server execution gate (ENABLE_LIVE_EXECUTION=true),
// validates route capabilities against the chain registry, obtains a fresh
// provider executable quote with deposit instructions, and atomically inserts
// the swap starting in state AWAITING_USER_DEPOSIT alongside its initial audit event.
//
// Idempotency: Replaying an identical key and request payload returns the
// existing swap with Replayed=true. Replaying the same key with a different
// payload returns IDEMPOTENCY_CONFLICT.
func (s *Service) CreateSwap(ctx context.Context, params CreateSwapParams) (CreateSwapResult, error) {
	if err := ValidateIdempotencyKey(params.IdempotencyKey); err != nil {
		return CreateSwapResult{}, err
	}

	payloadHash, err := CanonicalPayloadHash(params)
	if err != nil {
		return CreateSwapResult{}, err
	}

	quoteID := strings.TrimSpace(params.QuoteID)
	if quoteID == "" {
		return CreateSwapResult{}, domain.New(domain.CodeValidationFailed, "Quote ID is required.")
	}

	baseQuote, err := s.store.GetQuote(ctx, quoteID)
	if err != nil {
		return CreateSwapResult{}, err
	}

	now := s.now()
	if !baseQuote.ExpiresAt.IsZero() && baseQuote.ExpiresAt.Before(now) {
		return CreateSwapResult{}, domain.New(domain.CodeQuoteExpired, "The base quote has expired; request a fresh quote.")
	}

	if baseQuote.Mode == domain.QuoteModeSimulation {
		return s.createDemoSwap(ctx, baseQuote, params, payloadHash, now)
	}

	// Live mode checks
	if !s.enableLiveExecution {
		return CreateSwapResult{}, domain.New(domain.CodeLiveExecutionDisabled,
			"Live execution is not enabled on this server.")
	}

	originAsset, err := s.resolveAsset(ctx, baseQuote.OriginAssetID, false)
	if err != nil {
		return CreateSwapResult{}, err
	}
	destAsset, err := s.resolveAsset(ctx, baseQuote.DestinationAssetID, false)
	if err != nil {
		return CreateSwapResult{}, err
	}

	originChain, found := s.registry.Get(originAsset.Chain)
	if !found {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Origin chain %q is not registered.", originAsset.Chain)
	}
	if !originChain.Features.SwapEnabled || !originChain.Features.DepositSupported {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Swaps/deposits are not enabled for chain %q.", originAsset.Chain)
	}

	destChain, found := s.registry.Get(destAsset.Chain)
	if !found {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Destination chain %q is not registered.", destAsset.Chain)
	}
	if !destChain.Features.SwapEnabled {
		return CreateSwapResult{}, domain.Newf(domain.CodeNoRoute, "Swaps are not enabled for destination chain %q.", destAsset.Chain)
	}

	recipient := strings.TrimSpace(params.Recipient)
	if recipient == "" {
		recipient = originAsset.ContractAddress
		if recipient == "" {
			recipient = "0x0000000000000000000000000000000000000000"
		}
	}
	refundTo := strings.TrimSpace(params.RefundTo)
	if refundTo == "" {
		refundTo = recipient
	}

	normalizedRecipient, err := ValidateChainAddress(s.registry, destAsset.Chain, recipient)
	if err != nil {
		return CreateSwapResult{}, err
	}
	normalizedRefund, err := ValidateChainAddress(s.registry, originAsset.Chain, refundTo)
	if err != nil {
		return CreateSwapResult{}, err
	}

	depositMode := "SIMPLE"
	if destChain.Features.MemoRequired || originChain.Features.MemoRequired {
		depositMode = "MEMO"
	}

	deadline := now.Add(s.quoteDeadline)
	execReq := providers.ExecutableQuoteRequest{
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
	}

	quoteRes, err := s.provider.GetExecutableQuote(ctx, execReq)
	if err != nil {
		return CreateSwapResult{}, err
	}

	execQuote := quoteRes.Quote
	if execQuote.ID == "" {
		execQuote.ID = "qte_exec_" + uuid.NewString()
	}
	execQuote.SessionID = params.SessionID
	execQuote.Mode = domain.QuoteModeProviderExecutable
	execQuote.OriginAssetID = originAsset.ID
	execQuote.DestinationAssetID = destAsset.ID
	execQuote.AmountInAtomic = baseQuote.AmountInAtomic
	execQuote.AmountInDecimal = baseQuote.AmountInDecimal
	execQuote.FeeBPS = baseQuote.FeeBPS
	execQuote.SlippageBPS = baseQuote.SlippageBPS
	if execQuote.ExpiresAt.IsZero() {
		execQuote.ExpiresAt = deadline
	}
	execQuote.CreatedAt = now
	execQuote.UpdatedAt = now

	savedExecQuote, err := s.store.SaveQuote(ctx, execQuote)
	if err != nil {
		return CreateSwapResult{}, err
	}

	sessionID := strings.TrimSpace(params.SessionID)
	if sessionID == "" {
		sessionID = baseQuote.SessionID
	}

	swap := domain.Swap{
		ID:                     "swp_" + uuid.NewString(),
		QuoteID:                savedExecQuote.ID,
		IdempotencyKey:         params.IdempotencyKey,
		IdempotencyPayloadHash: payloadHash,
		SessionID:              sessionID,
		State:                  domain.StateAwaitingUserDeposit,
		ProviderStatus:         domain.ProviderStatusPendingDeposit,
		RealSettlement:         false,
		OriginChain:            originAsset.Chain,
		DestinationChain:       destAsset.Chain,
		DepositAddress:         savedExecQuote.DepositAddress,
		DepositMemo:            savedExecQuote.DepositMemo,
		RecipientAddress:       normalizedRecipient,
		RefundAddress:          normalizedRefund,
		ExpiresAt:              savedExecQuote.ExpiresAt,
		NextPollAt:             now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	createdSwap, replayed, err := s.store.CreateSwap(ctx, swap, params.IdempotencyKey)
	if err != nil {
		return CreateSwapResult{}, err
	}

	return CreateSwapResult{
		Swap:            createdSwap,
		Replayed:        replayed,
		ExecutableQuote: savedExecQuote,
	}, nil
}

func (s *Service) createDemoSwap(
	ctx context.Context,
	baseQuote domain.Quote,
	params CreateSwapParams,
	payloadHash string,
	now time.Time,
) (CreateSwapResult, error) {
	sessionID := strings.TrimSpace(params.SessionID)
	if sessionID == "" {
		sessionID = baseQuote.SessionID
	}

	originChain := "near"
	destChain := "base"

	swap := domain.Swap{
		ID:                     "swp_demo_" + uuid.NewString()[:8],
		QuoteID:                baseQuote.ID,
		IdempotencyKey:         params.IdempotencyKey,
		IdempotencyPayloadHash: payloadHash,
		SessionID:              sessionID,
		State:                  domain.StateQuoted,
		ProviderStatus:         domain.ProviderStatusPendingDeposit,
		RealSettlement:         false,
		OriginChain:            originChain,
		DestinationChain:       destChain,
		DepositAddress:         "0xDemoDepositAddress0000000000000000000",
		DepositMemo:            "",
		RecipientAddress:       params.Recipient,
		RefundAddress:          params.RefundTo,
		ExpiresAt:              baseQuote.ExpiresAt,
		NextPollAt:             now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	createdSwap, replayed, err := s.store.CreateSwap(ctx, swap, params.IdempotencyKey)
	if err != nil {
		return CreateSwapResult{}, err
	}

	return CreateSwapResult{
		Swap:            createdSwap,
		Replayed:        replayed,
		ExecutableQuote: baseQuote,
	}, nil
}
