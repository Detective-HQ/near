// Package memstore is an in-process implementation of the swap service store.
//
// It mirrors the PostgreSQL repository's contract (idempotent creates,
// optimistic state transitions, atomic deposit recording, settlement guards,
// keyset pagination) so the API can run the real service pipeline in local
// development without a database, and so service tests exercise the same
// semantics. It is not durable and is not meant for production.
package memstore

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/repository"
)

// maxQuotes bounds memory: expired quotes beyond this are evicted.
const maxQuotes = 10_000

// Store is a concurrency-safe in-memory swap store.
type Store struct {
	mu           sync.RWMutex
	quotes       map[string]domain.Quote
	quoteOrder   []string // insertion order, for O(1) amortized eviction
	swaps        map[string]domain.Swap
	byIdemKey    map[string]string
	events       map[string][]domain.Event
	transactions map[string][]domain.Deposit
	depositOwner map[string]string // chain|hash -> swap id
	assets       map[string]domain.Asset
	nextID       int64
	now          func() time.Time
}

// New creates an empty store.
func New() *Store {
	return &Store{
		quotes:       map[string]domain.Quote{},
		swaps:        map[string]domain.Swap{},
		byIdemKey:    map[string]string{},
		events:       map[string][]domain.Event{},
		transactions: map[string][]domain.Deposit{},
		depositOwner: map[string]string{},
		assets:       map[string]domain.Asset{},
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func cloneInt(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func cloneQuote(quote domain.Quote) domain.Quote {
	quote.AmountInAtomic = cloneInt(quote.AmountInAtomic)
	quote.AmountOutEstimateAtomic = cloneInt(quote.AmountOutEstimateAtomic)
	quote.MinAmountOutAtomic = cloneInt(quote.MinAmountOutAtomic)
	return quote
}

func clonePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}

// SaveQuote stores a quote.
func (s *Store) SaveQuote(_ context.Context, quote domain.Quote) (domain.Quote, error) {
	if strings.TrimSpace(quote.ID) == "" || quote.AmountInAtomic == nil || quote.AmountInAtomic.Sign() <= 0 {
		return domain.Quote{}, domain.New(domain.CodeValidationFailed, "Quote id and a positive amount are required.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.quotes[quote.ID]; exists {
		return domain.Quote{}, domain.New(domain.CodeConflict, "The record conflicts with an existing record.")
	}
	if len(s.quotes) >= maxQuotes {
		s.evictQuotesLocked()
	}
	now := s.now()
	if quote.CreatedAt.IsZero() {
		quote.CreatedAt = now
	}
	if quote.UpdatedAt.IsZero() {
		quote.UpdatedAt = quote.CreatedAt
	}
	s.quotes[quote.ID] = cloneQuote(quote)
	s.quoteOrder = append(s.quoteOrder, quote.ID)
	return cloneQuote(quote), nil
}

// evictQuotesLocked drops the oldest quotes that no swap references until the
// store is back under its cap. It walks the insertion-order queue from the
// front, so each quote is visited at most once over its lifetime instead of
// rescanning the whole map on every insert.
func (s *Store) evictQuotesLocked() {
	referenced := make(map[string]bool, len(s.swaps))
	for _, swap := range s.swaps {
		referenced[swap.QuoteID] = true
	}
	target := maxQuotes * 9 / 10
	kept := s.quoteOrder[:0:0]
	index := 0
	for ; index < len(s.quoteOrder) && len(s.quotes) > target; index++ {
		id := s.quoteOrder[index]
		if referenced[id] {
			kept = append(kept, id)
			continue
		}
		delete(s.quotes, id)
	}
	s.quoteOrder = append(kept, s.quoteOrder[index:]...)
}

// GetQuote loads a quote.
func (s *Store) GetQuote(_ context.Context, id string) (domain.Quote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	quote, ok := s.quotes[id]
	if !ok {
		return domain.Quote{}, domain.New(domain.CodeQuoteNotFound, "Quote not found.")
	}
	return cloneQuote(quote), nil
}

// GetQuotes loads several quotes keyed by id.
func (s *Store) GetQuotes(_ context.Context, ids []string) (map[string]domain.Quote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]domain.Quote, len(ids))
	for _, id := range ids {
		if quote, ok := s.quotes[id]; ok {
			out[id] = cloneQuote(quote)
		}
	}
	return out, nil
}

// CreateSwap inserts a swap idempotently together with its initial event.
func (s *Store) CreateSwap(_ context.Context, swap domain.Swap, idempotencyKey string) (domain.Swap, bool, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return domain.Swap{}, false, domain.New(domain.CodeIdempotencyKeyRequired, "An Idempotency-Key header is required.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.byIdemKey[key]; ok {
		existing := s.swaps[existingID]
		if !strings.EqualFold(existing.IdempotencyPayloadHash, swap.IdempotencyPayloadHash) {
			return domain.Swap{}, false, domain.New(domain.CodeIdempotencyConflict,
				"The idempotency key was already used with a different request payload.")
		}
		return existing, true, nil
	}
	if _, ok := s.quotes[swap.QuoteID]; !ok {
		return domain.Swap{}, false, domain.New(domain.CodeValidationFailed, "A referenced record does not exist.")
	}
	if _, exists := s.swaps[swap.ID]; exists {
		return domain.Swap{}, false, domain.New(domain.CodeConflict, "The record conflicts with an existing record.")
	}
	now := s.now()
	swap.IdempotencyKey = key
	if swap.State == "" {
		swap.State = domain.StateCreated
	}
	if swap.CreatedAt.IsZero() {
		swap.CreatedAt = now
	}
	if swap.UpdatedAt.IsZero() {
		swap.UpdatedAt = swap.CreatedAt
	}
	if swap.NextPollAt.IsZero() {
		swap.NextPollAt = swap.CreatedAt
	}
	s.swaps[swap.ID] = swap
	s.byIdemKey[key] = swap.ID
	event := domain.InitialEvent(swap)
	s.appendEventLocked(event)
	return swap, false, nil
}

func (s *Store) appendEventLocked(event domain.Event) domain.Event {
	s.nextID++
	event.ID = s.nextID
	event.Payload = clonePayload(event.Payload)
	s.events[event.SwapID] = append(s.events[event.SwapID], event)
	return event
}

// GetSwapByID loads a swap.
func (s *Store) GetSwapByID(_ context.Context, id string) (domain.Swap, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	swap, ok := s.swaps[id]
	if !ok {
		return domain.Swap{}, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}
	return swap, nil
}

// GetSwapByIdempotencyKey loads the swap created with a key.
func (s *Store) GetSwapByIdempotencyKey(_ context.Context, key string) (domain.Swap, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byIdemKey[strings.TrimSpace(key)]
	if !ok {
		return domain.Swap{}, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}
	return s.swaps[id], nil
}

// ClaimSwap assigns an unowned swap to the principal if the wallet matches.
func (s *Store) ClaimSwap(_ context.Context, id, principal, wallet string) (domain.Swap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	swap, ok := s.swaps[id]
	if !ok {
		return domain.Swap{}, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}
	if swap.SessionID != "" && !strings.EqualFold(swap.SessionID, principal) {
		return domain.Swap{}, domain.New(domain.CodeForbidden, "Swap is already claimed.")
	}
	if !strings.EqualFold(swap.RefundAddress, wallet) && !strings.EqualFold(swap.RecipientAddress, wallet) {
		return domain.Swap{}, domain.New(domain.CodeForbidden, "Wallet does not match swap recipient or refund address.")
	}
	swap.SessionID = principal
	swap.UpdatedAt = s.now()
	s.swaps[id] = swap
	return swap, nil
}

// ListStuckSwaps returns swaps with stuck_at set, ordered by stuck_at descending.
func (s *Store) ListStuckSwaps(_ context.Context, limit int) ([]domain.Swap, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []domain.Swap
	for _, sw := range s.swaps {
		if !sw.StuckAt.IsZero() {
			list = append(list, sw)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].StuckAt.After(list[j].StuckAt)
	})
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// RepollSwap clears stuck_at and sets next_poll_at to now.
func (s *Store) RepollSwap(_ context.Context, id string) (domain.Swap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sw, ok := s.swaps[id]
	if !ok {
		return domain.Swap{}, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}
	now := s.now()
	sw.StuckAt = time.Time{}
	sw.NextPollAt = now
	sw.UpdatedAt = now
	s.swaps[id] = sw
	return sw, nil
}

// UpdateSwapState applies one optimistic transition plus its audit event.
func (s *Store) UpdateSwapState(
	_ context.Context,
	swapID string,
	oldState, newState domain.State,
	providerStatus domain.ProviderStatus,
	payload map[string]any,
) (domain.Swap, domain.Event, error) {
	if err := domain.CanTransition(oldState, newState); err != nil {
		return domain.Swap{}, domain.Event{}, err
	}
	providerStatus = providerStatus.Normalize()
	if newState == domain.StateSucceeded && providerStatus != domain.ProviderStatusSuccess {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeInvalidSwapState,
			"SUCCEEDED requires a verified SUCCESS provider status.")
	}
	if newState == domain.StateRefunded && providerStatus != domain.ProviderStatusRefunded {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeInvalidSwapState,
			"REFUNDED requires a REFUNDED provider status.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	swap, ok := s.swaps[swapID]
	if !ok {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}
	if swap.State != oldState {
		return domain.Swap{}, domain.Event{}, domain.Newf(domain.CodeStateConflict,
			"Swap %s is not in state %s; re-read the swap and retry.", swapID, oldState)
	}
	now := s.now()
	swap.State = newState
	swap.ProviderStatus = providerStatus
	swap.RealSettlement = newState == domain.StateSucceeded && !swap.Simulated
	swap.UpdatedAt = now
	s.swaps[swapID] = swap
	event := s.appendEventLocked(domain.Event{
		SwapID: swapID, OldState: oldState, NewState: newState,
		ProviderStatus: providerStatus, Payload: payload, CreatedAt: now,
	})
	return swap, event, nil
}

// RecordDeposit atomically attaches a deposit and advances the swap.
func (s *Store) RecordDeposit(
	_ context.Context,
	swapID, chain, txHash string,
	providerStatus domain.ProviderStatus,
	payload map[string]any,
) (domain.Swap, domain.Deposit, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	swap, ok := s.swaps[swapID]
	if !ok {
		return domain.Swap{}, domain.Deposit{}, false, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}
	if swap.SourceTxHash != "" {
		if !strings.EqualFold(swap.SourceTxHash, txHash) {
			return domain.Swap{}, domain.Deposit{}, false, domain.New(domain.CodeConflict,
				"A different deposit transaction hash is already attached to this swap.")
		}
		for _, deposit := range s.transactions[swapID] {
			if deposit.Role == domain.TransactionRoleDeposit {
				return swap, deposit, true, nil
			}
		}
		return swap, domain.Deposit{}, true, nil
	}
	advance := swap.State == domain.StateAwaitingUserDeposit
	if !advance {
		if swap.State.Terminal() {
			return domain.Swap{}, domain.Deposit{}, false, domain.Newf(domain.CodeInvalidSwapState,
				"Cannot record a deposit for a swap in terminal state %s.", swap.State)
		}
		if err := domain.CanTransition(swap.State, domain.StateDepositSubmitted); err != nil && swap.State == domain.StateQuoted {
			return domain.Swap{}, domain.Deposit{}, false, err
		}
	}
	ownerKey := chain + "|" + strings.ToLower(txHash)
	if owner, taken := s.depositOwner[ownerKey]; taken && owner != swapID {
		return domain.Swap{}, domain.Deposit{}, false, domain.New(domain.CodeConflict,
			"This deposit transaction is already attached to another swap.")
	}
	now := s.now()
	s.nextID++
	deposit := domain.Deposit{
		ID: s.nextID, SwapID: swapID, ChainKey: chain, TxHash: txHash,
		Role: domain.TransactionRoleDeposit, ObservedAt: now, CreatedAt: now,
	}
	s.transactions[swapID] = append(s.transactions[swapID], deposit)
	s.depositOwner[ownerKey] = swapID

	if !advance {
		swap.SourceTxHash = txHash
		swap.UpdatedAt = now
		s.swaps[swapID] = swap
		return swap, deposit, false, nil
	}

	oldState := swap.State
	swap.State = domain.StateDepositSubmitted
	swap.ProviderStatus = providerStatus.Normalize()
	swap.SourceTxHash = txHash
	swap.NextPollAt = now
	swap.PollAttempts = 0
	swap.UpdatedAt = now
	s.swaps[swapID] = swap
	s.appendEventLocked(domain.Event{
		SwapID: swapID, OldState: oldState, NewState: domain.StateDepositSubmitted,
		ProviderStatus: swap.ProviderStatus, Payload: payload, CreatedAt: now,
	})
	return swap, deposit, false, nil
}

// ListSwapsBySession returns one keyset page owned by sessionID. An empty
// sessionID lists anonymous swaps only.
func (s *Store) ListSwapsBySession(_ context.Context, sessionID string, limit int, cursor string) ([]domain.Swap, string, error) {
	if limit <= 0 {
		limit = repository.DefaultListLimit
	}
	if limit > repository.MaxListLimit {
		limit = repository.MaxListLimit
	}
	var position *repository.Cursor
	if strings.TrimSpace(cursor) != "" {
		decoded, err := repository.DecodeCursor(cursor)
		if err != nil {
			return nil, "", domain.Wrap(domain.CodeInvalidRequest, "Invalid pagination cursor.", err)
		}
		position = &decoded
	}
	s.mu.RLock()
	matched := make([]domain.Swap, 0, len(s.swaps))
	for _, swap := range s.swaps {
		if !strings.EqualFold(swap.SessionID, sessionID) {
			continue
		}
		if position != nil {
			if swap.CreatedAt.After(position.CreatedAt) ||
				(swap.CreatedAt.Equal(position.CreatedAt) && swap.ID >= position.ID) {
				continue
			}
		}
		matched = append(matched, swap)
	}
	s.mu.RUnlock()
	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}
		return matched[i].ID > matched[j].ID
	})
	next := ""
	if len(matched) > limit {
		matched = matched[:limit]
		last := matched[len(matched)-1]
		next = repository.EncodeCursor(repository.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return matched, next, nil
}

// ListEventsBySwapIDs loads audit trails.
func (s *Store) ListEventsBySwapIDs(_ context.Context, ids []string) (map[string][]domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]domain.Event, len(ids))
	for _, id := range ids {
		events := s.events[id]
		copied := make([]domain.Event, len(events))
		for i, event := range events {
			event.Payload = clonePayload(event.Payload)
			copied[i] = event
		}
		out[id] = copied
	}
	return out, nil
}

// ListTransactionsBySwapIDs loads chain transactions.
func (s *Store) ListTransactionsBySwapIDs(_ context.Context, ids []string) (map[string][]domain.Deposit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]domain.Deposit, len(ids))
	for _, id := range ids {
		out[id] = append([]domain.Deposit(nil), s.transactions[id]...)
	}
	return out, nil
}

// UpsertAssets replaces the catalogue for a source, soft-disabling missing ones.
func (s *Store) UpsertAssets(_ context.Context, assets []domain.Asset, source string, observedAt time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool, len(assets))
	for _, asset := range assets {
		asset.Source = source
		asset.Enabled = true
		if asset.ObservedAt.IsZero() {
			asset.ObservedAt = observedAt
		}
		s.assets[asset.ID] = asset
		seen[asset.ID] = true
	}
	for id, asset := range s.assets {
		if asset.Source == source && !seen[id] {
			asset.Enabled = false
			s.assets[id] = asset
		}
	}
	return len(assets), nil
}

// PutAsset seeds one asset (tests and local fixtures).
func (s *Store) PutAsset(asset domain.Asset) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[asset.ID] = asset
}

// GetAssetByID loads an enabled asset by id or provider asset id.
func (s *Store) GetAssetByID(_ context.Context, id string) (domain.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if asset, ok := s.assets[id]; ok && asset.Enabled {
		return asset, nil
	}
	for _, asset := range s.assets {
		if asset.Enabled && asset.ProviderAssetID == id {
			return asset, nil
		}
	}
	return domain.Asset{}, domain.New(domain.CodeUnknownAsset, "Asset not found.")
}

// ListAssets lists enabled assets, optionally filtered by chain.
func (s *Store) ListAssets(_ context.Context, chain string) ([]domain.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Asset, 0, len(s.assets))
	for _, asset := range s.assets {
		if !asset.Enabled || (chain != "" && !strings.EqualFold(asset.Chain, chain)) {
			continue
		}
		out = append(out, asset)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Chain != out[j].Chain {
			return out[i].Chain < out[j].Chain
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out, nil
}
