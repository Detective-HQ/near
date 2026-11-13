package service

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/memstore"
	"universal-defi-intelligence/internal/swap/providers"
)

const (
	evmRecipient = "0x1111111111111111111111111111111111111111"
	evmRefund    = "0x2222222222222222222222222222222222222222"
	evmTxHash    = "0x3333333333333333333333333333333333333333333333333333333333333333"
	evmTxHash2   = "0x4444444444444444444444444444444444444444444444444444444444444444"
)

var testNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

type mockProvider struct {
	executableCalls atomic.Int32
	submitCalls     atomic.Int32
	tokens          []domain.Asset
	dryErr          error
}

func (m *mockProvider) Name() string                         { return "mock_provider" }
func (m *mockProvider) Capabilities() providers.Capabilities { return providers.Capabilities{} }
func (m *mockProvider) Health() providers.Health             { return providers.Health{Configured: true} }
func (m *mockProvider) GetTokens(context.Context) ([]domain.Asset, error) {
	return m.tokens, nil
}

func (m *mockProvider) GetDryQuote(_ context.Context, req providers.DryQuoteRequest) (providers.QuoteResult, error) {
	if m.dryErr != nil {
		return providers.QuoteResult{}, m.dryErr
	}
	out := new(big.Int).Mul(req.AmountAtomic, big.NewInt(2))
	return providers.QuoteResult{Quote: domain.Quote{
		ProviderName:            "mock_provider",
		AmountOutEstimateAtomic: out,
		MinAmountOutAtomic:      out,
		FeeBPS:                  7,
	}}, nil
}

func (m *mockProvider) GetExecutableQuote(_ context.Context, req providers.ExecutableQuoteRequest) (providers.QuoteResult, error) {
	m.executableCalls.Add(1)
	if req.Recipient == "" || req.RefundTo == "" {
		return providers.QuoteResult{}, errors.New("addresses missing")
	}
	return providers.QuoteResult{Quote: domain.Quote{
		ProviderName:            "mock_provider",
		AmountOutEstimateAtomic: big.NewInt(100),
		MinAmountOutAtomic:      big.NewInt(99),
		DepositAddress:          "0x9999999999999999999999999999999999999999",
		ExpiresAt:               req.Deadline,
	}}, nil
}

func (m *mockProvider) GetStatus(context.Context, providers.StatusRequest) (providers.StatusResult, error) {
	return providers.StatusResult{}, nil
}

func (m *mockProvider) SubmitDepositTx(context.Context, providers.DepositRequest) (providers.StatusResult, error) {
	m.submitCalls.Add(1)
	return providers.StatusResult{}, nil
}

func liveAssets() []domain.Asset {
	return []domain.Asset{
		{ID: "nep141:base-usdc", ProviderAssetID: "nep141:base-usdc", Chain: "base", Symbol: "USDC", Decimals: 6, Enabled: true},
		// The provider names Ethereum "eth"; the registry key is "ethereum".
		{ID: "nep141:eth-usdc", ProviderAssetID: "nep141:eth-usdc", Chain: "eth", Symbol: "USDC", Decimals: 6, Enabled: true},
		{ID: "nep141:wrap.near", ProviderAssetID: "nep141:wrap.near", Chain: "near", Symbol: "NEAR", Decimals: 24, Enabled: true},
	}
}

func newLiveService(t *testing.T, execution bool) (*Service, *memstore.Store, *mockProvider) {
	t.Helper()
	store := memstore.New()
	for _, asset := range liveAssets() {
		store.PutAsset(asset)
	}
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	prov := &mockProvider{}
	svc, err := New(Config{
		Store: store, Provider: prov, Registry: reg,
		QuoteMode: config.QuoteModeLive, EnableLiveExecution: execution,
		QuoteDeadline: 3 * time.Minute, Now: func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, prov
}

func newDemoService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(Config{
		Store: memstore.New(), Provider: &mockProvider{},
		QuoteMode: config.QuoteModeDemo, Now: func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func liveDryQuote(t *testing.T, svc *Service, session string) domain.Quote {
	t.Helper()
	quote, err := svc.DryQuote(context.Background(), DryQuoteParams{
		OriginAssetID: "nep141:eth-usdc", DestinationAssetID: "nep141:base-usdc",
		AmountIn: "12.5", Recipient: evmRecipient, RefundTo: evmRefund, SessionID: session,
	})
	if err != nil {
		t.Fatalf("DryQuote: %v", err)
	}
	return quote
}

func TestDryQuoteValidation(t *testing.T) {
	svc, _, _ := newLiveService(t, false)
	ctx := context.Background()
	cases := []struct {
		name   string
		params DryQuoteParams
		code   domain.Code
	}{
		{"same asset", DryQuoteParams{OriginAssetID: "nep141:base-usdc", DestinationAssetID: "nep141:base-usdc", AmountIn: "1"}, domain.CodeInvalidAssetPair},
		{"bad amount", DryQuoteParams{OriginAssetID: "nep141:eth-usdc", DestinationAssetID: "nep141:base-usdc", AmountIn: "1e5", Recipient: evmRecipient, RefundTo: evmRefund}, domain.CodeInvalidAmount},
		{"too many decimals", DryQuoteParams{OriginAssetID: "nep141:eth-usdc", DestinationAssetID: "nep141:base-usdc", AmountIn: "1.0000001", Recipient: evmRecipient, RefundTo: evmRefund}, domain.CodeInvalidAmount},
		{"missing recipient", DryQuoteParams{OriginAssetID: "nep141:eth-usdc", DestinationAssetID: "nep141:base-usdc", AmountIn: "1", RefundTo: evmRefund}, domain.CodeRecipientRequired},
		{"bad recipient", DryQuoteParams{OriginAssetID: "nep141:eth-usdc", DestinationAssetID: "nep141:base-usdc", AmountIn: "1", Recipient: "nope", RefundTo: evmRefund}, domain.CodeInvalidRecipient},
		{"unknown asset", DryQuoteParams{OriginAssetID: "nep141:missing", DestinationAssetID: "nep141:base-usdc", AmountIn: "1"}, domain.CodeUnknownAsset},
		{"slippage", DryQuoteParams{OriginAssetID: "nep141:eth-usdc", DestinationAssetID: "nep141:base-usdc", AmountIn: "1", SlippageBPS: 9000}, domain.CodeInvalidSlippage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.DryQuote(ctx, tc.params); !domain.IsCode(err, tc.code) {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
}

func TestDryQuoteKeepsProviderFeeAndAddresses(t *testing.T) {
	svc, _, _ := newLiveService(t, false)
	quote := liveDryQuote(t, svc, "")
	if quote.FeeBPS != 7 {
		t.Fatalf("FeeBPS = %d, want provider fee 7 (not slippage)", quote.FeeBPS)
	}
	if quote.SlippageBPS != defaultSlippageBPS || quote.Mode != domain.QuoteModeProviderDry {
		t.Fatalf("unexpected quote %+v", quote)
	}
	if quote.Recipient != evmRecipient || quote.RefundTo != evmRefund {
		t.Fatalf("reviewed addresses not stored: %q %q", quote.Recipient, quote.RefundTo)
	}
}

func TestCreateSwapExecutionGateDisabled(t *testing.T) {
	svc, _, _ := newLiveService(t, false)
	quote := liveDryQuote(t, svc, "")
	_, err := svc.CreateSwap(context.Background(), CreateSwapParams{QuoteID: quote.ID, IdempotencyKey: "idem-key-0001"})
	if !domain.IsCode(err, domain.CodeLiveExecutionDisabled) {
		t.Fatalf("err = %v, want LIVE_EXECUTION_DISABLED", err)
	}
}

func TestCreateSwapUsesReviewedAddressesAndChainAliases(t *testing.T) {
	svc, _, prov := newLiveService(t, true)
	quote := liveDryQuote(t, svc, "")
	result, err := svc.CreateSwap(context.Background(), CreateSwapParams{QuoteID: quote.ID, IdempotencyKey: "idem-key-0002"})
	if err != nil {
		t.Fatalf("CreateSwap: %v", err)
	}
	swap := result.Swap
	if swap.RecipientAddress != evmRecipient || swap.RefundAddress != evmRefund {
		t.Fatalf("swap addresses = %q/%q", swap.RecipientAddress, swap.RefundAddress)
	}
	if swap.OriginChain != "ethereum" || swap.DestinationChain != "base" {
		t.Fatalf("chains = %s -> %s, want canonical registry keys", swap.OriginChain, swap.DestinationChain)
	}
	if swap.State != domain.StateAwaitingUserDeposit || swap.Simulated || prov.executableCalls.Load() != 1 {
		t.Fatalf("unexpected swap %+v calls=%d", swap, prov.executableCalls.Load())
	}

	// Changing the reviewed recipient must not be possible.
	quote2 := liveDryQuote(t, svc, "")
	if _, err := svc.CreateSwap(context.Background(), CreateSwapParams{
		QuoteID: quote2.ID, IdempotencyKey: "idem-key-0003", Recipient: evmRefund,
	}); !domain.IsCode(err, domain.CodeInvalidRecipient) {
		t.Fatalf("changed recipient err = %v", err)
	}
}

func TestCreateSwapReplayDoesNotCallProviderAgain(t *testing.T) {
	svc, _, prov := newLiveService(t, true)
	quote := liveDryQuote(t, svc, "")
	params := CreateSwapParams{QuoteID: quote.ID, IdempotencyKey: "idem-key-0004"}
	first, err := svc.CreateSwap(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.CreateSwap(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Swap.ID != first.Swap.ID {
		t.Fatalf("replay = %+v", replay)
	}
	if calls := prov.executableCalls.Load(); calls != 1 {
		t.Fatalf("executable quote calls = %d, want 1 (no new deposit address on replay)", calls)
	}
	params.Recipient = evmRecipient
	if _, err := svc.CreateSwap(context.Background(), params); !domain.IsCode(err, domain.CodeIdempotencyConflict) {
		t.Fatalf("changed payload err = %v", err)
	}
}

func TestCreateSwapRejectsUnsupportedRoute(t *testing.T) {
	svc, _, _ := newLiveService(t, true)
	quote, err := svc.DryQuote(context.Background(), DryQuoteParams{
		OriginAssetID: "nep141:wrap.near", DestinationAssetID: "nep141:base-usdc",
		AmountIn: "1", Recipient: evmRecipient, RefundTo: "alice.near",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSwap(context.Background(), CreateSwapParams{QuoteID: quote.ID, IdempotencyKey: "idem-key-0005"}); !domain.IsCode(err, domain.CodeNoRoute) {
		t.Fatalf("err = %v, want NO_ROUTE", err)
	}
}

func TestCreateSwapRejectsExpiredAndForeignQuote(t *testing.T) {
	svc, _, _ := newLiveService(t, true)
	owned := liveDryQuote(t, svc, "evm:0xowner")
	if _, err := svc.CreateSwap(context.Background(), CreateSwapParams{QuoteID: owned.ID, IdempotencyKey: "idem-key-0006", SessionID: "evm:0xother"}); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("foreign quote err = %v", err)
	}
	svc.now = func() time.Time { return testNow.Add(time.Hour) }
	if _, err := svc.CreateSwap(context.Background(), CreateSwapParams{QuoteID: owned.ID, IdempotencyKey: "idem-key-0007", SessionID: "evm:0xowner"}); !domain.IsCode(err, domain.CodeQuoteExpired) {
		t.Fatalf("expired quote err = %v", err)
	}
}

func createLiveSwap(t *testing.T, svc *Service, session, key string) domain.Swap {
	t.Helper()
	quote := liveDryQuote(t, svc, session)
	result, err := svc.CreateSwap(context.Background(), CreateSwapParams{QuoteID: quote.ID, IdempotencyKey: key, SessionID: session})
	if err != nil {
		t.Fatal(err)
	}
	return result.Swap
}

func TestRecordDepositAtomicDuplicateAndConflicts(t *testing.T) {
	svc, _, prov := newLiveService(t, true)
	ctx := context.Background()
	swap := createLiveSwap(t, svc, "", "idem-key-0010")

	res, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: swap.ID, TxHash: evmTxHash, ChainID: "1"})
	if err != nil {
		t.Fatalf("RecordDeposit: %v", err)
	}
	if res.Duplicate || res.Swap.State != domain.StateDepositSubmitted || res.Swap.SourceTxHash != evmTxHash {
		t.Fatalf("unexpected result %+v", res)
	}
	_ = svc.Wait(ctx)
	if prov.submitCalls.Load() != 1 {
		t.Fatalf("provider notified %d times", prov.submitCalls.Load())
	}

	dup, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: swap.ID, TxHash: evmTxHash})
	if err != nil || !dup.Duplicate {
		t.Fatalf("duplicate = %+v, %v", dup, err)
	}
	if _, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: swap.ID, TxHash: evmTxHash2}); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("different hash err = %v", err)
	}

	// The same on-chain transaction cannot fund a second swap.
	other := createLiveSwap(t, svc, "", "idem-key-0011")
	if _, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: other.ID, TxHash: evmTxHash}); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("reused hash err = %v", err)
	}
	if _, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: other.ID, TxHash: evmTxHash2, ChainID: "8453"}); !domain.IsCode(err, domain.CodeWrongNetwork) {
		t.Fatalf("wrong network err = %v", err)
	}
	if _, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: other.ID, TxHash: "not-a-hash"}); !domain.IsCode(err, domain.CodeInvalidTxHash) {
		t.Fatalf("invalid hash err = %v", err)
	}
}

func TestOwnershipIsEnforcedOnEveryMutation(t *testing.T) {
	svc, _, _ := newLiveService(t, true)
	ctx := context.Background()
	swap := createLiveSwap(t, svc, "evm:0xowner", "idem-key-0020")

	if _, err := svc.CancelSwap(ctx, swap.ID, ""); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("anonymous cancel err = %v", err)
	}
	if _, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: swap.ID, TxHash: evmTxHash, SessionID: "evm:0xother"}); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("foreign deposit err = %v", err)
	}
	if _, err := svc.GetSwapView(ctx, swap.ID, ""); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("anonymous read err = %v", err)
	}
	if _, err := svc.CancelSwap(ctx, swap.ID, "evm:0xowner"); err != nil {
		t.Fatalf("owner cancel: %v", err)
	}
}

func TestCancelRules(t *testing.T) {
	svc, _, _ := newLiveService(t, true)
	ctx := context.Background()

	funded := createLiveSwap(t, svc, "", "idem-key-0030")
	if _, err := svc.RecordDeposit(ctx, RecordDepositParams{SwapID: funded.ID, TxHash: evmTxHash}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelSwap(ctx, funded.ID, ""); !domain.IsCode(err, domain.CodeInvalidSwapState) {
		t.Fatalf("funded cancel err = %v", err)
	}

	unfunded := createLiveSwap(t, svc, "", "idem-key-0031")
	cancelled, err := svc.CancelSwap(ctx, unfunded.ID, "")
	if err != nil || cancelled.State != domain.StateCancelled {
		t.Fatalf("cancel = %+v, %v", cancelled, err)
	}
	again, err := svc.CancelSwap(ctx, unfunded.ID, "")
	if err != nil || again.State != domain.StateCancelled {
		t.Fatalf("idempotent cancel = %+v, %v", again, err)
	}

	late := createLiveSwap(t, svc, "", "idem-key-0032")
	svc.now = func() time.Time { return testNow.Add(time.Hour) }
	expired, err := svc.CancelSwap(ctx, late.ID, "")
	if err != nil || expired.State != domain.StateExpired {
		t.Fatalf("late cancel = %+v, %v", expired, err)
	}
}

func TestDemoSimulationLifecycleIsNeverRealSettlement(t *testing.T) {
	svc := newDemoService(t)
	ctx := context.Background()

	quote, err := svc.DryQuote(ctx, DryQuoteParams{OriginAssetID: "demo:ethereum:eth", DestinationAssetID: "demo:base:usdc", AmountIn: "1"})
	if err != nil {
		t.Fatalf("demo quote without addresses: %v", err)
	}
	// 1 ETH at $3400 -> 3400 USDC minus 30 bps = 3389.8 USDC.
	if got := domain.FormatUnits(quote.AmountOutEstimateAtomic, 6); got != "3389.8" {
		t.Fatalf("demo estimate = %s, want 3389.8", got)
	}

	created, err := svc.CreateSwap(ctx, CreateSwapParams{QuoteID: quote.ID, IdempotencyKey: "idem-sim-0001"})
	if err != nil {
		t.Fatal(err)
	}
	swap := created.Swap
	if !swap.Simulated || swap.State != domain.StateAwaitingUserDeposit || swap.OriginChain != "ethereum" {
		t.Fatalf("simulated swap = %+v", swap)
	}
	for _, want := range []domain.State{domain.StateDepositSubmitted, domain.StateProcessing, domain.StateSucceeded} {
		swap, err = svc.AdvanceSimulation(ctx, swap.ID, "")
		if err != nil || swap.State != want {
			t.Fatalf("advance -> %s: %+v, %v", want, swap, err)
		}
	}
	if swap.RealSettlement {
		t.Fatal("a simulation must never be a real settlement")
	}
	if _, err := svc.AdvanceSimulation(ctx, swap.ID, ""); !domain.IsCode(err, domain.CodeInvalidSwapState) {
		t.Fatalf("advance past terminal err = %v", err)
	}
	if _, err := svc.CancelSwap(ctx, swap.ID, ""); !domain.IsCode(err, domain.CodeInvalidSwapState) {
		t.Fatalf("cancel after success err = %v", err)
	}

	view, err := svc.GetSwapView(ctx, swap.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Events) != 4 || view.Quote.ID != quote.ID {
		t.Fatalf("view events=%d quote=%s", len(view.Events), view.Quote.ID)
	}
}

func TestAdvanceSimulationRejectsProviderSwaps(t *testing.T) {
	svc, _, _ := newLiveService(t, true)
	swap := createLiveSwap(t, svc, "", "idem-key-0040")
	if _, err := svc.AdvanceSimulation(context.Background(), swap.ID, ""); !domain.IsCode(err, domain.CodeInvalidSwapState) {
		t.Fatalf("err = %v", err)
	}
}

func TestListActivityScopesBySession(t *testing.T) {
	svc, _, _ := newLiveService(t, true)
	ctx := context.Background()
	createLiveSwap(t, svc, "", "idem-key-0050")
	createLiveSwap(t, svc, "evm:0xowner", "idem-key-0051")

	anon, _, err := svc.ListActivity(ctx, "", 10, "")
	if err != nil || len(anon) != 1 || anon[0].Swap.SessionID != "" {
		t.Fatalf("anonymous activity = %d, %v", len(anon), err)
	}
	owned, _, err := svc.ListActivity(ctx, "evm:0xowner", 10, "")
	if err != nil || len(owned) != 1 || owned[0].Swap.SessionID != "evm:0xowner" {
		t.Fatalf("owned activity = %d, %v", len(owned), err)
	}
}

func TestCatalogueFallsBackToProviderAndNormalizesChains(t *testing.T) {
	reg, _ := registry.Load()
	prov := &mockProvider{tokens: liveAssets()}
	store := memstore.New()
	svc, err := New(Config{Store: store, Provider: prov, Registry: reg, QuoteMode: config.QuoteModeLive})
	if err != nil {
		t.Fatal(err)
	}
	assets, info, err := svc.ListAssets(context.Background(), "eth")
	if err != nil || len(assets) != 1 || assets[0].Chain != "ethereum" || info.Source != "provider_catalogue" {
		t.Fatalf("assets = %+v info=%+v err=%v", assets, info, err)
	}
	persisted, _ := store.ListAssets(context.Background(), "")
	if len(persisted) != 3 {
		t.Fatalf("cold catalogue was not persisted: %d", len(persisted))
	}
}

func TestCanonicalPayloadHashIsStable(t *testing.T) {
	a, _ := CanonicalPayloadHash(CreateSwapParams{QuoteID: "q", IdempotencyKey: "k"})
	b, _ := CanonicalPayloadHash(CreateSwapParams{QuoteID: "q", IdempotencyKey: "k"})
	c, _ := CanonicalPayloadHash(CreateSwapParams{QuoteID: "q2", IdempotencyKey: "k"})
	if a != b || a == c || len(a) != 64 {
		t.Fatalf("hashes %s %s %s", a, b, c)
	}
}
