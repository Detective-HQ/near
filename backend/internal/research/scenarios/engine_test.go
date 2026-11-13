package scenarios

import (
	"math"
	"testing"

	"universal-defi-intelligence/internal/research/models"
)

func TestStandardScenarios(t *testing.T) {
	eng := New()
	resp, err := eng.GetStandardScenarios()
	if err != nil {
		t.Fatalf("GetStandardScenarios: %v", err)
	}

	if len(resp.Scenarios) != 3 {
		t.Fatalf("expected 3 standard scenarios, got %d", len(resp.Scenarios))
	}

	expected := map[string]float64{
		"bear": 536112.00,
		"base": 3626219.52,
		"bull": 15137280.00,
	}

	for _, sc := range resp.Scenarios {
		want, ok := expected[sc.Key]
		if !ok {
			t.Fatalf("unexpected scenario key %q", sc.Key)
		}
		if math.Abs(sc.ProjectedAnnualFees-want) > 0.01 {
			t.Fatalf("scenario %s = %.2f, want %.2f", sc.Key, sc.ProjectedAnnualFees, want)
		}
	}
}

func TestEvaluateCustom(t *testing.T) {
	eng := New()
	customInputs := models.ScenarioInputs{
		DailyVolumeUSD:    20_000_000,
		FeeRatePct:        0.15,
		CapturePct:        30,
		ActivityGrowthPct: 20,
	}

	sc, err := eng.EvaluateCustom(customInputs)
	if err != nil {
		t.Fatalf("EvaluateCustom: %v", err)
	}

	// 20M * (1 + 0.20) * (0.0015) * (0.30) * 365 = 3,942,000.00
	want := 3942000.00
	if math.Abs(sc.ProjectedAnnualFees-want) > 0.01 {
		t.Fatalf("custom fees = %.2f, want %.2f", sc.ProjectedAnnualFees, want)
	}
}
