package trading

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"universal-defi-intelligence/internal/metrics"
)

// ErrUnavailable means no usable price data could be obtained.
var ErrUnavailable = errors.New("trading data is unavailable")

// AssetRun is one asset's data and assessment within a run.
type AssetRun struct {
	Asset      Asset
	Candles    []Candle
	Assessment Assessment
	LastSample time.Time
	Change24h  *float64
	Stale      bool
	Err        error
}

// Run is a complete scan of the universe on one timeframe.
type Run struct {
	Timeframe Timeframe
	At        time.Time
	Assets    []AssetRun
	Quality   string // live | partial | stale | unavailable
	Ranks     map[string]int
	RankDelta map[string]*int
	backtest  *BacktestResult
	btOnce    sync.Once
}

// Config configures the engine.
type Config struct {
	Source      PriceSource
	Journal     Journal
	Logger      *slog.Logger
	Now         func() time.Time
	Concurrency int
}

// Engine computes and caches runs per timeframe.
type Engine struct {
	source      PriceSource
	journal     Journal
	logger      *slog.Logger
	now         func() time.Time
	concurrency int

	mu        sync.RWMutex
	runs      map[string]*Run
	prevRanks map[string]map[string]int
	flight    singleflight.Group
}

// NewEngine creates an engine. A nil journal uses the in-memory journal.
func NewEngine(cfg Config) *Engine {
	if cfg.Journal == nil {
		cfg.Journal = NewMemoryJournal()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 8
	}
	return &Engine{
		source: cfg.Source, journal: cfg.Journal, logger: cfg.Logger, now: cfg.Now,
		concurrency: cfg.Concurrency, runs: map[string]*Run{}, prevRanks: map[string]map[string]int{},
	}
}

// Journal exposes the signal journal.
func (e *Engine) Journal() Journal { return e.journal }

// Get returns the run for tf.
//
// Freshness policy (stale-while-revalidate):
//   - younger than the TTL: served from cache;
//   - up to 4x the TTL: served from cache immediately while one background
//     refresh runs, so users never wait on a cold upstream scan;
//   - older, or no cache: computed synchronously (concurrent callers share one
//     computation). If that fails, the previous run is served marked "stale".
func (e *Engine) Get(ctx context.Context, tf Timeframe) (*Run, error) {
	e.mu.RLock()
	cached := e.runs[tf.Key]
	e.mu.RUnlock()
	if cached != nil {
		age := e.now().Sub(cached.At)
		if age < tf.TTL {
			return cached, nil
		}
		if age < 4*tf.TTL {
			e.refreshAsync(tf)
			return cached, nil
		}
	}
	result, err, _ := e.flight.Do(tf.Key, func() (any, error) {
		runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		return e.compute(runCtx, tf)
	})
	if err != nil {
		if cached != nil {
			e.logger.Warn("trading refresh failed; serving the previous run", "timeframe", tf.Key, "error", err)
			return cached.withQuality("stale"), nil
		}
		return nil, err
	}
	return result.(*Run), nil
}

// refreshAsync starts at most one background recomputation per timeframe.
func (e *Engine) refreshAsync(tf Timeframe) {
	go func() {
		_, err, _ := e.flight.Do(tf.Key, func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			return e.compute(ctx, tf)
		})
		if err != nil {
			e.logger.Warn("background trading refresh failed", "timeframe", tf.Key, "error", err)
		}
	}()
}

// Warm computes every timeframe in the background (used at startup).
func (e *Engine) Warm() {
	for _, tf := range Timeframes {
		e.refreshAsync(tf)
	}
}

// Refresh forces a recomputation of every timeframe (used by the worker).
func (e *Engine) Refresh(ctx context.Context) error {
	var errs []error
	for _, tf := range Timeframes {
		if _, err, _ := e.flight.Do(tf.Key, func() (any, error) { return e.compute(ctx, tf) }); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) compute(ctx context.Context, tf Timeframe) (run *Run, err error) {
	if e.source == nil {
		return nil, ErrUnavailable
	}
	started := time.Now()
	defer func() { metrics.ObserveTradingRefresh(tf.Key, err == nil, time.Since(started)) }()
	now := e.now()
	assets := make([]AssetRun, len(Universe))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(e.concurrency)
	for i, asset := range Universe {
		i, asset := i, asset
		group.Go(func() error {
			assets[i] = e.loadAsset(groupCtx, asset, tf, now)
			return nil
		})
	}
	_ = group.Wait()

	ok := 0
	for _, a := range assets {
		if a.Err == nil && len(a.Candles) > 0 {
			ok++
		}
	}
	if ok == 0 {
		return nil, ErrUnavailable
	}
	run = &Run{Timeframe: tf, At: now, Assets: assets, Quality: "live"}
	if ok < len(assets) {
		run.Quality = "partial"
	}
	e.rank(run)

	e.mu.Lock()
	e.runs[tf.Key] = run
	e.mu.Unlock()

	if err := e.journal.Record(ctx, journalEntries(run)); err != nil {
		e.logger.Warn("record trading signals", "timeframe", tf.Key, "error", err)
	}
	return run, nil
}

func (e *Engine) loadAsset(ctx context.Context, asset Asset, tf Timeframe, now time.Time) AssetRun {
	out := AssetRun{Asset: asset}
	points, err := e.source.History(ctx, asset.CoinKey, tf.Sample, tf.Points)
	if err != nil {
		out.Err = err
		out.Assessment = Assessment{Direction: "neutral", Reason: "Price data could not be retrieved."}
		return out
	}
	if len(points) > 0 {
		out.LastSample = points[len(points)-1].At
		out.Stale = now.Sub(out.LastSample) > 3*tf.Sample
		out.Change24h = change24h(points)
	}
	minSamples := int(tf.Bar/tf.Sample) - 1
	out.Candles = BuildCandles(points, tf.Bar, minSamples, now)
	out.Assessment = Assess(out.Candles, tf)
	if out.Stale && out.Assessment.Ranked {
		out.Assessment.Ranked = false
		out.Assessment.Reason = "Price data is stale."
	}
	return out
}

func change24h(points []PricePoint) *float64 {
	last := points[len(points)-1]
	target := last.At.Add(-24 * time.Hour)
	for i := len(points) - 1; i >= 0; i-- {
		if !points[i].At.After(target) {
			v := (last.Price/points[i].Price - 1) * 100
			return &v
		}
	}
	return nil
}

// rank orders bullish and bearish signals by score and records the rank
// change against the previous run of the same timeframe.
func (e *Engine) rank(run *Run) {
	run.Ranks = map[string]int{}
	run.RankDelta = map[string]*int{}
	for _, dir := range []string{"bullish", "bearish"} {
		list := run.Signals(dir)
		for i, a := range list {
			run.Ranks[dir+"|"+a.Asset.ID] = i + 1
		}
	}
	e.mu.Lock()
	prev := e.prevRanks[run.Timeframe.Key]
	for key, rank := range run.Ranks {
		if old, ok := prev[key]; ok {
			delta := old - rank
			run.RankDelta[key] = &delta
		}
	}
	e.prevRanks[run.Timeframe.Key] = run.Ranks
	e.mu.Unlock()
}

func (r *Run) withQuality(quality string) *Run {
	return &Run{Timeframe: r.Timeframe, At: r.At, Assets: r.Assets, Quality: quality, Ranks: r.Ranks, RankDelta: r.RankDelta}
}

// Signals returns ranked assets of one direction, strongest first.
func (r *Run) Signals(direction string) []AssetRun {
	var out []AssetRun
	for _, a := range r.Assets {
		if a.Assessment.Ranked && a.Assessment.Direction == direction {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Assessment.Score > out[j].Assessment.Score })
	return out
}

// Find returns an asset's run data.
func (r *Run) Find(id string) (AssetRun, bool) {
	for _, a := range r.Assets {
		if a.Asset.ID == id {
			return a, true
		}
	}
	return AssetRun{}, false
}

// journalEntries publishes the top five signals per direction. The bar time
// keys the entry, so recomputing within the same bar records nothing new.
func journalEntries(run *Run) []JournalEntry {
	var out []JournalEntry
	for _, dir := range []string{"bullish", "bearish"} {
		for i, a := range run.Signals(dir) {
			if i >= 5 {
				break
			}
			plan, ok := MarketPlan(a.Assessment)
			if !ok || len(a.Candles) == 0 {
				continue
			}
			out = append(out, JournalEntry{
				GeneratedAt:  run.At,
				BarTime:      a.Candles[len(a.Candles)-1].T,
				AssetID:      a.Asset.ID,
				Symbol:       a.Asset.Symbol,
				Chain:        a.Asset.Chain,
				Direction:    dir,
				Timeframe:    run.Timeframe.Key,
				ModelVersion: ModelVersion,
				Score:        a.Assessment.Score,
				Entry:        plan.Entry,
				Stop:         plan.Stop,
				Target1:      plan.Target1,
				Target2:      plan.Target2,
			})
		}
	}
	return out
}
