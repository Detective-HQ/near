package service

import (
	"context"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
)

// CancelSwap discards an unfunded swap.
//
//   - Only the owner may cancel a swap that has an owner.
//   - Cancelling an already CANCELLED swap is idempotent.
//   - A swap with a recorded source transaction is never cancelled, because
//     funds may already be in flight.
//   - Only QUOTED and AWAITING_USER_DEPOSIT swaps can be cancelled; a swap past
//     its deadline becomes EXPIRED instead.
func (s *Service) CancelSwap(ctx context.Context, swapID, sessionID string) (domain.Swap, error) {
	swapID = strings.TrimSpace(swapID)
	if swapID == "" {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Swap ID is required.")
	}
	swap, err := s.store.GetSwapByID(ctx, swapID)
	if err != nil {
		return domain.Swap{}, err
	}
	if err := checkOwner(swap, sessionID); err != nil {
		return domain.Swap{}, err
	}
	if swap.State == domain.StateCancelled {
		return swap, nil
	}
	if swap.SourceTxHash != "" {
		return domain.Swap{}, domain.New(domain.CodeInvalidSwapState,
			"Cannot cancel a swap that already has an on-chain deposit transaction.")
	}
	if swap.State != domain.StateAwaitingUserDeposit && swap.State != domain.StateQuoted {
		return domain.Swap{}, domain.Newf(domain.CodeInvalidSwapState,
			"Cannot cancel swap %s in state %s.", swap.ID, swap.State)
	}

	target, reason := domain.StateCancelled, "user_cancelled"
	if !swap.Simulated && !swap.ExpiresAt.IsZero() && !s.now().Before(swap.ExpiresAt) {
		target, reason = domain.StateExpired, "cancelled_after_expiry"
	}
	updated, _, err := s.store.UpdateSwapState(ctx, swap.ID, swap.State, target, swap.ProviderStatus,
		map[string]any{"reason": reason})
	if err != nil {
		return domain.Swap{}, err
	}
	return updated, nil
}

// ExpireSwap marks an unfunded swap as EXPIRED once its deadline is exceeded.
func (s *Service) ExpireSwap(ctx context.Context, swapID string) (domain.Swap, error) {
	swapID = strings.TrimSpace(swapID)
	if swapID == "" {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Swap ID is required.")
	}
	swap, err := s.store.GetSwapByID(ctx, swapID)
	if err != nil {
		return domain.Swap{}, err
	}
	if swap.State.Terminal() {
		return swap, nil
	}
	if swap.SourceTxHash != "" {
		return domain.Swap{}, domain.New(domain.CodeInvalidSwapState, "Cannot mark a funded swap as expired.")
	}
	updated, _, err := s.store.UpdateSwapState(ctx, swap.ID, swap.State, domain.StateExpired,
		swap.ProviderStatus, map[string]any{"reason": "deadline_exceeded"})
	if err != nil {
		return domain.Swap{}, err
	}
	return updated, nil
}

// AdvanceSimulation moves a simulated swap one step along the happy path:
// AWAITING_USER_DEPOSIT -> DEPOSIT_SUBMITTED -> PROCESSING -> SUCCEEDED.
// It only ever touches simulated records, and a simulated SUCCEEDED is never a
// real settlement.
func (s *Service) AdvanceSimulation(ctx context.Context, swapID, sessionID string) (domain.Swap, error) {
	swap, err := s.store.GetSwapByID(ctx, strings.TrimSpace(swapID))
	if err != nil {
		return domain.Swap{}, err
	}
	if err := checkOwner(swap, sessionID); err != nil {
		return domain.Swap{}, err
	}
	if !swap.Simulated {
		return domain.Swap{}, domain.New(domain.CodeInvalidSwapState,
			"Only simulation records can be advanced manually; provider swaps follow the provider.")
	}

	var (
		next   domain.State
		status domain.ProviderStatus
		detail string
	)
	switch swap.State {
	case domain.StateAwaitingUserDeposit:
		next, status, detail = domain.StateDepositSubmitted, domain.ProviderStatusKnownDepositTx,
			"A simulated deposit event was added. No chain transaction exists."
	case domain.StateDepositSubmitted, domain.StateIncompleteDeposit:
		next, status, detail = domain.StateProcessing, domain.ProviderStatusProcessing,
			"The simulation advanced to processing."
	case domain.StateProcessing:
		next, status, detail = domain.StateSucceeded, domain.ProviderStatusSuccess,
			"The simulated lifecycle completed. No funds moved."
	default:
		return domain.Swap{}, domain.New(domain.CodeInvalidSwapState, "This simulation has no further transitions.")
	}
	updated, _, err := s.store.UpdateSwapState(ctx, swap.ID, swap.State, next, status,
		map[string]any{"detail": detail, "simulated": true})
	if err != nil {
		return domain.Swap{}, err
	}
	return updated, nil
}
