package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewStateStore()
	handler := NewHandler(store)
	return NewRouter(handler, logger)
}

func TestHealthEndpoint(t *testing.T) {
	router := testRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if body["status"] != "healthy" {
		t.Errorf("expected status healthy, got %v", body["status"])
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
	quotePayload := map[string]string{
		"originAssetId":      "demo:near:near",
		"destinationAssetId": "demo:base:usdc",
		"amountIn":           "10",
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

	// 3. Simulate step 1
	req = httptest.NewRequest(http.MethodPost, "/api/v1/swaps/"+swap.ID+"/simulate", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for simulate, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Activity
	req = httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for activity, got %d: %s", rec.Code, rec.Body.String())
	}
}
