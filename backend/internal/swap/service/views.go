package service

import (
	"context"
	"strings"

	"golang.org/x/sync/errgroup"

	"universal-defi-intelligence/internal/swap/domain"
)

// SwapView is a swap together with everything the API renders for it.
type SwapView struct {
	Swap         domain.Swap
	Quote        domain.Quote
	Events       []domain.Event
	Transactions []domain.Deposit
}

// GetSwap loads a swap the caller is allowed to see.
func (s *Service) GetSwap(ctx context.Context, swapID, sessionID string) (domain.Swap, error) {
	swap, err := s.store.GetSwapByID(ctx, strings.TrimSpace(swapID))
	if err != nil {
		return domain.Swap{}, err
	}
	if err := checkOwner(swap, sessionID); err != nil {
		return domain.Swap{}, err
	}
	return swap, nil
}

// GetSwapView loads one swap with its quote, audit trail and transactions.
func (s *Service) GetSwapView(ctx context.Context, swapID, sessionID string) (SwapView, error) {
	swap, err := s.GetSwap(ctx, swapID, sessionID)
	if err != nil {
		return SwapView{}, err
	}
	views, err := s.BuildViews(ctx, []domain.Swap{swap})
	if err != nil {
		return SwapView{}, err
	}
	return views[0], nil
}

// ListActivity returns one page of swaps owned by sessionID (anonymous swaps
// when sessionID is empty), newest first.
func (s *Service) ListActivity(ctx context.Context, sessionID string, limit int, cursor string) ([]SwapView, string, error) {
	swaps, next, err := s.store.ListSwapsBySession(ctx, strings.TrimSpace(sessionID), limit, cursor)
	if err != nil {
		return nil, "", err
	}
	views, err := s.BuildViews(ctx, swaps)
	if err != nil {
		return nil, "", err
	}
	return views, next, nil
}

// BuildViews hydrates swaps with three batched queries run concurrently,
// independent of the number of swaps (no N+1 round trips).
func (s *Service) BuildViews(ctx context.Context, swaps []domain.Swap) ([]SwapView, error) {
	if len(swaps) == 0 {
		return []SwapView{}, nil
	}
	ids := make([]string, len(swaps))
	quoteIDs := make([]string, len(swaps))
	for i, swap := range swaps {
		ids[i] = swap.ID
		quoteIDs[i] = swap.QuoteID
	}

	var (
		quotes       map[string]domain.Quote
		events       map[string][]domain.Event
		transactions map[string][]domain.Deposit
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() (err error) { quotes, err = s.store.GetQuotes(groupCtx, quoteIDs); return })
	group.Go(func() (err error) { events, err = s.store.ListEventsBySwapIDs(groupCtx, ids); return })
	group.Go(func() (err error) { transactions, err = s.store.ListTransactionsBySwapIDs(groupCtx, ids); return })
	if err := group.Wait(); err != nil {
		return nil, err
	}

	views := make([]SwapView, len(swaps))
	for i, swap := range swaps {
		views[i] = SwapView{
			Swap:         swap,
			Quote:        quotes[swap.QuoteID],
			Events:       events[swap.ID],
			Transactions: transactions[swap.ID],
		}
	}
	return views, nil
}
