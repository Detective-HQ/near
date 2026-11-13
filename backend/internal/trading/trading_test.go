package trading

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// fakeSource produces deterministic trending series per coin.
type fakeSource struct {
	trend map[string]float64 // per-sample drift
	fail  map[string]bool
	down  atomic.Bool
	calls atomic.Int64
}

func (f *fakeSource) Name() string { return "fake" }

func (f *fakeSource) History(_ context.Context, coin string, sample time.Duration, points int) ([]PricePoint, error) {
	f.calls.Add(1)
	if f.down.Load() || f.fail[coin] {
		return nil, errors.New("upstream down")
	}
	drift := f.trend[coin]
	out := make([]PricePoint, points)
	start := testNow.Add(-sample * time.Duration(points))
	for i := 0; i < points; i++ {
		// Trend plus a deterministic oscillation so swings and pullbacks form.
		price := 100 * math.Exp(drift*float64(i)) * (1 + 0.02*math.Sin(float64(i)/5))
		out[i] = PricePoint{At: start.Add(sample * time.Duration(i+1)), Price: price}
	}
	return out, nil
}

func newFake() *fakeSource {
	f := &fakeSource{trend: map[string]float64{}, fail: map[string]bool{}}
	for i, asset := range Universe {
		switch {
		case i%3 == 0:
			f.trend[asset.CoinKey] = 0.002
		case i%3 == 1:
			f.trend[asset.CoinKey] = -0.002
		default:
			f.trend[asset.CoinKey] = 0
		}
	}
	return f
}

func newTestEngine(src PriceSource) *Engine {
	return NewEngine(Config{Source: src, Now: func() time.Time { return testNow }})
}

func TestBuildCandlesAggregatesAndDropsIncompleteBars(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := []float64{10, 12, 9, 11, 11, 13, 8, 10, 7}
	var points []PricePoint
	for i, p := range prices {
		points = append(points, PricePoint{At: base.Add(time.Duration(i) * time.Hour), Price: p})
	}
	candles := BuildCandles(points, 4*time.Hour, 3, base.Add(9*time.Hour))
	if len(candles) != 2 {
		t.Fatalf("candles = %d, want 2 (last bucket incomplete)", len(candles))
	}
	c := candles[0]
	if c.O != 10 || c.H != 12 || c.L != 9 || c.C != 11 {
		t.Fatalf("first candle = %+v", c)
	}
	if c2 := candles[1]; c2.O != 11 || c2.H != 13 || c2.L != 8 || c2.C != 10 {
		t.Fatalf("second candle = %+v", c2)
	}
}

func seriesCandles(drift float64, n int) []Candle {
	src := &fakeSource{trend: map[string]float64{"x": drift}}
	tf := Timeframes[0]
	points, _ := src.History(context.Background(), "x", tf.Sample, n*4)
	return BuildCandles(points, tf.Bar, 3, testNow.Add(time.Hour))
}

func TestAssessDirectionFollowsTrend(t *testing.T) {
	tf := Timeframes[0]
	up := Assess(seriesCandles(0.003, 120), tf)
	down := Assess(seriesCandles(-0.003, 120), tf)
	if !up.Ranked || up.Direction != "bullish" || up.Score <= 0 {
		t.Fatalf("uptrend assessment = %+v", up)
	}
	if !down.Ranked || down.Direction != "bearish" {
		t.Fatalf("downtrend assessment = %+v", down)
	}
	short := Assess(seriesCandles(0.003, 30), tf)
	if short.Ranked || !strings.Contains(short.Reason, "Insufficient") {
		t.Fatalf("short history must not rank: %+v", short)
	}
	if len(up.Evidence) == 0 || up.Structure.Trend != "bullish" {
		t.Fatalf("missing evidence/structure: %+v", up.Structure.Trend)
	}
}

func TestStructureFindsSwingsBreaksAndZones(t *testing.T) {
	st := AnalyzeStructure(seriesCandles(0.003, 120))
	if len(st.Swings) == 0 || len(st.Breaks) == 0 {
		t.Fatalf("swings=%d breaks=%d", len(st.Swings), len(st.Breaks))
	}
	hasOB := false
	for i, z := range st.Zones {
		if z.Kind == "order_block" {
			hasOB = true
		}
		if z.Top < z.Bottom {
			t.Fatalf("zone %d inverted: %+v", i, z)
		}
		if i > 0 && st.Zones[i-1].Index > z.Index {
			t.Fatal("zones not chronological")
		}
		switch z.Status {
		case "fresh", "tested", "mitigated", "invalidated", "expired":
		default:
			t.Fatalf("bad status %q", z.Status)
		}
	}
	if !hasOB {
		t.Fatal("no order block detected in a trending series")
	}
	if st.Breaks[0].Kind != "BOS" {
		t.Fatal("first break must be a BOS")
	}
}

func TestPlanAndEvaluateStopBeforeTarget(t *testing.T) {
	plan := Plan{Direction: "bullish", Entry: 100, Stop: 95, Target2: 115}
	candles := []Candle{
		{C: 100},
		{H: 104, L: 99, C: 103},
		{H: 116, L: 94, C: 110}, // both touched: stop wins (conservative)
	}
	if out := Evaluate(plan, candles, 0, 10); out.Result != "stop_hit" || out.R != -1 {
		t.Fatalf("outcome = %+v", out)
	}
	candles[2] = Candle{H: 116, L: 101, C: 115}
	if out := Evaluate(plan, candles, 0, 10); out.Result != "target_hit" || out.R != 3 || out.MFE <= 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if out := Evaluate(plan, candles[:2], 0, 1); out.Result != "expired" {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestEngineEndToEnd(t *testing.T) {
	src := newFake()
	src.fail[Universe[len(Universe)-1].CoinKey] = true
	engine := newTestEngine(src)
	ctx := context.Background()

	overview, err := engine.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b := *overview.Breadth
	if b["bullish"] == 0 || b["bearish"] == 0 || overview.Quality != "partial" || overview.ModelVersion == nil {
		t.Fatalf("overview = %+v breadth=%v", overview, b)
	}

	signals, err := engine.Signals(ctx, SignalFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals.Bullish) == 0 || len(signals.Bearish) == 0 || len(signals.NotRanked) == 0 {
		t.Fatalf("signals bull=%d bear=%d notRanked=%d", len(signals.Bullish), len(signals.Bearish), len(signals.NotRanked))
	}
	for i := 1; i < len(signals.Bullish); i++ {
		if signals.Bullish[i].Rank != i+1 {
			t.Fatal("ranks are not contiguous")
		}
	}
	if _, err := engine.Signals(ctx, SignalFilter{Timeframe: "1h"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsupported timeframe err = %v", err)
	}
	filtered, _ := engine.Signals(ctx, SignalFilter{Chain: "solana"})
	for _, s := range append(filtered.Bullish, filtered.Bearish...) {
		if s.Chain != "solana" {
			t.Fatalf("chain filter leaked %s", s.Chain)
		}
	}

	top := signals.Bullish[0].AssetID
	detail, err := engine.AssetDetail(ctx, top)
	if err != nil || detail.LastPrice == nil || detail.Derivatives["openInterest"].Quality != "unavailable" {
		t.Fatalf("detail = %+v err=%v", detail, err)
	}
	structure, err := engine.Structure(ctx, top, "1d")
	if err != nil || len(structure.Candles) == 0 || structure.Timeframe != "1d" {
		t.Fatalf("structure err=%v candles=%d", err, len(structure.Candles))
	}
	if _, err := engine.Liquidity(ctx, top); err != nil {
		t.Fatal(err)
	}
	scenario, err := engine.Scenario(ctx, top)
	if err != nil || scenario.Status != "available" || scenario.Invalidation == nil {
		t.Fatalf("scenario = %+v err=%v", scenario, err)
	}
	if _, err := engine.AssetDetail(ctx, "nowhere:none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown asset err = %v", err)
	}

	history, err := engine.History(ctx, JournalFilter{})
	if err != nil || history.Total == 0 || len(history.Items) == 0 {
		t.Fatalf("history total=%d err=%v", history.Total, err)
	}
	backtests, err := engine.Backtests(ctx)
	if err != nil || len(backtests.Items) == 0 || backtests.Items[0].SampleSize == 0 {
		t.Fatalf("backtests = %+v err=%v", backtests, err)
	}
}

func TestEngineCachesAndServesStaleOnOutage(t *testing.T) {
	src := newFake()
	now := testNow
	engine := NewEngine(Config{Source: src, Now: func() time.Time { return now }})
	ctx := context.Background()
	tf := Timeframes[0]

	if _, err := engine.Get(ctx, tf); err != nil {
		t.Fatal(err)
	}
	calls := src.calls.Load()
	if _, err := engine.Get(ctx, tf); err != nil || src.calls.Load() != calls {
		t.Fatal("second read within TTL hit the upstream")
	}

	// Within 4x TTL the cached run is served immediately (refresh in background).
	now = now.Add(tf.TTL + time.Minute)
	if run, err := engine.Get(ctx, tf); err != nil || run.Quality != "live" {
		t.Fatalf("refresh-ahead run = %v err=%v", run, err)
	}

	// Past 4x TTL with the upstream down, the previous run is served as stale.
	src.down.Store(true)
	now = now.Add(4 * tf.TTL)
	run, err := engine.Get(ctx, tf)
	if err != nil || run.Quality != "stale" {
		t.Fatalf("outage run = %v err=%v, want stale previous run", run, err)
	}

	cold := NewEngine(Config{Source: src, Now: func() time.Time { return now }})
	if _, err := cold.Get(ctx, tf); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cold outage err = %v", err)
	}
}

func TestJournalDedupesAndPaginates(t *testing.T) {
	j := NewMemoryJournal()
	ctx := context.Background()
	entry := JournalEntry{AssetID: "near:near", Symbol: "NEAR", Direction: "bullish", Timeframe: "4h", BarTime: testNow, GeneratedAt: testNow}
	_ = j.Record(ctx, []JournalEntry{entry, entry})
	other := entry
	other.Direction, other.GeneratedAt = "bearish", testNow.Add(time.Hour)
	_ = j.Record(ctx, []JournalEntry{other})
	items, total, _ := j.List(ctx, JournalFilter{PageSize: 1})
	if total != 2 || len(items) != 1 || items[0].Direction != "bearish" {
		t.Fatalf("items=%+v total=%d", items, total)
	}
	items, total, _ = j.List(ctx, JournalFilter{Direction: "bullish", Asset: "near"})
	if total != 1 || items[0].Direction != "bullish" {
		t.Fatalf("filtered = %+v", items)
	}
}

func TestJournalDateRange(t *testing.T) {
	j := NewMemoryJournal()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		at := testNow.Add(time.Duration(i) * 24 * time.Hour)
		_ = j.Record(ctx, []JournalEntry{{AssetID: "near:near", Symbol: "NEAR", Direction: "bullish", Timeframe: "4h", BarTime: at, GeneratedAt: at}})
	}
	_, total, _ := j.List(ctx, JournalFilter{From: testNow.Add(24 * time.Hour), To: testNow.Add(48 * time.Hour)})
	if total != 1 {
		t.Fatalf("range total = %d, want 1", total)
	}
}

// TestBacktestHasNoLookAhead checks that a trade is graded only on bars after
// its entry and only at a price that traded: on a pure random-walk-like
// oscillation with no drift, results must not be strongly positive.
func TestBacktestHasNoLookAhead(t *testing.T) {
	src := &fakeSource{trend: map[string]float64{}, fail: map[string]bool{}}
	for _, asset := range Universe {
		src.trend[asset.CoinKey] = 0 // pure oscillation, no edge to find
	}
	run, err := newTestEngine(src).Get(context.Background(), Timeframes[0])
	if err != nil {
		t.Fatal(err)
	}
	bt := RunBacktest(run)
	if bt.Trades > 0 && bt.AvgR > 1 {
		t.Fatalf("driftless series produced avg R %.2f over %d trades; grading is optimistic", bt.AvgR, bt.Trades)
	}
}
