package trading

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Backtest assumptions, reported verbatim to users.
const (
	backtestCostBps    = 10   // round-trip cost, basis points of notional
	backtestEntryScore = 40.0 // minimum score to take a trade
	backtestMinWindow  = minBars
)

// BacktestResult summarizes a walk-forward test on one timeframe.
type BacktestResult struct {
	Timeframe   string
	From, To    time.Time
	Trades      int
	Wins        int
	Losses      int
	Expired     int
	AvgR        float64
	TotalR      float64
	ProfitFact  float64
	MaxDDR      float64
	BenchReturn float64
	Assets      int
}

type trade struct {
	exit time.Time
	r    float64
	res  string
}

// RunBacktest replays the model bar by bar on the run's candles. At each bar
// only data up to that bar is used (no look-ahead). A trade opens when the
// score is at least 40 and the asset has no open trade; it is graded with the
// same Plan/Evaluate rules as the live journal, net of a 10 bps round-trip
// cost. Trades whose horizon extends past the data are excluded.
func RunBacktest(run *Run) BacktestResult {
	res := BacktestResult{Timeframe: run.Timeframe.Key}
	var trades []trade
	var benchSum float64
	for _, a := range run.Assets {
		c := a.Candles
		if len(c) < backtestMinWindow+run.Timeframe.Horizon+1 {
			continue
		}
		res.Assets++
		start := c[backtestMinWindow-1]
		if res.From.IsZero() || start.T.Before(res.From) {
			res.From = start.T
		}
		if c[len(c)-1].T.After(res.To) {
			res.To = c[len(c)-1].T
		}
		benchSum += c[len(c)-1].C/start.C - 1

		busyUntil := -1
		for i := backtestMinWindow - 1; i < len(c)-run.Timeframe.Horizon; i++ {
			if i <= busyUntil {
				continue
			}
			assessment := Assess(c[:i+1], run.Timeframe)
			if !assessment.Ranked || assessment.Direction == "neutral" || assessment.Score < backtestEntryScore {
				continue
			}
			plan, ok := MarketPlan(assessment)
			if !ok {
				continue
			}
			outcome := Evaluate(plan, c, i, run.Timeframe.Horizon)
			if outcome.Result == "open" {
				continue
			}
			risk := math.Abs(plan.Entry - plan.Stop)
			cost := plan.Entry * backtestCostBps / 10_000 / risk
			trades = append(trades, trade{exit: outcome.At, r: outcome.R - cost, res: outcome.Result})
			for k := i + 1; k < len(c); k++ {
				if !c[k].T.After(outcome.At) {
					busyUntil = k
				}
			}
		}
	}
	if res.Assets > 0 {
		res.BenchReturn = benchSum / float64(res.Assets) * 100
	}

	sort.Slice(trades, func(i, j int) bool { return trades[i].exit.Before(trades[j].exit) })
	var gains, losses, equity, peak float64
	for _, t := range trades {
		res.Trades++
		res.TotalR += t.r
		switch t.res {
		case "target_hit":
			res.Wins++
		case "stop_hit":
			res.Losses++
		default:
			res.Expired++
		}
		if t.r > 0 {
			gains += t.r
		} else {
			losses -= t.r
		}
		equity += t.r
		peak = math.Max(peak, equity)
		res.MaxDDR = math.Max(res.MaxDDR, peak-equity)
	}
	if res.Trades > 0 {
		res.AvgR = res.TotalR / float64(res.Trades)
	}
	if losses > 0 {
		res.ProfitFact = gains / losses
	}
	return res
}

// Backtest returns the run's backtest, computed once per run.
func (r *Run) Backtest() BacktestResult {
	r.btOnce.Do(func() {
		result := RunBacktest(r)
		r.backtest = &result
	})
	return *r.backtest
}

func backtestMethodology(tf Timeframe) string {
	return fmt.Sprintf("Walk-forward replay of model %s on %s candles: at each bar only prior data is used; a trade opens when |score| >= %.0f "+
		"and the asset has no open trade. Entry is the signal bar's close (no limit fills are assumed), the stop 1.5 ATR away and the target 3R, "+
		"the same rules used to grade the live signal journal; "+
		"trades close at target, stop (checked first within a bar) or after %d bars. Candles are approximated from sampled aggregate prices, "+
		"so intrabar extremes are understated. Past results do not predict future results.",
		ModelVersion, tf.Key, backtestEntryScore, tf.Horizon)
}
