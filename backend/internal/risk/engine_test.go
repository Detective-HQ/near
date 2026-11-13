package risk

import (
	"testing"
	"time"
)

func TestRiskEngine_SlippageCheck(t *testing.T) {
	engine := NewEngine(Config{
		MaxSlippageBps:     500, // 5%
		WarningSlippageBps: 200, // 2%
	})

	// Normal slippage (1%)
	exceeds, warn := engine.CheckSlippage(100)
	if exceeds || warn {
		t.Fatalf("Expected 100 bps to be normal, got exceeds=%v, warn=%v", exceeds, warn)
	}

	// Warning slippage (3%)
	exceeds, warn = engine.CheckSlippage(300)
	if exceeds || !warn {
		t.Fatalf("Expected 300 bps to trigger warning only, got exceeds=%v, warn=%v", exceeds, warn)
	}

	// Dangerous slippage (6%)
	exceeds, warn = engine.CheckSlippage(600)
	if !exceeds || !warn {
		t.Fatalf("Expected 600 bps to exceed maximum, got exceeds=%v, warn=%v", exceeds, warn)
	}
}

func TestRiskEngine_CircuitBreaker(t *testing.T) {
	engine := NewEngine(Config{
		ErrorRateThreshold: 0.30,
	})

	// Record 10 successful calls
	for i := 0; i < 10; i++ {
		engine.RecordProviderCall(200)
	}
	status := engine.Status()
	if status.CircuitBreaker != CircuitClosed {
		t.Fatalf("Expected circuit CLOSED, got %s", status.CircuitBreaker)
	}

	// Record 5 provider 500 errors
	for i := 0; i < 5; i++ {
		engine.RecordProviderCall(500)
	}
	status = engine.Status()
	if status.CircuitBreaker == CircuitClosed {
		t.Fatalf("Expected circuit breaker to be elevated, got CLOSED")
	}
	if status.ProviderErrorRate <= 0 {
		t.Fatalf("Expected provider error rate > 0")
	}
}

func TestRiskEngine_CatalogueStaleness(t *testing.T) {
	engine := NewEngine(Config{
		CatalogueMaxAge: 1 * time.Hour,
	})

	// Set last refreshed to 3 hours ago
	engine.RecordCatalogueRefresh(time.Now().UTC().Add(-3 * time.Hour))

	status := engine.Status()
	if !status.CatalogueStale {
		t.Fatalf("Expected catalogue to be marked stale")
	}
	if len(status.ActiveAlerts) == 0 {
		t.Fatalf("Expected alert for stale catalogue")
	}
}
