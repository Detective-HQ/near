package trading

import (
	"fmt"
	"math"
	"time"
)

// ModelVersion identifies the scoring rules below.
const ModelVersion = "trend-structure-1.0"

// MethodologyURL documents the model for users.
const MethodologyURL = "https://github.com/Detective-HQ/near/blob/main/docs/research-methodology.md#trading-signals"

const (
	minBars          = 60   // bars required before an asset is ranked
	minCoverage      = 80.0 // percent of expected bars required
	neutralBand      = 0.15 // |composite| below this is neutral
	momentumLookback = 20
)

// EvidenceItem is one input to the score, reported verbatim to users.
type EvidenceItem struct {
	Type  string
	Value string
}

// Assessment is the model output for one asset on one timeframe.
//
// Composite is a weighted sum in [-1, 1]:
//
//	0.40 × momentum   tanh of the lookback return divided by its own volatility
//	0.35 × trend      +1 when close > EMA20 > EMA50, −1 for the mirror, else
//	                  the sign of EMA20 − EMA50 scaled by 0.5
//	0.25 × structure  +1 / −1 for the direction of the latest BOS/CHoCH
//
// Direction is the sign of Composite (neutral inside ±0.15). Score is
// |Composite| × 100 and only ranks assets against each other; it is not a
// probability.
type Assessment struct {
	Direction string
	Composite float64
	Score     float64
	Momentum  float64
	Trend     float64
	Struct    float64
	RSI       float64
	ATR       float64
	Last      float64
	Coverage  float64
	Evidence  []EvidenceItem
	Structure Structure
	Ranked    bool
	Reason    string
}

// Assess runs the model on a candle series.
func Assess(candles []Candle, tf Timeframe) Assessment {
	a := Assessment{Coverage: Coverage(candles, tf), Direction: "neutral"}
	if len(candles) < minBars {
		a.Reason = fmt.Sprintf("Insufficient candle history (%d of %d bars).", len(candles), minBars)
		return a
	}
	if a.Coverage < minCoverage {
		a.Reason = fmt.Sprintf("Coverage %.0f%% is below the %.0f%% minimum for ranking.", a.Coverage, minCoverage)
		return a
	}

	c := closes(candles)
	last := c[len(c)-1]
	a.Last = last
	a.ATR = ATR(candles, 14)

	// Momentum: lookback return normalized by realized volatility.
	ret := last/c[len(c)-1-momentumLookback] - 1
	var sumSq float64
	for i := len(c) - momentumLookback; i < len(c); i++ {
		r := c[i]/c[i-1] - 1
		sumSq += r * r
	}
	vol := math.Sqrt(sumSq/momentumLookback) * math.Sqrt(momentumLookback)
	if vol > 0 {
		a.Momentum = math.Tanh(ret / vol)
	}

	ema20, ema50 := EMA(c, 20), EMA(c, 50)
	e20, e50 := ema20[len(ema20)-1], ema50[len(ema50)-1]
	switch {
	case last > e20 && e20 > e50:
		a.Trend = 1
	case last < e20 && e20 < e50:
		a.Trend = -1
	case e20 > e50:
		a.Trend = 0.5
	case e20 < e50:
		a.Trend = -0.5
	}

	a.Structure = AnalyzeStructure(candles)
	switch a.Structure.Trend {
	case "bullish":
		a.Struct = 1
	case "bearish":
		a.Struct = -1
	}

	a.RSI, _ = RSI(c, 14)
	a.Composite = 0.40*a.Momentum + 0.35*a.Trend + 0.25*a.Struct
	a.Score = math.Abs(a.Composite) * 100
	switch {
	case a.Composite >= neutralBand:
		a.Direction = "bullish"
	case a.Composite <= -neutralBand:
		a.Direction = "bearish"
	}
	a.Ranked = true

	a.Evidence = []EvidenceItem{
		{Type: fmt.Sprintf("%d-bar return", momentumLookback), Value: pct(ret * 100)},
		{Type: "Momentum (vol-normalized)", Value: fmt.Sprintf("%.2f", a.Momentum)},
		{Type: "EMA20 vs EMA50", Value: emaRelation(e20, e50)},
		{Type: "Latest structure break", Value: latestBreak(a.Structure)},
		{Type: "RSI(14)", Value: fmt.Sprintf("%.1f", a.RSI)},
		{Type: "ATR(14)", Value: FormatPrice(a.ATR)},
	}
	return a
}

func emaRelation(e20, e50 float64) string {
	if e20 > e50 {
		return fmt.Sprintf("EMA20 above EMA50 by %s", pct((e20/e50-1)*100))
	}
	return fmt.Sprintf("EMA20 below EMA50 by %s", pct((1-e20/e50)*100))
}

func latestBreak(s Structure) string {
	if len(s.Breaks) == 0 {
		return "None in window"
	}
	b := s.Breaks[len(s.Breaks)-1]
	return fmt.Sprintf("%s %s at %s", b.Direction, b.Kind, FormatPrice(b.Price))
}

// Plan is a rule-based trade framing used by scenarios, the journal and
// backtests. Entry is the reference close; the stop sits 1.5 ATR beyond it
// and the targets at 1.5R and 3R.
type Plan struct {
	Direction  string
	Entry      float64
	EntryLow   float64
	EntryHigh  float64
	Stop       float64
	Target1    float64
	Target2    float64
	RiskReward float64
	FromZone   bool
}

// BuildPlan frames a plan for an assessment. When a fresh or tested order
// block exists in the signal direction, the entry area is that block and the
// stop sits 0.25 ATR beyond its far edge; otherwise the entry is the last
// close with an ATR stop.
func BuildPlan(a Assessment) (Plan, bool) {
	if !a.Ranked || a.Direction == "neutral" || a.ATR <= 0 {
		return Plan{}, false
	}
	p := Plan{Direction: a.Direction}
	sign := 1.0
	if a.Direction == "bearish" {
		sign = -1
	}
	if zone, ok := a.Structure.ActiveZone(a.Direction); ok && zoneNear(zone, a.Last, a.ATR) {
		p.FromZone = true
		p.EntryLow, p.EntryHigh = zone.Bottom, zone.Top
		p.Entry = (zone.Top + zone.Bottom) / 2
		if sign > 0 {
			p.Stop = zone.Bottom - 0.25*a.ATR
		} else {
			p.Stop = zone.Top + 0.25*a.ATR
		}
	} else {
		p.Entry = a.Last
		p.EntryLow, p.EntryHigh = a.Last-0.25*a.ATR, a.Last+0.25*a.ATR
		p.Stop = a.Last - sign*1.5*a.ATR
	}
	risk := math.Abs(p.Entry - p.Stop)
	if risk <= 0 {
		return Plan{}, false
	}
	p.Target1 = p.Entry + sign*1.5*risk
	p.Target2 = p.Entry + sign*3*risk
	p.RiskReward = 3
	return p, true
}

// MarketPlan is the executable framing used to grade signals in the journal
// and in backtests: entry at the signal bar's close (a price that actually
// traded), stop 1.5 ATR away, targets at 1.5R and 3R. Zone-based entries are
// limit-order suggestions that may never fill, so grading them as filled would
// overstate results; they are only shown in the scenario view.
func MarketPlan(a Assessment) (Plan, bool) {
	if !a.Ranked || a.Direction == "neutral" || a.ATR <= 0 || a.Last <= 0 {
		return Plan{}, false
	}
	sign := 1.0
	if a.Direction == "bearish" {
		sign = -1
	}
	risk := 1.5 * a.ATR
	return Plan{
		Direction:  a.Direction,
		Entry:      a.Last,
		EntryLow:   a.Last,
		EntryHigh:  a.Last,
		Stop:       a.Last - sign*risk,
		Target1:    a.Last + sign*1.5*risk,
		Target2:    a.Last + sign*3*risk,
		RiskReward: 3,
	}, true
}

// zoneNear accepts a zone within 3 ATR of the current price.
func zoneNear(z Zone, price, atr float64) bool {
	mid := (z.Top + z.Bottom) / 2
	return math.Abs(price-mid) <= 3*atr
}

// Outcome evaluates a plan against the candles after entry, for at most
// horizon bars. Stops are checked before targets inside a bar (conservative,
// because sampled candles cannot order intrabar moves).
type Outcome struct {
	Result string // target_hit | stop_hit | expired | open
	R      float64
	MFE    float64 // percent
	MAE    float64 // percent
	At     time.Time
}

// Evaluate walks forward from the bar after entryIndex.
func Evaluate(p Plan, candles []Candle, entryIndex, horizon int) Outcome {
	sign := 1.0
	if p.Direction == "bearish" {
		sign = -1
	}
	risk := math.Abs(p.Entry - p.Stop)
	out := Outcome{Result: "open"}
	end := entryIndex + horizon
	for i := entryIndex + 1; i < len(candles) && i <= end; i++ {
		c := candles[i]
		var fav, adv float64
		if sign > 0 {
			fav = (c.H - p.Entry) / p.Entry * 100
			adv = (c.L - p.Entry) / p.Entry * 100
		} else {
			fav = (p.Entry - c.L) / p.Entry * 100
			adv = (p.Entry - c.H) / p.Entry * 100
		}
		out.MFE = math.Max(out.MFE, fav)
		out.MAE = math.Min(out.MAE, adv)
		stopHit := (sign > 0 && c.L <= p.Stop) || (sign < 0 && c.H >= p.Stop)
		targetHit := (sign > 0 && c.H >= p.Target2) || (sign < 0 && c.L <= p.Target2)
		if stopHit {
			out.Result, out.R, out.At = "stop_hit", -1, c.T
			return out
		}
		if targetHit {
			out.Result, out.R, out.At = "target_hit", math.Abs(p.Target2-p.Entry)/risk, c.T
			return out
		}
		if i == end {
			out.Result, out.At = "expired", c.T
			out.R = sign * (c.C - p.Entry) / risk
			return out
		}
	}
	return out
}

// FormatPrice renders a price with precision suited to its magnitude.
func FormatPrice(v float64) string {
	abs := math.Abs(v)
	switch {
	case abs == 0:
		return "0"
	case abs >= 1000:
		return fmt.Sprintf("%.2f", v)
	case abs >= 1:
		return fmt.Sprintf("%.4f", v)
	case abs >= 0.01:
		return fmt.Sprintf("%.6f", v)
	default:
		return fmt.Sprintf("%.10f", v)
	}
}

func pct(v float64) string { return fmt.Sprintf("%.2f%%", v) }

func dec(v float64, places int) string { return fmt.Sprintf("%.*f", places, v) }
