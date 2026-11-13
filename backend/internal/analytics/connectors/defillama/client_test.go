package defillama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"universal-defi-intelligence/internal/analytics/domain"
)

func TestDeFiLlamaFetchHappyPath(t *testing.T) {
	mux := http.NewServeMux()

	// 1. Chains catalogue
	mux.HandleFunc("/v2/chains", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "Near", "tvl": 125000000.0},
			{"name": "Ethereum", "tvl": 50000000000.0},
		})
	})

	// 2. Historical TVL
	mux.HandleFunc("/v2/historicalChainTvl/Near", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"date": time.Now().Add(-24 * time.Hour).Unix(), "tvl": 120000000.0},
			{"date": time.Now().Unix(), "tvl": 125000000.0},
		})
	})

	// 3. DEX volume
	mux.HandleFunc("/overview/dexs/Near", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total24h": 17600000.0,
		})
	})

	// 4. Stablecoin chains
	mux.HandleFunc("/stablecoinchains", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"name": "Near",
				"totalCirculatingUSD": map[string]any{
					"peggedUSD": 82000000.0,
				},
			},
		})
	})

	// 5. Fees & Revenue
	mux.HandleFunc("/overview/fees/Near", func(w http.ResponseWriter, r *http.Request) {
		dataType := r.URL.Query().Get("dataType")
		w.Header().Set("Content-Type", "application/json")
		if dataType == "dailyFees" {
			_ = json.NewEncoder(w).Encode(map[string]any{"total24h": 18400.0})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"total24h": 4600.0})
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := New(Config{
		APIBaseURL:        server.URL,
		StablecoinBaseURL: server.URL,
		Timeout:           2 * time.Second,
	})

	snap, err := client.Fetch(context.Background(), "near")
	if err != nil {
		t.Fatalf("client.Fetch: %v", err)
	}

	if snap.Status != domain.StatusAvailable {
		t.Fatalf("expected status %s, got %s", domain.StatusAvailable, snap.Status)
	}

	if len(snap.Metrics) != 5 {
		t.Fatalf("expected 5 metrics, got %d", len(snap.Metrics))
	}

	metricMap := make(map[string]*float64)
	for _, m := range snap.Metrics {
		metricMap[m.MetricKey] = m.Value
	}

	if metricMap["tvl"] == nil || *metricMap["tvl"] != 125000000.0 {
		t.Fatalf("tvl = %v, want 125000000", metricMap["tvl"])
	}
	if metricMap["dex_volume_24h"] == nil || *metricMap["dex_volume_24h"] != 17600000.0 {
		t.Fatalf("dex_volume_24h = %v, want 17600000", metricMap["dex_volume_24h"])
	}
	if metricMap["stablecoin_supply"] == nil || *metricMap["stablecoin_supply"] != 82000000.0 {
		t.Fatalf("stablecoin_supply = %v, want 82000000", metricMap["stablecoin_supply"])
	}
	if metricMap["protocol_fees_24h"] == nil || *metricMap["protocol_fees_24h"] != 18400.0 {
		t.Fatalf("protocol_fees_24h = %v, want 18400", metricMap["protocol_fees_24h"])
	}
}

func TestDeFiLlamaPartialFailure(t *testing.T) {
	mux := http.NewServeMux()

	// Only chains returns data, other endpoints 500
	mux.HandleFunc("/v2/chains", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "Near", "tvl": 100000.0},
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := New(Config{
		APIBaseURL:        server.URL,
		StablecoinBaseURL: server.URL,
		Timeout:           time.Second,
	})

	snap, err := client.Fetch(context.Background(), "near")
	if err != nil {
		t.Fatalf("client.Fetch: %v", err)
	}

	// Because TVL succeeded but DEX/fees failed, status should be PARTIAL
	if snap.Status != domain.StatusPartial {
		t.Fatalf("expected status %s, got %s", domain.StatusPartial, snap.Status)
	}
}

