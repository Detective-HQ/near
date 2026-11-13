package httpapi

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"

	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/memstore"
	"universal-defi-intelligence/internal/swap/providers/alternative"
	"universal-defi-intelligence/internal/swap/service"
)

type testAPI struct {
	t       *testing.T
	handler http.Handler
}

func newTestAPI(t *testing.T, authRequired bool, opts RouterOptions) *testAPI {
	t.Helper()
	swaps, err := service.New(service.Config{
		Store:     memstore.New(),
		Provider:  alternative.New(),
		QuoteMode: config.QuoteModeDemo,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(HandlerOptions{
		Swaps:  swaps,
		Auth:   auth.NewManager(auth.Config{AuthRequired: authRequired, Registry: swaps.Registry()}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.RateLimitRPS == 0 {
		opts.RateLimitRPS, opts.RateLimitBurst = 1000, 1000
	}
	return &testAPI{t: t, handler: NewRouter(handler, slog.New(slog.NewTextHandler(io.Discard, nil)), opts)}
}

func (a *testAPI) do(method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	a.t.Helper()
	var reader io.Reader
	switch value := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(value)
	default:
		encoded, _ := json.Marshal(value)
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "203.0.113.10:5555"
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

type errEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if got := decode[errEnvelope](t, rec).Error.Code; got != code {
		t.Fatalf("code = %s, want %s", got, code)
	}
}

func TestHealthReadyAndStaticContracts(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})

	if rec := api.do("GET", "/healthz", nil, nil); rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
	if rec := api.do("GET", "/readyz", nil, nil); rec.Code != 200 {
		t.Fatalf("readyz = %d %s", rec.Code, rec.Body.String())
	}

	health := decode[map[string]any](t, api.do("GET", "/api/v1/health", nil, nil))
	if health["quoteMode"] != "demo" || health["demoMode"] != true || health["marketDataMode"] != "fixture" {
		t.Fatalf("health = %v", health)
	}

	chains := decode[struct {
		Chains []ChainDTO `json:"chains"`
	}](t, api.do("GET", "/api/v1/chains", nil, nil))
	if len(chains.Chains) < 10 || chains.Chains[0].Key == "" {
		t.Fatalf("chains = %+v", chains)
	}

	assets := decode[struct {
		Mode   string     `json:"mode"`
		Assets []AssetDTO `json:"assets"`
	}](t, api.do("GET", "/api/v1/assets?chain=eth", nil, nil))
	if assets.Mode != "fixture" || len(assets.Assets) != 2 || assets.Assets[0].Chain != "ethereum" || assets.Assets[0].PriceUSD == nil {
		t.Fatalf("assets (alias filter) = %+v", assets)
	}

	research := decode[ResearchDTO](t, api.do("GET", "/api/v1/research/scenarios", nil, nil))
	if len(research.Scenarios) != 3 || research.Scenarios[1].ProjectedAnnualFeesUSD < 3_626_219 || research.Scenarios[1].ProjectedAnnualFeesUSD > 3_626_220 {
		t.Fatalf("research = %+v", research)
	}

	if rec := api.do("GET", "/api/v1/markets/overview", nil, nil); rec.Code != 200 {
		t.Fatalf("markets = %d", rec.Code)
	}
	expectError(t, api.do("GET", "/api/v1/nope", nil, nil), 404, "NOT_FOUND")
	if rec := api.do("POST", "/api/v1/health", nil, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method = %d", rec.Code)
	}
}

func TestSimulationFlowMatchesFrontendContract(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})

	rec := api.do("POST", "/api/v1/quotes", map[string]any{
		"originAssetId": "demo:ethereum:eth", "destinationAssetId": "demo:base:usdc", "amountIn": "1.5",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("quote = %d %s", rec.Code, rec.Body.String())
	}
	quote := decode[struct {
		Quote SwapQuoteDTO `json:"quote"`
	}](t, rec).Quote
	if quote.Mode != "simulation" || quote.AmountOutEstimate != "5084.7" || quote.OriginAsset.Symbol != "ETH" || quote.MinimumAmountOut == nil {
		t.Fatalf("quote = %+v", quote)
	}

	key := map[string]string{"Idempotency-Key": "sim-key-000001"}
	rec = api.do("POST", "/api/v1/swaps", map[string]any{"quoteId": quote.ID}, key)
	created := decode[struct {
		Swap    SwapRecordDTO `json:"swap"`
		Created bool          `json:"created"`
	}](t, rec)
	if rec.Code != http.StatusCreated || !created.Created || created.Swap.State != "AWAITING_USER_DEPOSIT" || created.Swap.Mode != "simulation" {
		t.Fatalf("create = %d %+v", rec.Code, created)
	}
	replay := decode[struct {
		Created bool `json:"created"`
	}](t, api.do("POST", "/api/v1/swaps", map[string]any{"quoteId": quote.ID}, key))
	if replay.Created {
		t.Fatal("replay reported created=true")
	}
	expectError(t, api.do("POST", "/api/v1/swaps", map[string]any{"quoteId": quote.ID}, nil), 400, "IDEMPOTENCY_KEY_REQUIRED")

	id := created.Swap.ID
	for _, want := range []string{"DEPOSIT_SUBMITTED", "PROCESSING", "SUCCEEDED"} {
		rec = api.do("POST", "/api/v1/swaps/"+id+"/simulate", "{}", nil)
		swap := decode[struct {
			Swap SwapRecordDTO `json:"swap"`
		}](t, rec).Swap
		if swap.State != want || swap.RealSettlement {
			t.Fatalf("simulate -> %s: %d %+v", want, rec.Code, swap)
		}
	}
	// Audit bug: a completed swap could previously be cancelled.
	expectError(t, api.do("POST", "/api/v1/swaps/"+id+"/cancel", "{}", nil), http.StatusConflict, "INVALID_SWAP_STATE")
	// Audit bug: a deposit could previously reopen a terminal swap.
	expectError(t, api.do("POST", "/api/v1/swaps/"+id+"/deposit", map[string]any{
		"txHash": "0x" + strings.Repeat("ab", 32),
	}, nil), http.StatusConflict, "INVALID_SWAP_STATE")

	fetched := decode[struct {
		Swap SwapRecordDTO `json:"swap"`
	}](t, api.do("GET", "/api/v1/swaps/"+id, nil, nil)).Swap
	if len(fetched.Events) != 4 || fetched.Quote.ID != quote.ID {
		t.Fatalf("fetched = %+v", fetched)
	}

	activity := decode[struct {
		Swaps []SwapRecordDTO `json:"swaps"`
	}](t, api.do("GET", "/api/v1/activity", nil, nil))
	if len(activity.Swaps) != 1 || activity.Swaps[0].ID != id {
		t.Fatalf("activity = %+v", activity)
	}
	expectError(t, api.do("GET", "/api/v1/swaps/swp_missing", nil, nil), 404, "SWAP_NOT_FOUND")
}

func TestDepositValidationAndCancel(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})
	quote := decode[struct {
		Quote SwapQuoteDTO `json:"quote"`
	}](t, api.do("POST", "/api/v1/quotes", map[string]any{
		"originAssetId": "demo:ethereum:usdc", "destinationAssetId": "demo:base:usdc", "amountIn": "10",
	}, nil)).Quote
	swap := decode[struct {
		Swap SwapRecordDTO `json:"swap"`
	}](t, api.do("POST", "/api/v1/swaps", map[string]any{"quoteId": quote.ID}, map[string]string{"Idempotency-Key": "dep-key-000001"})).Swap

	expectError(t, api.do("POST", "/api/v1/swaps/"+swap.ID+"/deposit", map[string]any{"txHash": ""}, nil), 400, "INVALID_TRANSACTION_HASH")
	expectError(t, api.do("POST", "/api/v1/swaps/"+swap.ID+"/deposit", map[string]any{"txHash": "0xnothex"}, nil), 400, "INVALID_TRANSACTION_HASH")

	hash := "0x" + strings.Repeat("cd", 32)
	rec := api.do("POST", "/api/v1/swaps/"+swap.ID+"/deposit", map[string]any{"txHash": hash, "chainId": "1"}, nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("deposit = %d %s", rec.Code, rec.Body.String())
	}
	dup := decode[struct {
		Duplicate bool `json:"duplicate"`
	}](t, api.do("POST", "/api/v1/swaps/"+swap.ID+"/deposit", map[string]any{"txHash": hash}, nil))
	if !dup.Duplicate {
		t.Fatal("same hash replay not reported as duplicate")
	}
	expectError(t, api.do("POST", "/api/v1/swaps/"+swap.ID+"/cancel", "{}", nil), http.StatusConflict, "INVALID_SWAP_STATE")
}

func TestBodyLimitAndMalformedJSON(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{MaxBodyBytes: 1024})
	expectError(t, api.do("POST", "/api/v1/quotes", `{"amountIn":"`+strings.Repeat("9", 4096)+`"}`, nil), http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE")
	expectError(t, api.do("POST", "/api/v1/quotes", `{not json`, nil), 400, "INVALID_JSON")
}

func TestCORSAllowlistAndSecurityHeaders(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{CORSAllowedOrigins: []string{"https://app.example.com"}})

	rec := api.do("GET", "/api/v1/health", nil, map[string]string{"Origin": "https://app.example.com"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("allowed origin missing CORS headers: %v", rec.Header())
	}
	rec = api.do("GET", "/api/v1/health", nil, map[string]string{"Origin": "https://evil.example"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin received CORS headers")
	}
	rec = api.do("OPTIONS", "/api/v1/quotes", nil, map[string]string{"Origin": "https://evil.example", "Access-Control-Request-Method": "POST"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disallowed preflight = %d", rec.Code)
	}
	for _, header := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy"} {
		if rec.Header().Get(header) == "" {
			t.Fatalf("missing %s", header)
		}
	}
}

func TestRequestIDIsSanitized(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})
	rec := api.do("GET", "/api/v1/health", nil, map[string]string{"X-Request-Id": "ok-id_123"})
	if rec.Header().Get("X-Request-Id") != "ok-id_123" {
		t.Fatal("valid request id not echoed")
	}
	rec = api.do("GET", "/api/v1/health", nil, map[string]string{"X-Request-Id": "bad id\nInjected: yes"})
	if got := rec.Header().Get("X-Request-Id"); strings.Contains(got, " ") || got == "" {
		t.Fatalf("unsafe request id echoed: %q", got)
	}
}

func TestRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{RateLimitRPS: 1, RateLimitBurst: 2})
	limited := false
	for i := 0; i < 5; i++ {
		// A direct (untrusted) client rotating X-Forwarded-For must still be limited.
		rec := api.do("GET", "/api/v1/health", nil, map[string]string{"X-Forwarded-For": "198.51.100." + string(rune('1'+i))})
		if rec.Code == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatal("spoofed X-Forwarded-For bypassed the rate limiter")
	}
}

func TestTrustedProxyForwardedForIsHonored(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}
	api := newTestAPI(t, false, RouterOptions{RateLimitRPS: 1, RateLimitBurst: 1, TrustedProxies: trusted})
	for i := 0; i < 4; i++ {
		rec := api.do("GET", "/api/v1/health", nil, map[string]string{"X-Forwarded-For": "198.51.100." + string(rune('1'+i))})
		if rec.Code != 200 {
			t.Fatalf("distinct clients behind a trusted proxy were limited together (request %d)", i)
		}
	}
}

func signEVM(t *testing.T, key *secp256k1.PrivateKey, message string) string {
	t.Helper()
	compact := ecdsa.SignCompact(key, auth.PersonalSignDigest([]byte(message)), false)
	sig := append(append([]byte{}, compact[1:]...), compact[0])
	return "0x" + hex.EncodeToString(sig)
}

func TestAuthRequiredFlowAndOwnership(t *testing.T) {
	api := newTestAPI(t, true, RouterOptions{})

	expectError(t, api.do("GET", "/api/v1/activity", nil, nil), http.StatusUnauthorized, "UNAUTHORIZED")
	expectError(t, api.do("POST", "/api/v1/quotes", map[string]any{}, nil), http.StatusUnauthorized, "UNAUTHORIZED")

	login := func() string {
		key, _ := secp256k1.GeneratePrivateKey()
		hash := sha3.NewLegacyKeccak256()
		hash.Write(key.PubKey().SerializeUncompressed()[1:])
		address, _ := evm.Checksum(hash.Sum(nil)[12:])
		nonce := decode[map[string]string](t, api.do("POST", "/api/v1/auth/nonce", "{}", nil))
		rec := api.do("POST", "/api/v1/auth/verify", map[string]any{
			"chain": "ethereum", "walletAddress": address, "nonce": nonce["nonce"],
			"message": nonce["message"], "signature": signEVM(t, key, nonce["message"]),
		}, nil)
		if rec.Code != 200 {
			t.Fatalf("verify = %d %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Set-Cookie"), "HttpOnly") {
			t.Fatal("session cookie is not HttpOnly")
		}
		return decode[map[string]any](t, rec)["token"].(string)
	}
	owner, intruder := login(), login()
	bearer := func(token string) map[string]string { return map[string]string{"Authorization": "Bearer " + token} }

	quote := decode[struct {
		Quote SwapQuoteDTO `json:"quote"`
	}](t, api.do("POST", "/api/v1/quotes", map[string]any{
		"originAssetId": "demo:near:near", "destinationAssetId": "demo:base:usdc", "amountIn": "3",
	}, bearer(owner))).Quote
	headers := bearer(owner)
	headers["Idempotency-Key"] = "auth-key-000001"
	swap := decode[struct {
		Swap SwapRecordDTO `json:"swap"`
	}](t, api.do("POST", "/api/v1/swaps", map[string]any{"quoteId": quote.ID}, headers)).Swap

	expectError(t, api.do("GET", "/api/v1/swaps/"+swap.ID, nil, bearer(intruder)), http.StatusForbidden, "FORBIDDEN")
	expectError(t, api.do("POST", "/api/v1/swaps/"+swap.ID+"/cancel", "{}", bearer(intruder)), http.StatusForbidden, "FORBIDDEN")
	intruderActivity := decode[struct {
		Swaps []SwapRecordDTO `json:"swaps"`
	}](t, api.do("GET", "/api/v1/activity", nil, bearer(intruder)))
	if len(intruderActivity.Swaps) != 0 {
		t.Fatal("another wallet's swaps leaked into activity")
	}
	if rec := api.do("GET", "/api/v1/swaps/"+swap.ID, nil, bearer(owner)); rec.Code != 200 {
		t.Fatalf("owner read = %d", rec.Code)
	}

	// Forged signature: valid shape, wrong signer.
	other, _ := secp256k1.GeneratePrivateKey()
	nonce := decode[map[string]string](t, api.do("POST", "/api/v1/auth/nonce", "{}", nil))
	expectError(t, api.do("POST", "/api/v1/auth/verify", map[string]any{
		"chain": "ethereum", "walletAddress": "0x1111111111111111111111111111111111111111",
		"nonce": nonce["nonce"], "message": nonce["message"] + " 0x1111111111111111111111111111111111111111",
		"signature": signEVM(t, other, nonce["message"]+" 0x1111111111111111111111111111111111111111"),
	}, nil), http.StatusUnauthorized, "UNAUTHORIZED")

	if rec := api.do("POST", "/api/v1/auth/logout", "{}", bearer(owner)); rec.Code != 200 {
		t.Fatalf("logout = %d", rec.Code)
	}
	expectError(t, api.do("GET", "/api/v1/activity", nil, bearer(owner)), http.StatusUnauthorized, "UNAUTHORIZED")
}

func TestUnknownErrorsAreNotLeaked(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})
	rec := api.do("POST", "/api/v1/quotes", map[string]any{
		"originAssetId": "demo:ethereum:eth", "destinationAssetId": "missing", "amountIn": "1",
	}, nil)
	expectError(t, rec, domain.CodeUnknownAsset.HTTPStatus(), "UNKNOWN_ASSET")
}

func TestLargeResponsesAreGzipped(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})
	rec := api.do("GET", "/api/v1/chains", nil, map[string]string{"Accept-Encoding": "gzip"})
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("chains (>1KB) not compressed: %v", rec.Header())
	}
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Chains []ChainDTO `json:"chains"`
	}
	if err := json.NewDecoder(reader).Decode(&body); err != nil || len(body.Chains) == 0 {
		t.Fatalf("decode gzipped body: %v", err)
	}
	small := api.do("GET", "/healthz", nil, map[string]string{"Accept-Encoding": "gzip"})
	if small.Header().Get("Content-Encoding") != "" {
		t.Fatal("tiny response was compressed")
	}
}
