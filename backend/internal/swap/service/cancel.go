package service

import (
	"context"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
)

// CancelSwap discards an unfunded quote instruction.
//
// Rules:
//   - Discards unfunded swaps in state AWAITING_USER_DEPOSIT (or QUOTED).
//   - If the swap is already CANCELLED, returns the swap with no error.
//   - If the swap has a recorded source_tx_hash, cancellation is strictly rejected
//     because funds may already be in flight.
//   - If the provider deadline has already passed at the time of cancellation,
//     the swap is transitioned to EXPIRED.
//   - If a session ID is provided and does not match the swap owner, returns FORBIDDEN.
func (s *Service) CancelSwap(ctx context.Context, swapID, sessionID string) (domain.Swap, error) {
	swapID = strings.TrimSpace(swapID)
	if swapID == "" {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Swap ID is required.")
	}

	swap, err := s.store.GetSwapByID(ctx, swapID)
	if err != nil {
		return domain.Swap{}, err
	}

	if sessionID != "" && swap.SessionID != "" && !strings.EqualFold(swap.SessionID, sessionID) {
		return domain.Swap{}, domain.New(domain.CodeForbidden, "You do not own this swap record.")
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

	now := s.now()
	if !swap.ExpiresAt.IsZero() && swap.ExpiresAt.Before(now) {
		updated, _, expireErr := s.store.UpdateSwapState(
			ctx,
			swap.ID,
			swap.State,
			domain.StateExpired,
			swap.ProviderStatus,
			map[string]any{"reason": "cancelled_after_expiry"},
		)
		if expireErr != nil {
			return domain.Swap{}, expireErr
		}
		return updated, nil
	}

	updated, _, cancelErr := s.store.UpdateSwapState(
		ctx,
		swap.ID,
		swap.State,
		domain.StateCancelled,
		swap.ProviderStatus,
		map[string]any{"reason": "user_cancelled"},
	)
	if cancelErr != nil {
		return domain.Swap{}, cancelErr
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
		return domain.Swap{}, domain.New(domain.CodeInvalidSwapState,
			"Cannot mark a funded swap as expired.")
	}

	updated, _, err := s.store.UpdateSwapState(
		ctx,
		swap.ID,
		swap.State,
		domain.StateExpired,
		swap.ProviderStatus,
		map[string]any{"reason": "deadline_exceeded"},
	)
	if err != nil {
		return domain.Swap{}, err
	}

	return updated, nil
}
