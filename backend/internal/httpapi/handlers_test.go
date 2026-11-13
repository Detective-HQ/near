package httpapi

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/encoding/base58"
	"universal-defi-intelligence/internal/risk"
)

func testRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewStateStore()
	rEngine := risk.NewEngine(risk.Config{})
	aMgr := auth.NewManager(auth.Config{AuthRequired: false})

	handler := NewHandlerWithOptions(HandlerOptions{
		Store:       store,
		RiskEngine:  rEngine,
		AuthManager: aMgr,
	})
	return NewRouter(handler, logger)
}

func TestHealthAndReadinessEndpoints(t *testing.T) {
	router := testRouter()

	// 1. /healthz
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", rec.Code)
	}

	// 2. /readyz
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /readyz, got %d", rec.Code)
	}

	// 3. /api/v1/health
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/health, got %d", rec.Code)
	}
	var health map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&health); err != nil {
		t.Fatalf("decode health json: %v", err)
	}
	if health["status"] != "healthy" {
		t.Errorf("expected status healthy, got %v", health["status"])
	}
}

func TestSecurityHeadersAndCORS(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chains", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Check Security Headers
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control header missing or wrong: %s", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options missing: %s", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options missing: %s", rec.Header().Get("X-Frame-Options"))
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Errorf("X-Request-Id missing in response headers")
	}

	// Preflight OPTIONS test
	optReq := httptest.NewRequest(http.MethodOptions, "/api/v1/quotes", nil)
	optReq.Header.Set("Origin", "http://localhost:3000")
	optRec := httptest.NewRecorder()
	router.ServeHTTP(optRec, optReq)
	if optRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", optRec.Code)
	}
}

func TestRiskStatusEndpoint(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risk/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/risk/status, got %d", rec.Code)
	}

	var report risk.RiskReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("decode risk report: %v", err)
	}
	if report.Status != "HEALTHY" {
		t.Errorf("expected HEALTHY risk status, got %s", report.Status)
	}
	if report.CircuitBreaker != risk.CircuitClosed {
		t.Errorf("expected CLOSED circuit breaker, got %s", report.CircuitBreaker)
	}
}

func TestAuthEndpoints(t *testing.T) {
	router := testRouter()

	// 1. Get nonce
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/nonce", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/auth/nonce, got %d", rec.Code)
	}
	var nonceResp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&nonceResp)
	nonce := nonceResp["nonce"]
	if nonce == "" {
		t.Fatalf("nonce is empty")
	}

	// 2. Sign and verify Solana signature
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	wallet := base58.Encode(pub)
	msg := "Sign in with nonce " + nonce
	sig := ed25519.Sign(priv, []byte(msg))
	sigHex := "0x" + hex.EncodeToString(sig)

	verifyPayload := auth.VerifyRequest{
		Chain:         "solana",
		WalletAddress: wallet,
		Nonce:         nonce,
		Signature:     sigHex,
		Message:       msg,
	}
	b, _ := json.Marshal(verifyPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/auth/verify, got %d: %s", rec.Code, rec.Body.String())
	}

	var verifyResp struct {
		Token   string        `json:"token"`
		Session *auth.Session `json:"session"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&verifyResp)
	if verifyResp.Token == "" || verifyResp.Session == nil {
		t.Fatalf("expected token and session in auth response")
	}

	// 3. Logout
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+verifyResp.Token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/auth/logout, got %d", rec.Code)
	}
}

func TestChainsAndAssetsEndpoints(t *testing.T) {
	router := testRouter()

	// Chains
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chains", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Assets
	req = httptest.NewRequest(http.MethodGet, "/api/v1/assets?chain=near", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestQuoteAndSwapFlow(t *testing.T) {
	router := testRouter()

	// 1. Create quote
	quotePayload := map[string]any{
		"originAssetId":      "demo:near:near",
		"destinationAssetId": "demo:base:usdc",
		"amountIn":           "10",
		"slippageBps":        100,
	}
	bodyBytes, _ := json.Marshal(quotePayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for quote, got %d: %s", rec.Code, rec.Body.String())
	}

	var quote QuoteResponse
	if err := json.NewDecoder(rec.Body).Decode(&quote); err != nil {
		t.Fatalf("failed to decode quote response: %v", err)
	}
	if quote.ID == "" {
		t.Fatalf("quote ID is empty")
	}

	// 2. Create swap
	swapPayload := map[string]string{
		"quoteId": quote.ID,
	}
	bodyBytes, _ = json.Marshal(swapPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/swaps", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-idempotency-key-12345")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for swap, got %d: %s", rec.Code, rec.Body.String())
	}

	var swap SwapRecordResponse
	if err := json.NewDecoder(rec.Body).Decode(&swap); err != nil {
		t.Fatalf("failed to decode swap response: %v", err)
	}
	if swap.ID == "" {
		t.Fatalf("swap ID is empty")
	}

	// 3. Deposit step
	depPayload := map[string]string{
		"txHash":  "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"chainId": "8453",
	}
	bodyBytes, _ = json.Marshal(depPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/swaps/"+swap.ID+"/deposit", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for deposit, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Activity
	req = httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for activity, got %d: %s", rec.Code, rec.Body.String())
	}
}
