package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/portfolio"
)

const portfolioAddr = "0x1111111111111111111111111111111111111111"

// newPortfolioAPI serves the router with a portfolio reader whose EVM chains
// point at a fake JSON-RPC node (1 native unit, 200 USDC on every EVM chain).
func newPortfolioAPI(t *testing.T) *testAPI {
	t.Helper()
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		result := "0xde0b6b3a7640000"
		if req.Method == "eth_call" {
			result = "0x" + strings.Repeat("0", 56) + "0bebc200"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(node.Close)

	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	endpoints := map[string][]string{}
	for _, c := range reg.ByFamily(registry.FamilyEVM) {
		endpoints[c.Key] = []string{node.URL}
	}
	swaps := mustDemoService(t)
	h, err := NewHandler(HandlerOptions{
		Swaps:     swaps,
		Auth:      auth.NewManager(auth.Config{Registry: swaps.Registry()}),
		Portfolio: portfolio.NewReader(portfolio.Config{Registry: reg, Endpoints: endpoints}),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &testAPI{t: t, handler: NewRouter(h, slog.New(slog.NewTextHandler(io.Discard, nil)),
		RouterOptions{RateLimitRPS: 1e6, RateLimitBurst: 1 << 20})}
}

func TestPortfolioEmptyAddress(t *testing.T) {
	api := newPortfolioAPI(t)
	rec := api.do("GET", "/api/v1/portfolio", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	res := decode[PortfolioResponseDTO](t, rec)
	if len(res.Balances) != 0 || res.TotalValueUSD != nil || res.Quality != "unavailable" {
		t.Fatalf("empty address must not invent balances: %+v", res)
	}
}

func TestPortfolioReadsOnChainBalances(t *testing.T) {
	api := newPortfolioAPI(t)
	rec := api.do("GET", "/api/v1/portfolio?address="+portfolioAddr+"&chain=base", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	res := decode[PortfolioResponseDTO](t, rec)
	if res.Address == nil || *res.Address != portfolioAddr || res.Quality != "live" {
		t.Fatalf("unexpected envelope: %+v", res)
	}
	got := map[string]string{}
	for _, b := range res.Balances {
		if b.Chain != "base" {
			t.Fatalf("chain filter leaked %s", b.Chain)
		}
		if b.ValueUSD != nil {
			t.Fatal("no price source configured: values must be null, not invented")
		}
		got[b.Symbol] = b.Balance
	}
	if got["ETH"] != "1" || got["USDC"] != "200" {
		t.Fatalf("balances = %v", got)
	}
	if res.TotalValueUSD != nil {
		t.Fatalf("total must be null without prices, got %v", *res.TotalValueUSD)
	}
}

func TestPortfolioScansEveryEVMChainForAnEVMAddress(t *testing.T) {
	api := newPortfolioAPI(t)
	res := decode[PortfolioResponseDTO](t, api.do("GET", "/api/v1/portfolio?address="+portfolioAddr, nil, nil))
	chains := map[string]bool{}
	for _, b := range res.Balances {
		chains[b.Chain] = true
	}
	for _, want := range []string{"ethereum", "base", "arbitrum", "polygon", "avalanche", "bnb", "celo"} {
		if !chains[want] {
			t.Fatalf("missing %s in %v", want, chains)
		}
	}
}

func TestPortfolioRejectsInvalidAddress(t *testing.T) {
	api := newPortfolioAPI(t)
	expectError(t, api.do("GET", "/api/v1/portfolio?address=not-an-address!", nil, nil), http.StatusBadRequest, "VALIDATION_FAILED")
	expectError(t, api.do("GET", "/api/v1/portfolio?address="+portfolioAddr+"&chain=solana", nil, nil), http.StatusBadRequest, "VALIDATION_FAILED")
}
