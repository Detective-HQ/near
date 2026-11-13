package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPortfolioEmptyAddress(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})
	req := httptest.NewRequest("GET", "/api/v1/portfolio", nil)
	rec := httptest.NewRecorder()
	api.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res PortfolioResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	if len(res.Balances) != 0 {
		t.Fatalf("expected 0 balances, got %d", len(res.Balances))
	}
	if res.TotalValueUSD == nil || *res.TotalValueUSD != "0" {
		t.Fatalf("expected totalValueUsd to be 0, got %v", res.TotalValueUSD)
	}
}

func TestPortfolioWithAddress(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})
	req := httptest.NewRequest("GET", "/api/v1/portfolio?address=0x1111111111111111111111111111111111111111", nil)
	rec := httptest.NewRecorder()
	api.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res PortfolioResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	if len(res.Balances) == 0 {
		t.Fatalf("expected balances, got 0")
	}
	if res.Address == nil || *res.Address != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("unexpected address in response: %v", res.Address)
	}
	if res.TotalValueUSD == nil || *res.TotalValueUSD == "0" {
		t.Fatalf("expected non-zero totalValueUsd, got %v", res.TotalValueUSD)
	}
	for _, b := range res.Balances {
		if b.AssetID == "" || b.Symbol == "" || b.Chain == "" || b.Balance == "" {
			t.Errorf("balance item missing required fields: %+v", b)
		}
	}
}

func TestPortfolioWithChainFilter(t *testing.T) {
	api := newTestAPI(t, false, RouterOptions{})
	req := httptest.NewRequest("GET", "/api/v1/portfolio?address=0x1111111111111111111111111111111111111111&chain=ethereum", nil)
	rec := httptest.NewRecorder()
	api.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res PortfolioResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	if len(res.Balances) == 0 {
		t.Fatalf("expected ethereum balances, got 0")
	}
	for _, b := range res.Balances {
		if b.Chain != "ethereum" {
			t.Errorf("expected chain 'ethereum', got %s", b.Chain)
		}
	}
}
