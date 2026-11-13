package trading

import (
	"math"
	"time"
)

// Candle is an OHLC bar approximated from sampled prices.
type Candle struct {
	T          time.Time
	O, H, L, C float64
	Samples    int
}

// Timeframe describes how candles are built from samples.
type Timeframe struct {
	Key     string
	Bar     time.Duration // candle width
	Sample  time.Duration // spacing of source samples
	Points  int           // samples requested per asset (<= MaxPointsPerRequest)
	TTL     time.Duration // cache lifetime of computed results
	Horizon int           // bars a signal or scenario stays valid
}

// Timeframes are the only timeframes served. Each candle needs at least four
// samples, so no timeframe is offered below what the source can support.
var Timeframes = []Timeframe{
	{Key: "4h", Bar: 4 * time.Hour, Sample: time.Hour, Points: 480, TTL: 15 * time.Minute, Horizon: 12},
	{Key: "1d", Bar: 24 * time.Hour, Sample: 4 * time.Hour, Points: 480, TTL: time.Hour, Horizon: 10},
}

// DefaultTimeframe is used when a request does not choose one.
const DefaultTimeframe = "4h"

// FindTimeframe returns a served timeframe.
func FindTimeframe(key string) (Timeframe, bool) {
	if key == "" {
		key = DefaultTimeframe
	}
	for _, tf := range Timeframes {
		if tf.Key == key {
			return tf, true
		}
	}
	return Timeframe{}, false
}

// TimeframeKeys lists served timeframe keys.
func TimeframeKeys() []string {
	keys := make([]string, len(Timeframes))
	for i, tf := range Timeframes {
		keys[i] = tf.Key
	}
	return keys
}

// BuildCandles buckets samples into UTC-aligned bars. Buckets with fewer than
// minSamples samples are dropped (an incomplete bar would fake a range), and
// so is the still-forming last bar.
func BuildCandles(points []PricePoint, bar time.Duration, minSamples int, now time.Time) []Candle {
	var candles []Candle
	for _, p := range points {
		start := p.At.Truncate(bar)
		if n := len(candles); n > 0 && candles[n-1].T.Equal(start) {
			c := &candles[n-1]
			c.H = math.Max(c.H, p.Price)
			c.L = math.Min(c.L, p.Price)
			c.C = p.Price
			c.Samples++
			continue
		}
		candles = append(candles, Candle{T: start, O: p.Price, H: p.Price, L: p.Price, C: p.Price, Samples: 1})
	}
	out := candles[:0]
	for _, c := range candles {
		if c.Samples >= minSamples && !c.T.Add(bar).After(now) {
			out = append(out, c)
		}
	}
	return out
}

// Coverage is the share of expected bars present, in percent.
func Coverage(candles []Candle, tf Timeframe) float64 {
	expected := float64(tf.Points) * float64(tf.Sample) / float64(tf.Bar)
	if expected <= 0 {
		return 0
	}
	return math.Min(100, 100*float64(len(candles))/(expected-1))
}

func closes(candles []Candle) []float64 {
	out := make([]float64, len(candles))
	for i, c := range candles {
		out[i] = c.C
	}
	return out
}

// EMA returns the exponential moving average series (first value seeded with
// the simple average of the first period values).
func EMA(values []float64, period int) []float64 {
	if len(values) < period || period <= 0 {
		return nil
	}
	out := make([]float64, len(values))
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	out[period-1] = sum / float64(period)
	k := 2 / float64(period+1)
	for i := period; i < len(values); i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out
}

// RSI returns Wilder's relative strength index of the last value.
func RSI(values []float64, period int) (float64, bool) {
	if len(values) <= period {
		return 0, false
	}
	var gain, loss float64
	for i := 1; i <= period; i++ {
		d := values[i] - values[i-1]
		if d > 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	gain /= float64(period)
	loss /= float64(period)
	for i := period + 1; i < len(values); i++ {
		d := values[i] - values[i-1]
		g, l := 0.0, 0.0
		if d > 0 {
			g = d
		} else {
			l = -d
		}
		gain = (gain*float64(period-1) + g) / float64(period)
		loss = (loss*float64(period-1) + l) / float64(period)
	}
	if loss == 0 {
		return 100, true
	}
	return 100 - 100/(1+gain/loss), true
}

// ATR returns the average true range over the last period bars.
func ATR(candles []Candle, period int) float64 {
	if len(candles) < period+1 {
		return 0
	}
	sum := 0.0
	for i := len(candles) - period; i < len(candles); i++ {
		prev := candles[i-1].C
		tr := math.Max(candles[i].H-candles[i].L, math.Max(math.Abs(candles[i].H-prev), math.Abs(candles[i].L-prev)))
		sum += tr
	}
	return sum / float64(period)
}

// Volatility is the annualized standard deviation of log returns, in percent.
func Volatility(candles []Candle, bar time.Duration) float64 {
	if len(candles) < 3 {
		return 0
	}
	returns := make([]float64, 0, len(candles)-1)
	for i := 1; i < len(candles); i++ {
		returns = append(returns, math.Log(candles[i].C/candles[i-1].C))
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns) - 1)
	barsPerYear := float64(365*24*time.Hour) / float64(bar)
	return math.Sqrt(variance*barsPerYear) * 100
}
