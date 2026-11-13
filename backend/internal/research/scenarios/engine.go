// Package scenarios evaluates financial and market research scenarios for the protocol.
package scenarios

import (
	"universal-defi-intelligence/internal/research/models"
)

// Scenario represents a named research projection scenario.
type Scenario struct {
	Key                 string                `json:"key"`
	Label               string                `json:"label"`
	Inputs              models.ScenarioInputs `json:"inputs"`
	ProjectedAnnualFees float64               `json:"projectedAnnualFees"`
}

// ScenariosResponse matches the research scenarios contract.
type ScenariosResponse struct {
	Unit      string     `json:"unit"`
	Note      string     `json:"note"`
	Formula   string     `json:"formula"`
	Scenarios []Scenario `json:"scenarios"`
}

// Predefined baseline scenario inputs matching golden reference numbers.
var DefaultBaseline = map[string]models.ScenarioInputs{
	"bear": {
		DailyVolumeUSD:    8_000_000,
		FeeRatePct:        0.12,
		CapturePct:        18,
		ActivityGrowthPct: -15,
	},
	"base": {
		DailyVolumeUSD:    17_600_000,
		FeeRatePct:        0.18,
		CapturePct:        28,
		ActivityGrowthPct: 12,
	},
	"bull": {
		DailyVolumeUSD:    32_000_000,
		FeeRatePct:        0.24,
		CapturePct:        40,
		ActivityGrowthPct: 35,
	},
}

// Engine provides evaluation of standard and custom research scenarios.
type Engine struct{}

// New creates an initialized scenario engine.
func New() *Engine {
	return &Engine{}
}

// GetStandardScenarios returns the Bear, Base, and Bull models evaluated with the golden numbers.
func (e *Engine) GetStandardScenarios() (ScenariosResponse, error) {
	keys := []string{"bear", "base", "bull"}
	labels := map[string]string{
		"bear": "Bear",
		"base": "Base",
		"bull": "Bull",
	}

	scenarios := make([]Scenario, 0, len(keys))
	for _, k := range keys {
		inputs := DefaultBaseline[k]
		annual, err := models.CalculateAnnualCapturedFees(inputs)
		if err != nil {
			return ScenariosResponse{}, err
		}
		scenarios = append(scenarios, Scenario{
			Key:                 k,
			Label:               labels[k],
			Inputs:              inputs,
			ProjectedAnnualFees: annual,
		})
	}

	return ScenariosResponse{
		Unit:      "USD / year",
		Note:      "Scenario arithmetic only. Illustrative research projections; not investment advice.",
		Formula:   "Annual captured fees = daily volume × (1 + activity growth% / 100) × (fee rate% / 100) × (capture share% / 100) × 365",
		Scenarios: scenarios,
	}, nil
}

// EvaluateCustom calculates projected annual fees for custom user-supplied parameters.
func (e *Engine) EvaluateCustom(inputs models.ScenarioInputs) (Scenario, error) {
	annual, err := models.CalculateAnnualCapturedFees(inputs)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{
		Key:                 "custom",
		Label:               "Custom Scenario",
		Inputs:              inputs,
		ProjectedAnnualFees: annual,
	}, nil
}
