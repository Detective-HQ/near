// Package models implements exact research modeling and scenario mathematics.
package models

import (
	"errors"
	"math"
)

// DaysPerYear is the annualization factor specified by the research methodology.
const DaysPerYear = 365.0

// ScenarioInputs holds the quantitative inputs for a fee-capture scenario.
// Percentage fields (Growth, FeeRate, Capture) are percent-valued (e.g. 12 means 12%).
type ScenarioInputs struct {
	DailyVolumeUSD    float64 `json:"dailyVolumeUsd"`
	ActivityGrowthPct float64 `json:"activityGrowthPct"`
	FeeRatePct        float64 `json:"feeRatePct"`
	CapturePct        float64 `json:"capturePct"`
}

// Validate ensures inputs are within reasonable economic bounds.
func (s ScenarioInputs) Validate() error {
	if s.DailyVolumeUSD < 0 {
		return errors.New("daily volume cannot be negative")
	}
	if s.FeeRatePct < 0 || s.FeeRatePct > 100 {
		return errors.New("fee rate percentage must be between 0 and 100")
	}
	if s.CapturePct < 0 || s.CapturePct > 100 {
		return errors.New("capture share percentage must be between 0 and 100")
	}
	if s.ActivityGrowthPct < -100 {
		return errors.New("activity growth percentage cannot be less than -100%")
	}
	return nil
}

// CalculateAnnualCapturedFees computes annual projected protocol fees using the
// standardized UDI research formula:
//
//	Annual Fees = Daily Volume × (1 + Growth% / 100) × (Fee Rate% / 100) × (Capture% / 100) × 365
//
// Golden reference values:
//   - Bear:  $536,112.00    (Volume: $8M,  FeeRate: 0.12%, Capture: 18%, Growth: -15%)
//   - Base:  $3,626,219.52  (Volume: $17.6M, FeeRate: 0.18%, Capture: 28%, Growth: +12%)
//   - Bull:  $15,137,280.00 (Volume: $32M, FeeRate: 0.24%, Capture: 40%, Growth: +35%)
func CalculateAnnualCapturedFees(inputs ScenarioInputs) (float64, error) {
	if err := inputs.Validate(); err != nil {
		return 0, err
	}

	growthMultiplier := 1.0 + (inputs.ActivityGrowthPct / 100.0)
	feeRateFactor := inputs.FeeRatePct / 100.0
	captureFactor := inputs.CapturePct / 100.0

	annual := inputs.DailyVolumeUSD * growthMultiplier * feeRateFactor * captureFactor * DaysPerYear
	// Round to 2 decimal places (cents)
	rounded := math.Round(annual*100) / 100
	return rounded, nil
}

// SensitivityCell represents one cell in a sensitivity projection table.
type SensitivityCell struct {
	VolumeMultiplier float64 `json:"volumeMultiplier"`
	DailyVolumeUSD   float64 `json:"dailyVolumeUsd"`
	FeeRatePct       float64 `json:"feeRatePct"`
	AnnualFeesUSD    float64 `json:"annualFeesUsd"`
}

// GenerateSensitivityGrid calculates annual fees across a matrix of volume multipliers and fee rates.
func GenerateSensitivityGrid(base ScenarioInputs, volumeMultipliers []float64, feeRates []float64) ([][]SensitivityCell, error) {
	if err := base.Validate(); err != nil {
		return nil, err
	}

	grid := make([][]SensitivityCell, len(volumeMultipliers))
	for i, vMult := range volumeMultipliers {
		grid[i] = make([]SensitivityCell, len(feeRates))
		vol := base.DailyVolumeUSD * vMult
		for j, fRate := range feeRates {
			modInputs := ScenarioInputs{
				DailyVolumeUSD:    vol,
				ActivityGrowthPct: base.ActivityGrowthPct,
				FeeRatePct:        fRate,
				CapturePct:        base.CapturePct,
			}
			annual, err := CalculateAnnualCapturedFees(modInputs)
			if err != nil {
				return nil, err
			}
			grid[i][j] = SensitivityCell{
				VolumeMultiplier: vMult,
				DailyVolumeUSD:   vol,
				FeeRatePct:       fRate,
				AnnualFeesUSD:    annual,
			}
		}
	}
	return grid, nil
}
