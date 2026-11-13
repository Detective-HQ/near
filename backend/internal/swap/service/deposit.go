package service

import (
	"context"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// RecordDepositParams contains the user-reported source-chain deposit data.
type RecordDepositParams struct {
	SwapID    string `json:"swapId"`
	TxHash    string `json:"txHash"`
	ChainID   string `json:"chainId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// RecordDepositResult is the outcome of recording a deposit transaction.
type RecordDepositResult struct {
	Swap      domain.Swap    `json:"swap"`
	Deposit   domain.Deposit `json:"deposit"`
	Duplicate bool           `json:"duplicate"`
}

// RecordDeposit processes a wallet-reported source transaction.
//
// Behavior:
//   - Validates the transaction hash per the origin chain's family.
//   - If the same hash is replayed for the same swap, returns HTTP 200 / Duplicate=true.
//   - If a different hash is submitted, returns CONFLICT.
//   - If the swap is unfunded and expired, rejects with QUOTE_EXPIRED.
//   - If the swap is in a terminal state, rejects with INVALID_SWAP_STATE.
//   - Atomically records the transaction and transitions the swap from
//     AWAITING_USER_DEPOSIT -> DEPOSIT_SUBMITTED with provider status KNOWN_DEPOSIT_TX.
//   - Submits the deposit notification to the provider via SubmitDepositTx. A
//     provider error during notification does NOT roll back local persistence;
//     the background poller remains the authority for eventual settlement.
func (s *Service) RecordDeposit(ctx context.Context, params RecordDepositParams) (RecordDepositResult, error) {
	swapID := strings.TrimSpace(params.SwapID)
	if swapID == "" {
		return RecordDepositResult{}, domain.New(domain.CodeValidationFailed, "Swap ID is required.")
	}
	rawTxHash := strings.TrimSpace(params.TxHash)
	if rawTxHash == "" {
		return RecordDepositResult{}, domain.New(domain.CodeInvalidTxHash, "Transaction hash is required.")
	}

	swap, err := s.store.GetSwapByID(ctx, swapID)
	if err != nil {
		return RecordDepositResult{}, err
	}

	normalizedHash, err := ValidateTxHash(s.registry, swap.OriginChain, rawTxHash)
	if err != nil {
		return RecordDepositResult{}, err
	}

	// Idempotent duplicate check: same swap + same tx hash
	if swap.SourceTxHash != "" && strings.EqualFold(swap.SourceTxHash, normalizedHash) {
		return RecordDepositResult{
			Swap:      swap,
			Deposit:   domain.Deposit{SwapID: swap.ID, ChainKey: swap.OriginChain, TxHash: normalizedHash, Role: domain.TransactionRoleDeposit},
			Duplicate: true,
		}, nil
	}

	// Conflict check: different tx hash already recorded
	if swap.SourceTxHash != "" && !strings.EqualFold(swap.SourceTxHash, normalizedHash) {
		return RecordDepositResult{}, domain.New(domain.CodeConflict,
			"A different deposit transaction hash is already attached to this swap.")
	}

	// Terminal state check
	if swap.State.Terminal() {
		return RecordDepositResult{}, domain.Newf(domain.CodeInvalidSwapState,
			"Cannot record a deposit transaction for swap %s in terminal state %s.", swap.ID, swap.State)
	}

	// Expiry check
	now := s.now()
	if !swap.ExpiresAt.IsZero() && swap.ExpiresAt.Before(now) {
		return RecordDepositResult{}, domain.New(domain.CodeQuoteExpired,
			"The quote instruction has expired; deposit cannot be accepted.")
	}

	// Must be awaiting user deposit
	if swap.State != domain.StateAwaitingUserDeposit {
		return RecordDepositResult{}, domain.Newf(domain.CodeInvalidSwapState,
			"Swap %s is in state %s; expected AWAITING_USER_DEPOSIT.", swap.ID, swap.State)
	}

	// 1. Record transaction in repository
	deposit, _, err := s.store.RecordTransaction(ctx, swap.ID, swap.OriginChain, normalizedHash, domain.TransactionRoleDeposit)
	if err != nil {
		return RecordDepositResult{}, err
	}

	// 2. Advance state: AWAITING_USER_DEPOSIT -> DEPOSIT_SUBMITTED
	payload := map[string]any{
		"txHash":   normalizedHash,
		"chainKey": swap.OriginChain,
	}
	updatedSwap, _, err := s.store.UpdateSwapState(
		ctx,
		swap.ID,
		domain.StateAwaitingUserDeposit,
		domain.StateDepositSubmitted,
		domain.ProviderStatusKnownDepositTx,
		payload,
	)
	if err != nil {
		return RecordDepositResult{}, err
	}

	// 3. Notify provider (as an accelerator). If this fails, we DO NOT rollback
	// because the on-chain transaction was already broadcast.
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		bgCtx, cancel := context.WithTimeout(context.Background(), s.quoteDeadline)
		defer cancel()

		_, submitErr := s.provider.SubmitDepositTx(bgCtx, providers.DepositRequest{
			TxHash:         normalizedHash,
			DepositAddress: swap.DepositAddress,
			DepositMemo:    swap.DepositMemo,
		})
		if submitErr != nil {
			s.logger.Warn("provider deposit submission notification failed; background poller will recover",
				"swap_id", swap.ID,
				"tx_hash", normalizedHash,
				"error", submitErr,
			)
		}
	}()

	return RecordDepositResult{
		Swap:      updatedSwap,
		Deposit:   deposit,
		Duplicate: false,
	}, nil
}
