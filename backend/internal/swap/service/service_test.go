package service

import (
	"context"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// mockStore is an in-memory thread-safe implementation of service.Store.
type mockStore struct {
	mu           sync.Mutex
	quotes       map[string]domain.Quote
	swaps        map[string]domain.Swap
	idempotent   map[string]domain.Swap
	transactions map[string][]domain.Deposit
	assets       map[string]domain.Asset
	events       map[string][]domain.Event
}

func newMockStore() *mockStore {
	return &mockStore{
		quotes:       make(map[string]domain.Quote),
		swaps:        make(map[string]domain.Swap),
		idempotent:   make(map[string]domain.Swap),
		transactions: make(map[string][]domain.Deposit),
		assets:       make(map[string]domain.Asset),
		events:       make(map[string][]domain.Event),
	}
}

func (m *mockStore) SaveQuote(ctx context.Context, quote domain.Quote) (domain.Quote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotes[quote.ID] = quote
	return quote, nil
}

func (m *mockStore) GetQuote(ctx context.Context, id string) (domain.Quote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	quote, ok := m.quotes[id]
	if !ok {
		return domain.Quote{}, domain.New(domain.CodeQuoteNotFound, "Quote not found.")
	}
	return quote, nil
}

func (m *mockStore) CreateSwap(ctx context.Context, swap domain.Swap, idempotencyKey string) (domain.Swap, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.idempotent[idempotencyKey]; ok {
		if !strings.EqualFold(existing.IdempotencyPayloadHash, swap.IdempotencyPayloadHash) {
			return domain.Swap{}, false, domain.New(domain.CodeIdempotencyConflict, "Idempotency conflict.")
		}
		return existing, true, nil
	}

	m.swaps[swap.ID] = swap
	m.idempotent[idempotencyKey] = swap
	m.events[swap.ID] = append(m.events[swap.ID], domain.InitialEvent(swap))
	return swap, false, nil
}

func (m *mockStore) GetSwapByID(ctx context.Context, id string) (domain.Swap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	swap, ok := m.swaps[id]
	if !ok {
		return domain.Swap{}, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}
	return swap, nil
}

func (m *mockStore) UpdateSwapState(
	ctx context.Context,
	swapID string,
	oldState, newState domain.State,
	providerStatus domain.ProviderStatus,
	payload map[string]any,
) (domain.Swap, domain.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	swap, ok := m.swaps[swapID]
	if !ok {
		return domain.Swap{}, domain.Event{}, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}

	if err := domain.CanTransition(oldState, newState); err != nil {
		return domain.Swap{}, domain.Event{}, err
	}

	swap.State = newState
	swap.ProviderStatus = providerStatus
	m.swaps[swapID] = swap

	event := domain.Event{
		SwapID:         swapID,
		OldState:       oldState,
		NewState:       newState,
		ProviderStatus: providerStatus,
		Payload:        payload,
		CreatedAt:      time.Now().UTC(),
	}
	m.events[swapID] = append(m.events[swapID], event)

	return swap, event, nil
}

func (m *mockStore) RecordTransaction(
	ctx context.Context,
	swapID, chain, txHash string,
	role domain.TransactionRole,
) (domain.Deposit, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	swap, ok := m.swaps[swapID]
	if !ok {
		return domain.Deposit{}, false, domain.New(domain.CodeSwapNotFound, "Swap not found.")
	}

	for _, dep := range m.transactions[swapID] {
		if strings.EqualFold(dep.TxHash, txHash) && dep.Role == role {
			return dep, false, nil
		}
	}

	dep := domain.Deposit{
		ID:         int64(len(m.transactions[swapID]) + 1),
		SwapID:     swapID,
		ChainKey:   chain,
		TxHash:     txHash,
		Role:       role,
		ObservedAt: time.Now().UTC(),
		CreatedAt:  time.Now().UTC(),
	}
	m.transactions[swapID] = append(m.transactions[swapID], dep)

	if role == domain.TransactionRoleDeposit && swap.SourceTxHash == "" {
		swap.SourceTxHash = txHash
		m.swaps[swapID] = swap
	}

	return dep, true, nil
}

func (m *mockStore) ListSwaps(ctx context.Context, limit int, cursor string) ([]domain.Swap, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]domain.Swap, 0, len(m.swaps))
	for _, s := range m.swaps {
		res = append(res, s)
	}
	return res, "", nil
}

func (m *mockStore) ListSwapsBySession(ctx context.Context, sessionID string, limit int, cursor string) ([]domain.Swap, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]domain.Swap, 0)
	for _, s := range m.swaps {
		if strings.EqualFold(s.SessionID, sessionID) {
			res = append(res, s)
		}
	}
	return res, "", nil
}

func (m *mockStore) GetAssetByID(ctx context.Context, id string) (domain.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.assets[id]; ok {
		return a, nil
	}
	return domain.Asset{}, domain.New(domain.CodeUnknownAsset, "Asset not found.")
}

func (m *mockStore) ListAssets(ctx context.Context, chain string) ([]domain.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []domain.Asset
	for _, a := range m.assets {
		if chain == "" || strings.EqualFold(a.Chain, chain) {
			list = append(list, a)
		}
	}
	return list, nil
}

// mockProvider implements providers.Adapter for testing.
type mockProvider struct {
	getDryQuoteFunc        func(ctx context.Context, req providers.DryQuoteRequest) (providers.QuoteResult, error)
	getExecutableQuoteFunc func(ctx context.Context, req providers.ExecutableQuoteRequest) (providers.QuoteResult, error)
	submitDepositTxFunc    func(ctx context.Context, req providers.DepositRequest) (providers.StatusResult, error)
}

func (m *mockProvider) Name() string { return "mock_provider" }
func (m *mockProvider) Capabilities() providers.Capabilities {
	return providers.Capabilities{Catalogue: true, DryQuote: true, ExecutableQuote: true, Status: true, SubmitDeposit: true}
}
func (m *mockProvider) Health() providers.Health {
	return providers.Health{Configured: true, SignatureVerification: true}
}
func (m *mockProvider) GetTokens(ctx context.Context) ([]domain.Asset, error) {
	return nil, nil
}
func (m *mockProvider) GetDryQuote(ctx context.Context, req providers.DryQuoteRequest) (providers.QuoteResult, error) {
	if m.getDryQuoteFunc != nil {
		return m.getDryQuoteFunc(ctx, req)
	}
	return providers.QuoteResult{
		Quote: domain.Quote{
			ID:                      "qte_mock_dry_1",
			Mode:                    domain.QuoteModeProviderDry,
			AmountInAtomic:          req.AmountAtomic,
			AmountOutEstimateAtomic: big.NewInt(5000000),
			MinAmountOutAtomic:      big.NewInt(4950000),
			ExpiresAt:               time.Now().Add(5 * time.Minute),
		},
	}, nil
}
func (m *mockProvider) GetExecutableQuote(ctx context.Context, req providers.ExecutableQuoteRequest) (providers.QuoteResult, error) {
	if m.getExecutableQuoteFunc != nil {
		return m.getExecutableQuoteFunc(ctx, req)
	}
	return providers.QuoteResult{
		Quote: domain.Quote{
			ID:                      "qte_mock_exec_1",
			Mode:                    domain.QuoteModeProviderExecutable,
			DepositAddress:          "0x1111111111111111111111111111111111111111",
			DepositMemo:             "",
			AmountInAtomic:          req.AmountAtomic,
			AmountOutEstimateAtomic: big.NewInt(5000000),
			MinAmountOutAtomic:      big.NewInt(4950000),
			ExpiresAt:               req.Deadline,
		},
	}, nil
}
func (m *mockProvider) GetStatus(ctx context.Context, req providers.StatusRequest) (providers.StatusResult, error) {
	return providers.StatusResult{ProviderStatus: domain.ProviderStatusProcessing}, nil
}
func (m *mockProvider) SubmitDepositTx(ctx context.Context, req providers.DepositRequest) (providers.StatusResult, error) {
	if m.submitDepositTxFunc != nil {
		return m.submitDepositTxFunc(ctx, req)
	}
	return providers.StatusResult{ProviderStatus: domain.ProviderStatusKnownDepositTx}, nil
}

var testNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

func setupTestService(t *testing.T, liveExecution bool) (*Service, *mockStore, *mockProvider) {
	t.Helper()
	store := newMockStore()

	// Seed test assets
	store.assets["nep141:wrap.near"] = domain.Asset{
		ID:              "nep141:wrap.near",
		ProviderAssetID: "nep141:wrap.near",
		Chain:           "near",
		Symbol:          "NEAR",
		Decimals:        24,
		Enabled:         true,
	}
	store.assets["nep141:usdc.near"] = domain.Asset{
		ID:              "nep141:usdc.near",
		ProviderAssetID: "nep141:usdc.near",
		Chain:           "near",
		Symbol:          "USDC",
		Decimals:        6,
		Enabled:         true,
	}
	store.assets["erc20:base:usdc"] = domain.Asset{
		ID:              "erc20:base:usdc",
		ProviderAssetID: "erc20:base:usdc",
		Chain:           "base",
		Symbol:          "USDC",
		Decimals:        6,
		ContractAddress: "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		Enabled:         true,
	}
	store.assets["erc20:eth:usdc"] = domain.Asset{
		ID:              "erc20:eth:usdc",
		ProviderAssetID: "erc20:eth:usdc",
		Chain:           "ethereum",
		Symbol:          "USDC",
		Decimals:        6,
		ContractAddress: "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
		Enabled:         true,
	}

	reg, err := registry.Load()
	if err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	prov := &mockProvider{}

	svc, err := New(Config{
		Store:               store,
		Provider:            prov,
		Registry:            reg,
		QuoteMode:           config.QuoteModeLive,
		EnableLiveExecution: liveExecution,
		QuoteDeadline:       3 * time.Minute,
		Now:                 func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc, store, prov
}

func TestDryQuoteValidation(t *testing.T) {
	svc, _, _ := setupTestService(t, false)
	ctx := context.Background()

	// 1. Same asset pair rejected
	_, err := svc.DryQuote(ctx, DryQuoteParams{
		OriginAssetID:      "nep141:wrap.near",
		DestinationAssetID: "nep141:wrap.near",
		AmountIn:           "1.0",
		Recipient:          "recipient.near",
		RefundTo:           "refund.near",
	})
	if !domain.IsCode(err, domain.CodeInvalidAssetPair) {
		t.Fatalf("expected INVALID_ASSET_PAIR, got %v", err)
	}

	// 2. Non-positive amount rejected
	_, err = svc.DryQuote(ctx, DryQuoteParams{
		OriginAssetID:      "nep141:wrap.near",
		DestinationAssetID: "nep141:usdc.near",
		AmountIn:           "-1.0",
		Recipient:          "recipient.near",
		RefundTo:           "refund.near",
	})
	if !domain.IsCode(err, domain.CodeInvalidAmount) {
		t.Fatalf("expected INVALID_AMOUNT, got %v", err)
	}

	// 3. Invalid recipient address rejected
	_, err = svc.DryQuote(ctx, DryQuoteParams{
		OriginAssetID:      "nep141:wrap.near",
		DestinationAssetID: "erc20:base:usdc",
		AmountIn:           "1.0",
		Recipient:          "not_an_evm_address",
		RefundTo:           "refund.near",
	})
	if !domain.IsCode(err, domain.CodeInvalidRecipient) {
		t.Fatalf("expected INVALID_RECIPIENT, got %v", err)
	}
}

func TestDryQuoteLiveHappyPath(t *testing.T) {
	svc, store, _ := setupTestService(t, false)
	ctx := context.Background()

	quote, err := svc.DryQuote(ctx, DryQuoteParams{
		OriginAssetID:      "nep141:wrap.near",
		DestinationAssetID: "erc20:base:usdc",
		AmountIn:           "2.5",
		Recipient:          "0x1111111111111111111111111111111111111111",
		RefundTo:           "refund.near",
		SlippageBPS:        150,
	})
	if err != nil {
		t.Fatalf("DryQuote: %v", err)
	}

	if quote.ID == "" {
		t.Fatal("expected non-empty quote ID")
	}
	if quote.Mode != domain.QuoteModeProviderDry {
		t.Fatalf("expected mode %s, got %s", domain.QuoteModeProviderDry, quote.Mode)
	}

	// Stored in repository
	stored, err := store.GetQuote(ctx, quote.ID)
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if stored.ID != quote.ID {
		t.Fatalf("stored ID %s != quote ID %s", stored.ID, quote.ID)
	}
}

func TestCreateSwapExecutionGateDisabled(t *testing.T) {
	svc, store, _ := setupTestService(t, false) // EnableLiveExecution = false
	ctx := context.Background()

	// Persist a base dry quote
	baseQuote := domain.Quote{
		ID:                 "qte_base_1",
		Mode:               domain.QuoteModeProviderDry,
		OriginAssetID:      "nep141:wrap.near",
		DestinationAssetID: "erc20:base:usdc",
		AmountInAtomic:     big.NewInt(1000000000),
		AmountInDecimal:    "1.0",
		ExpiresAt:          testNow.Add(5 * time.Minute),
	}
	_, _ = store.SaveQuote(ctx, baseQuote)

	_, err := svc.CreateSwap(ctx, CreateSwapParams{
		QuoteID:        baseQuote.ID,
		IdempotencyKey: "idem_key_12345",
	})
	if !domain.IsCode(err, domain.CodeLiveExecutionDisabled) {
		t.Fatalf("expected LIVE_EXECUTION_DISABLED, got %v", err)
	}
}

func TestCreateSwapLiveHappyPathAndIdempotency(t *testing.T) {
	svc, store, _ := setupTestService(t, true) // EnableLiveExecution = true
	ctx := context.Background()

	baseQuote := domain.Quote{
		ID:                 "qte_base_1",
		Mode:               domain.QuoteModeProviderDry,
		OriginAssetID:      "erc20:eth:usdc",
		DestinationAssetID: "erc20:base:usdc",
		AmountInAtomic:     big.NewInt(1000000000),
		AmountInDecimal:    "1.0",
		ExpiresAt:          testNow.Add(5 * time.Minute),
	}
	_, _ = store.SaveQuote(ctx, baseQuote)

	params := CreateSwapParams{
		QuoteID:        baseQuote.ID,
		IdempotencyKey: "key_unique_12345",
		Recipient:      "0x1111111111111111111111111111111111111111",
		RefundTo:       "0x2222222222222222222222222222222222222222",
	}

	// First execution -> creates swap
	res1, err := svc.CreateSwap(ctx, params)
	if err != nil {
		t.Fatalf("CreateSwap: %v", err)
	}
	if res1.Replayed {
		t.Fatal("first call should not be replayed")
	}
	if res1.Swap.State != domain.StateAwaitingUserDeposit {
		t.Fatalf("expected state AWAITING_USER_DEPOSIT, got %s", res1.Swap.State)
	}
	if res1.Swap.DepositAddress != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("unexpected deposit address %s", res1.Swap.DepositAddress)
	}

	// Second execution with identical payload -> Replayed=true, same swap
	res2, err := svc.CreateSwap(ctx, params)
	if err != nil {
		t.Fatalf("CreateSwap replay: %v", err)
	}
	if !res2.Replayed {
		t.Fatal("expected replayed = true")
	}
	if res2.Swap.ID != res1.Swap.ID {
		t.Fatalf("replayed swap ID %s != original %s", res2.Swap.ID, res1.Swap.ID)
	}

	// Third execution with same key but different recipient -> IDEMPOTENCY_CONFLICT
	conflictParams := params
	conflictParams.Recipient = "0x3333333333333333333333333333333333333333"
	_, err = svc.CreateSwap(ctx, conflictParams)
	if !domain.IsCode(err, domain.CodeIdempotencyConflict) {
		t.Fatalf("expected IDEMPOTENCY_CONFLICT, got %v", err)
	}
}

func TestCreateSwapUnsupportedChainRoute(t *testing.T) {
	svc, store, _ := setupTestService(t, true)
	ctx := context.Background()

	// Quote with NEAR origin (which is planned / swap_enabled: false in registry)
	baseQuote := domain.Quote{
		ID:                 "qte_near_origin",
		Mode:               domain.QuoteModeProviderDry,
		OriginAssetID:      "nep141:wrap.near",
		DestinationAssetID: "erc20:base:usdc",
		AmountInAtomic:     big.NewInt(1000000000),
		AmountInDecimal:    "1.0",
		ExpiresAt:          testNow.Add(5 * time.Minute),
	}
	_, _ = store.SaveQuote(ctx, baseQuote)

	_, err := svc.CreateSwap(ctx, CreateSwapParams{
		QuoteID:        baseQuote.ID,
		IdempotencyKey: "key_near_route",
		Recipient:      "0x1111111111111111111111111111111111111111",
		RefundTo:       "refund.near",
	})
	if !domain.IsCode(err, domain.CodeNoRoute) {
		t.Fatalf("expected NO_ROUTE for unenabled chain, got %v", err)
	}
}

func TestRecordDepositHappyPathAndDuplicate(t *testing.T) {
	svc, store, _ := setupTestService(t, true)
	ctx := context.Background()

	// Create active swap
	swap := domain.Swap{
		ID:             "swp_test_1",
		OriginChain:    "base",
		DepositAddress: "0x1111111111111111111111111111111111111111",
		State:          domain.StateAwaitingUserDeposit,
		ExpiresAt:      testNow.Add(10 * time.Minute),
	}
	_, _, _ = store.CreateSwap(ctx, swap, "key_swap_1")

	txHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// 1. First record -> updates state and saves tx
	res, err := svc.RecordDeposit(ctx, RecordDepositParams{
		SwapID: swap.ID,
		TxHash: txHash,
	})
	if err != nil {
		t.Fatalf("RecordDeposit: %v", err)
	}
	if res.Duplicate {
		t.Fatal("first record should not be duplicate")
	}
	if res.Swap.State != domain.StateDepositSubmitted {
		t.Fatalf("expected state DEPOSIT_SUBMITTED, got %s", res.Swap.State)
	}
	if res.Swap.SourceTxHash != txHash {
		t.Fatalf("expected SourceTxHash %s, got %s", txHash, res.Swap.SourceTxHash)
	}

	// 2. Repeat record with same hash -> Duplicate=true
	resDup, err := svc.RecordDeposit(ctx, RecordDepositParams{
		SwapID: swap.ID,
		TxHash: txHash,
	})
	if err != nil {
		t.Fatalf("RecordDeposit duplicate: %v", err)
	}
	if !resDup.Duplicate {
		t.Fatal("expected duplicate = true")
	}

	// 3. Different tx hash -> CONFLICT
	_, err = svc.RecordDeposit(ctx, RecordDepositParams{
		SwapID: swap.ID,
		TxHash: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("expected CONFLICT, got %v", err)
	}
}

func TestCancelSwap(t *testing.T) {
	svc, store, _ := setupTestService(t, true)
	ctx := context.Background()

	// 1. Unfunded swap -> CANCELLED
	swap := domain.Swap{
		ID:        "swp_cancel_1",
		State:     domain.StateAwaitingUserDeposit,
		ExpiresAt: testNow.Add(10 * time.Minute),
	}
	_, _, _ = store.CreateSwap(ctx, swap, "key_cancel_1")

	cancelled, err := svc.CancelSwap(ctx, swap.ID, "")
	if err != nil {
		t.Fatalf("CancelSwap: %v", err)
	}
	if cancelled.State != domain.StateCancelled {
		t.Fatalf("expected CANCELLED, got %s", cancelled.State)
	}

	// 2. Funded swap cannot be cancelled
	fundedSwap := domain.Swap{
		ID:           "swp_funded_1",
		State:        domain.StateDepositSubmitted,
		SourceTxHash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpiresAt:    testNow.Add(10 * time.Minute),
	}
	_, _, _ = store.CreateSwap(ctx, fundedSwap, "key_funded_1")

	_, err = svc.CancelSwap(ctx, fundedSwap.ID, "")
	if !domain.IsCode(err, domain.CodeInvalidSwapState) {
		t.Fatalf("expected INVALID_SWAP_STATE, got %v", err)
	}
}

func TestExpireSwap(t *testing.T) {
	svc, store, _ := setupTestService(t, true)
	ctx := context.Background()

	// 1. Unfunded swap can be expired
	swap := domain.Swap{
		ID:        "swp_expire_1",
		State:     domain.StateAwaitingUserDeposit,
		ExpiresAt: testNow.Add(-time.Minute),
	}
	_, _, _ = store.CreateSwap(ctx, swap, "key_expire_1")

	expired, err := svc.ExpireSwap(ctx, swap.ID)
	if err != nil {
		t.Fatalf("ExpireSwap: %v", err)
	}
	if expired.State != domain.StateExpired {
		t.Fatalf("expected EXPIRED, got %s", expired.State)
	}

	// 2. Funded swap cannot be expired
	fundedSwap := domain.Swap{
		ID:           "swp_funded_exp",
		State:        domain.StateDepositSubmitted,
		SourceTxHash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	_, _, _ = store.CreateSwap(ctx, fundedSwap, "key_funded_exp")

	_, err = svc.ExpireSwap(ctx, fundedSwap.ID)
	if !domain.IsCode(err, domain.CodeInvalidSwapState) {
		t.Fatalf("expected INVALID_SWAP_STATE, got %v", err)
	}
}
