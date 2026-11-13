package models

import (
	"math"
	"testing"
)

func TestGoldenScenarioValues(t *testing.T) {
	tests := []struct {
		name     string
		inputs   ScenarioInputs
		wantFees float64
	}{
		{
			name: "Bear scenario ($536,112)",
			inputs: ScenarioInputs{
				DailyVolumeUSD:    8_000_000,
				FeeRatePct:        0.12,
				CapturePct:        18,
				ActivityGrowthPct: -15,
			},
			wantFees: 536112.00,
		},
		{
			name: "Base scenario ($3,626,219.52)",
			inputs: ScenarioInputs{
				DailyVolumeUSD:    17_600_000,
				FeeRatePct:        0.18,
				CapturePct:        28,
				ActivityGrowthPct: 12,
			},
			wantFees: 3626219.52,
		},
		{
			name: "Bull scenario ($15,137,280)",
			inputs: ScenarioInputs{
				DailyVolumeUSD:    32_000_000,
				FeeRatePct:        0.24,
				CapturePct:        40,
				ActivityGrowthPct: 35,
			},
			wantFees: 15137280.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateAnnualCapturedFees(tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(got-tt.wantFees) > 0.001 {
				t.Fatalf("got = %.2f, want = %.2f", got, tt.wantFees)
			}
		})
	}
}

func TestScenarioValidation(t *testing.T) {
	// 1. Negative volume
	_, err := CalculateAnnualCapturedFees(ScenarioInputs{DailyVolumeUSD: -100})
	if err == nil {
		t.Fatal("expected error for negative volume")
	}

	// 2. Fee rate > 100%
	_, err = CalculateAnnualCapturedFees(ScenarioInputs{DailyVolumeUSD: 1000, FeeRatePct: 105})
	if err == nil {
		t.Fatal("expected error for fee rate > 100%")
	}

	// 3. Growth < -100%
	_, err = CalculateAnnualCapturedFees(ScenarioInputs{DailyVolumeUSD: 1000, ActivityGrowthPct: -150})
	if err == nil {
		t.Fatal("expected error for growth < -100%")
	}
}

func TestSensitivityGrid(t *testing.T) {
	base := ScenarioInputs{
		DailyVolumeUSD:    10_000_000,
		ActivityGrowthPct: 10,
		FeeRatePct:        0.20,
		CapturePct:        25,
	}

	volMults := []float64{0.5, 1.0, 1.5}
	feeRates := []float64{0.10, 0.20}

	grid, err := GenerateSensitivityGrid(base, volMults, feeRates)
	if err != nil {
		t.Fatalf("GenerateSensitivityGrid: %v", err)
	}

	if len(grid) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(grid))
	}
	if len(grid[0]) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(grid[0]))
	}

	// Cell [1][1] is exactly the base scenario:
	// 10M * (1 + 0.10) * 0.002 * 0.25 * 365 = 11,000,000 * 0.0005 * 365 = 2,007,500
	want := 2007500.00
	got := grid[1][1].AnnualFeesUSD
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("base cell = %.2f, want %.2f", got, want)
	}
}
