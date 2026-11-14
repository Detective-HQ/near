package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"universal-defi-intelligence/internal/chains/registry"
)

// rpcServer answers JSON-RPC calls with handler(method, params).
func rpcServer(t *testing.T, handler func(method string, params json.RawMessage) (any, *map[string]any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     uint64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		result, rpcErr := handler(req.Method, req.Params)
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			resp["error"] = *rpcErr
		} else {
			resp["result"] = result
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

type fixedPrices map[string]float64

func (f fixedPrices) Current(context.Context, []string) (map[string]float64, error) { return f, nil }

func newReader(t *testing.T, endpoints map[string][]string, prices PriceSource) *Reader {
	t.Helper()
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	return NewReader(Config{Registry: reg, Endpoints: endpoints, Prices: prices})
}

const evmAddr = "0x1111111111111111111111111111111111111111"

func TestEVMNativeAndUSDCWithPrices(t *testing.T) {
	srv := rpcServer(t, func(method string, params json.RawMessage) (any, *map[string]any) {
		switch method {
		case "eth_getBalance":
			return "0xde0b6b3a7640000", nil // 1 ETH
		case "eth_call":
			return "0x" + strings.Repeat("0", 56) + "0bebc200", nil // 200 USDC (6 decimals)
		}
		return nil, &map[string]any{"code": -32601, "message": "method not found"}
	})
	defer srv.Close()
	r := newReader(t, map[string][]string{"base": {srv.URL}}, fixedPrices{"coingecko:ethereum": 2000, "coingecko:usd-coin": 1})

	res, err := r.Read(context.Background(), evmAddr, "base")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Holdings) != 2 || res.Quality != "live" || !res.Priced {
		t.Fatalf("result = %+v", res)
	}
	got := map[string]string{}
	for _, h := range res.Holdings {
		got[h.Symbol] = h.Balance()
		if h.ValueUSD == nil {
			t.Fatalf("%s not priced", h.Symbol)
		}
	}
	if got["ETH"] != "1" || got["USDC"] != "200" {
		t.Fatalf("balances = %v", got)
	}
	if *res.Holdings[0].ValueUSD != 2000 {
		t.Fatalf("sorted by value: first = %+v", res.Holdings[0])
	}
}

func TestEVMAddressScansAllEVMChainsAndReportsPartial(t *testing.T) {
	var calls atomic.Int32
	ok := rpcServer(t, func(method string, _ json.RawMessage) (any, *map[string]any) {
		calls.Add(1)
		return "0x0", nil
	})
	defer ok.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer down.Close()

	reg, _ := registry.Load()
	endpoints := map[string][]string{}
	for _, c := range reg.ByFamily(registry.FamilyEVM) {
		endpoints[c.Key] = []string{ok.URL}
	}
	endpoints["polygon"] = []string{down.URL}
	r := newReader(t, endpoints, nil)

	res, err := r.Read(context.Background(), evmAddr, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Quality != "partial" || len(res.Failed) != 1 || res.Failed[0] != "polygon" || res.Priced {
		t.Fatalf("result quality=%s failed=%v priced=%v", res.Quality, res.Failed, res.Priced)
	}
	for _, h := range res.Holdings {
		if h.ValueUSD != nil {
			t.Fatal("no price source configured, values must be null")
		}
	}

	// Cached: a second read within the TTL makes no RPC calls.
	before := calls.Load()
	if _, err := r.Read(context.Background(), evmAddr, ""); err != nil || calls.Load() != before {
		t.Fatalf("cache miss: err=%v calls %d -> %d", err, before, calls.Load())
	}
}

func TestNEARMissingAccountIsZeroAndSolanaLamports(t *testing.T) {
	nearSrv := rpcServer(t, func(string, json.RawMessage) (any, *map[string]any) {
		return nil, &map[string]any{"code": -32000, "message": "Server error", "data": "account nobody.near does not exist while viewing"}
	})
	defer nearSrv.Close()
	solSrv := rpcServer(t, func(string, json.RawMessage) (any, *map[string]any) {
		return map[string]any{"value": 2_500_000_000}, nil
	})
	defer solSrv.Close()
	r := newReader(t, map[string][]string{"near": {nearSrv.URL}, "solana": {solSrv.URL}}, nil)

	res, err := r.Read(context.Background(), "nobody.near", "")
	if err != nil || len(res.Holdings) != 1 || res.Holdings[0].Balance() != "0" {
		t.Fatalf("near = %+v err=%v", res, err)
	}
	res, err = r.Read(context.Background(), "11111111111111111111111111111111", "solana")
	if err != nil || res.Holdings[0].Balance() != "2.5" {
		t.Fatalf("solana = %+v err=%v", res, err)
	}
}

func TestStellarAndAptosREST(t *testing.T) {
	horizon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"balances":[{"asset_type":"credit_alphanum4","balance":"9"},{"asset_type":"native","balance":"12.5000000"}]}`))
	}))
	defer horizon.Close()
	aptosNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/balance/0x1::aptos_coin::AptosCoin") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`150000000`))
	}))
	defer aptosNode.Close()
	r := newReader(t, map[string][]string{"stellar": {horizon.URL}, "aptos": {aptosNode.URL}}, nil)

	res, err := r.Read(context.Background(), "GAAZI4TCR3TY5OJHCTJC2A4QSY6CJWJH5IAJTGKIN2ER7LBNVKOCCWN7", "")
	if err != nil || res.Holdings[0].Balance() != "12.5" {
		t.Fatalf("stellar = %+v err=%v", res, err)
	}
	res, err = r.Read(context.Background(), "0x"+strings.Repeat("a", 64), "aptos")
	if err != nil || res.Holdings[0].Balance() != "1.5" {
		t.Fatalf("aptos = %+v err=%v", res, err)
	}
}

func TestInvalidAddressAndTotalOutage(t *testing.T) {
	r := newReader(t, nil, nil)
	if _, err := r.Read(context.Background(), "not an address!", ""); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("invalid err = %v", err)
	}
	if _, err := r.Read(context.Background(), evmAddr, "solana"); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("wrong-chain err = %v", err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer down.Close()
	r = newReader(t, map[string][]string{"base": {down.URL}}, nil)
	if _, err := r.Read(context.Background(), evmAddr, "base"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("outage err = %v", err)
	}
}
