package service

import (
	"context"
	"strings"

	"universal-defi-intelligence/internal/swap/domain"
)

// ClaimSwap assigns an unowned swap to the authenticated principal, provided
// the swap's recipient or refund address matches the authenticated wallet.
func (s *Service) ClaimSwap(ctx context.Context, id, principal, wallet string) (domain.Swap, error) {
	id = strings.TrimSpace(id)
	principal = strings.TrimSpace(principal)
	wallet = strings.TrimSpace(wallet)
	if id == "" {
		return domain.Swap{}, domain.New(domain.CodeValidationFailed, "Swap id is required.")
	}
	if principal == "" || wallet == "" {
		return domain.Swap{}, domain.New(domain.CodeUnauthorized, "Authentication is required to claim a swap.")
	}
	return s.store.ClaimSwap(ctx, id, principal, wallet)
}
