package trading

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// The types below mirror frontend/src/lib/trading-contracts.ts. Numbers are
// decimal strings and timestamps ISO-8601 UTC, as the contract requires.

type Provenance struct {
	Source  string  `json:"source"`
	AsOf    *string `json:"asOf"`
	Quality string  `json:"quality"`
}

type MetricValue struct {
	Provenance
	Value *string `json:"value"`
	Unit  string  `json:"unit"`
}

type OverviewResponse struct {
	Provenance
	Regime               *string         `json:"regime"`
	RegimeMethodologyURL *string         `json:"regimeMethodologyUrl"`
	Breadth              *map[string]int `json:"breadth"`
	Universe             *map[string]int `json:"universe"`
	Coverage             *string         `json:"coverage"`
	ModelVersion         *string         `json:"modelVersion"`
	Timeframe            string          `json:"timeframe"`
}

type SignalDTO struct {
	Provenance
	AssetID       string  `json:"assetId"`
	Symbol        string  `json:"symbol"`
	Chain         string  `json:"chain"`
	Venue         string  `json:"venue"`
	Quote         string  `json:"quote"`
	LiquidityTier string  `json:"liquidityTier"`
	Direction     string  `json:"direction"`
	Rank          int     `json:"rank"`
	RankChange    *int    `json:"rankChange"`
	Score         string  `json:"score"`
	Change        *string `json:"change"`
	Coverage      *string `json:"coverage"`
	Timeframe     string  `json:"timeframe"`
	Suppressed    bool    `json:"suppressed"`
}

type NotRanked struct {
	AssetID string `json:"assetId"`
	Symbol  string `json:"symbol"`
	Chain   string `json:"chain"`
	Reason  string `json:"reason"`
}

type SignalsResponse struct {
	Provenance
	ModelVersion string              `json:"modelVersion"`
	Bullish      []SignalDTO         `json:"bullish"`
	Bearish      []SignalDTO         `json:"bearish"`
	NotRanked    []NotRanked         `json:"notRanked"`
	Filters      map[string][]string `json:"filters"`
}

type EvidenceDTO struct {
	Type       string  `json:"type"`
	Value      string  `json:"value"`
	Source     string  `json:"source"`
	ObservedAt *string `json:"observedAt"`
}

type AssetDetailResponse struct {
	Provenance
	AssetID     string                  `json:"assetId"`
	Symbol      string                  `json:"symbol"`
	Chain       string                  `json:"chain"`
	Venues      []string                `json:"venues"`
	LastPrice   *string                 `json:"lastPrice"`
	Change24h   *string                 `json:"change24h"`
	Scores      map[string]*string      `json:"scores"`
	Evidence    []EvidenceDTO           `json:"evidence"`
	Derivatives map[string]*MetricValue `json:"derivatives"`
	Risk        map[string]*MetricValue `json:"risk"`
	Timeframes  []string                `json:"timeframes"`
}

type CandleDTO struct {
	T string `json:"t"`
	O string `json:"o"`
	H string `json:"h"`
	L string `json:"l"`
	C string `json:"c"`
}

type ZoneDTO struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Direction string  `json:"direction"`
	Status    string  `json:"status"`
	Top       string  `json:"top"`
	Bottom    string  `json:"bottom"`
	From      string  `json:"from"`
	To        *string `json:"to"`
}

type BreakDTO struct {
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	Price     string `json:"price"`
	At        string `json:"at"`
}

type SwingDTO struct {
	Kind  string `json:"kind"`
	Price string `json:"price"`
	At    string `json:"at"`
}

type StructureResponse struct {
	Provenance
	Timeframe    string      `json:"timeframe"`
	RulesVersion string      `json:"rulesVersion"`
	Candles      []CandleDTO `json:"candles"`
	Zones        []ZoneDTO   `json:"zones"`
	Structure    []BreakDTO  `json:"structure"`
	Swings       []SwingDTO  `json:"swings"`
}

type LevelDTO struct {
	Price string `json:"price"`
	Side  string `json:"side"`
	Label string `json:"label"`
}

type LiquidityResponse struct {
	Provenance
	Levels     []LevelDTO   `json:"levels"`
	SpotVolume *MetricValue `json:"spotVolume"`
	CVD        *MetricValue `json:"cvd"`
	Depth      *Provenance  `json:"depth"`
	Notice     string       `json:"notice"`
}

type RangeDTO struct {
	Low  string `json:"low"`
	High string `json:"high"`
}

type ScenarioResponse struct {
	Provenance
	Status         string    `json:"status"`
	ModelVersion   *string   `json:"modelVersion"`
	ReferencePrice *string   `json:"referencePrice"`
	EntryArea      *RangeDTO `json:"entryArea"`
	Target1        *string   `json:"target1"`
	Target2        *string   `json:"target2"`
	Invalidation   *string   `json:"invalidation"`
	RiskReward     *string   `json:"riskReward"`
	Distance       *string   `json:"distance"`
	ExpiresAt      *string   `json:"expiresAt"`
	Timeframe      string    `json:"timeframe"`
}

type TimelineDTO struct {
	At    string `json:"at"`
	Label string `json:"label"`
}

type HistoryItemDTO struct {
	ID           string        `json:"id"`
	GeneratedAt  string        `json:"generatedAt"`
	AssetID      string        `json:"assetId"`
	Symbol       string        `json:"symbol"`
	Chain        string        `json:"chain"`
	Direction    string        `json:"direction"`
	Score        string        `json:"score"`
	Timeframe    string        `json:"timeframe"`
	ModelVersion string        `json:"modelVersion"`
	Outcome      string        `json:"outcome"`
	MFE          *string       `json:"mfe"`
	MAE          *string       `json:"mae"`
	Timeline     []TimelineDTO `json:"timeline"`
}

type AggregateDTO struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Methodology string `json:"methodology"`
	SampleSize  int    `json:"sampleSize"`
}

type HistoryResponse struct {
	Provenance
	Items     []HistoryItemDTO    `json:"items"`
	Page      int                 `json:"page"`
	PageSize  int                 `json:"pageSize"`
	Total     int                 `json:"total"`
	Filters   map[string][]string `json:"filters"`
	Aggregate *AggregateDTO       `json:"aggregate"`
}

type MetricPair struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type BacktestDTO struct {
	ID             string            `json:"id"`
	Model          string            `json:"model"`
	Period         map[string]string `json:"period"`
	Universe       string            `json:"universe"`
	Timeframe      string            `json:"timeframe"`
	SampleSize     int               `json:"sampleSize"`
	Costs          string            `json:"costs"`
	Benchmark      string            `json:"benchmark"`
	MaxDrawdown    *string           `json:"maxDrawdown"`
	Metrics        []MetricPair      `json:"metrics"`
	Methodology    string            `json:"methodology"`
	CostsAssumed   string            `json:"costsAssumed"`
	MethodologyURL *string           `json:"methodologyUrl"`
}

type BacktestsResponse struct {
	Provenance
	Items []BacktestDTO `json:"items"`
}

// ---- builders ----------------------------------------------------------------

func iso(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func isoPtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := iso(t)
	return &s
}

func sp(s string) *string { return &s }

func (e *Engine) provenance(run *Run) Provenance {
	return Provenance{Source: e.sourceName(), AsOf: isoPtr(run.At), Quality: run.Quality}
}

func (e *Engine) sourceName() string {
	if e.source == nil {
		return "unavailable"
	}
	return e.source.Name()
}

func unavailableMetric(source, unit string) *MetricValue {
	return &MetricValue{Provenance: Provenance{Source: source, Quality: "unavailable"}, Unit: unit}
}

// Overview summarizes the default-timeframe run.
func (e *Engine) Overview(ctx context.Context) (OverviewResponse, error) {
	tf, _ := FindTimeframe(DefaultTimeframe)
	run, err := e.Get(ctx, tf)
	if err != nil {
		return OverviewResponse{}, err
	}
	breadth := map[string]int{"bullish": 0, "neutral": 0, "bearish": 0}
	scanned := 0
	var coverageSum float64
	for _, a := range run.Assets {
		if !a.Assessment.Ranked {
			continue
		}
		scanned++
		coverageSum += a.Assessment.Coverage
		breadth[a.Assessment.Direction]++
	}
	universe := map[string]int{"scanned": scanned, "total": len(run.Assets)}
	out := OverviewResponse{
		Provenance: e.provenance(run), Universe: &universe, Breadth: &breadth,
		RegimeMethodologyURL: sp(MethodologyURL), ModelVersion: sp(ModelVersion), Timeframe: tf.Key,
	}
	if scanned > 0 {
		out.Coverage = sp(dec(coverageSum/float64(scanned), 1))
		regime := "mixed"
		if float64(breadth["bullish"]) >= 0.6*float64(scanned) {
			regime = "bullish"
		} else if float64(breadth["bearish"]) >= 0.6*float64(scanned) {
			regime = "bearish"
		}
		out.Regime = &regime
	}
	return out, nil
}

// SignalFilter narrows the signal lists.
type SignalFilter struct {
	Timeframe, Chain, Venue, Quote, LiquidityTier, Asset string
}

// Signals returns ranked bullish/bearish lists.
func (e *Engine) Signals(ctx context.Context, f SignalFilter) (SignalsResponse, error) {
	tf, ok := FindTimeframe(f.Timeframe)
	if !ok {
		return SignalsResponse{}, fmt.Errorf("%w: unsupported timeframe %q", ErrInvalidInput, f.Timeframe)
	}
	run, err := e.Get(ctx, tf)
	if err != nil {
		return SignalsResponse{}, err
	}
	prov := e.provenance(run)
	match := func(a Asset) bool {
		return (f.Chain == "" || a.Chain == f.Chain) &&
			(f.Venue == "" || f.Venue == Venue) &&
			(f.Quote == "" || f.Quote == Quote) &&
			(f.LiquidityTier == "" || a.Tier == f.LiquidityTier) &&
			(f.Asset == "" || strings.Contains(strings.ToLower(a.Symbol), strings.ToLower(f.Asset)) || strings.EqualFold(a.ID, f.Asset))
	}
	build := func(dir string) []SignalDTO {
		list := []SignalDTO{}
		for _, a := range run.Signals(dir) {
			if !match(a.Asset) {
				continue
			}
			dto := SignalDTO{
				Provenance:    Provenance{Source: prov.Source, AsOf: isoPtr(a.LastSample), Quality: assetQuality(a)},
				AssetID:       a.Asset.ID,
				Symbol:        a.Asset.Symbol,
				Chain:         a.Asset.Chain,
				Venue:         Venue,
				Quote:         Quote,
				LiquidityTier: a.Asset.Tier,
				Direction:     dir,
				Rank:          len(list) + 1,
				RankChange:    run.RankDelta[dir+"|"+a.Asset.ID],
				Score:         dec(a.Assessment.Score, 1),
				Coverage:      sp(dec(a.Assessment.Coverage, 1)),
				Timeframe:     tf.Key,
				Suppressed:    a.Stale,
			}
			if a.Change24h != nil {
				dto.Change = sp(dec(*a.Change24h, 2))
			}
			list = append(list, dto)
		}
		return list
	}
	notRanked := []NotRanked{}
	chains := map[string]bool{}
	for _, a := range run.Assets {
		chains[a.Asset.Chain] = true
		if !a.Assessment.Ranked && match(a.Asset) {
			reason := a.Assessment.Reason
			if reason == "" {
				reason = "Not enough data to rank."
			}
			notRanked = append(notRanked, NotRanked{AssetID: a.Asset.ID, Symbol: a.Asset.Symbol, Chain: a.Asset.Chain, Reason: reason})
		}
	}
	chainList := make([]string, 0, len(chains))
	for c := range chains {
		chainList = append(chainList, c)
	}
	sort.Strings(chainList)
	return SignalsResponse{
		Provenance:   prov,
		ModelVersion: ModelVersion,
		Bullish:      build("bullish"),
		Bearish:      build("bearish"),
		NotRanked:    notRanked,
		Filters: map[string][]string{
			"timeframe":     TimeframeKeys(),
			"chain":         chainList,
			"venue":         {Venue},
			"quote":         {Quote},
			"liquidityTier": LiquidityTiers,
		},
	}, nil
}

func assetQuality(a AssetRun) string {
	switch {
	case a.Err != nil:
		return "unavailable"
	case a.Stale:
		return "stale"
	case a.Assessment.Coverage < 100:
		return "partial"
	default:
		return "live"
	}
}

// ErrInvalidInput marks bad request parameters.
var ErrInvalidInput = fmt.Errorf("invalid trading request")

// ErrNotFound marks an unknown asset.
var ErrNotFound = fmt.Errorf("trading asset not found")

func (e *Engine) assetRun(ctx context.Context, assetID, timeframe string) (*Run, AssetRun, error) {
	asset, ok := FindAsset(assetID)
	if !ok {
		return nil, AssetRun{}, ErrNotFound
	}
	tf, ok := FindTimeframe(timeframe)
	if !ok {
		return nil, AssetRun{}, fmt.Errorf("%w: unsupported timeframe %q", ErrInvalidInput, timeframe)
	}
	run, err := e.Get(ctx, tf)
	if err != nil {
		return nil, AssetRun{}, err
	}
	a, _ := run.Find(asset.ID)
	if a.Err != nil || len(a.Candles) == 0 {
		return run, a, ErrUnavailable
	}
	return run, a, nil
}

// AssetDetail returns the asset view on the default timeframe.
func (e *Engine) AssetDetail(ctx context.Context, assetID string) (AssetDetailResponse, error) {
	run, a, err := e.assetRun(ctx, assetID, DefaultTimeframe)
	if err != nil {
		return AssetDetailResponse{}, err
	}
	source := e.sourceName()
	observed := isoPtr(a.LastSample)
	out := AssetDetailResponse{
		Provenance: Provenance{Source: source, AsOf: observed, Quality: assetQuality(a)},
		AssetID:    a.Asset.ID, Symbol: a.Asset.Symbol, Chain: a.Asset.Chain,
		Venues:     []string{Venue},
		LastPrice:  sp(FormatPrice(a.Candles[len(a.Candles)-1].C)),
		Scores:     map[string]*string{"bullish": nil, "bearish": nil, "coverage": sp(dec(a.Assessment.Coverage, 1)), "modelVersion": sp(ModelVersion)},
		Evidence:   []EvidenceDTO{},
		Timeframes: TimeframeKeys(),
		Derivatives: map[string]*MetricValue{
			"openInterest":  unavailableMetric("No derivatives source connected", "USD"),
			"fundingRate":   unavailableMetric("No derivatives source connected", "% / 8h"),
			"liquidations":  unavailableMetric("No derivatives source connected", "USD / 24h"),
			"futuresVolume": unavailableMetric("No derivatives source connected", "USD / 24h"),
		},
	}
	if a.Change24h != nil {
		out.Change24h = sp(dec(*a.Change24h, 2))
	}
	if a.Assessment.Ranked {
		bull := (a.Assessment.Composite + 1) / 2 * 100
		out.Scores["bullish"] = sp(dec(bull, 1))
		out.Scores["bearish"] = sp(dec(100-bull, 1))
		for _, ev := range a.Assessment.Evidence {
			out.Evidence = append(out.Evidence, EvidenceDTO{Type: ev.Type, Value: ev.Value, Source: source, ObservedAt: observed})
		}
	}
	freshness := run.At.Sub(a.LastSample).Minutes()
	out.Risk = map[string]*MetricValue{
		"volatility": {Provenance: Provenance{Source: source, AsOf: observed, Quality: assetQuality(a)},
			Value: sp(dec(Volatility(a.Candles, run.Timeframe.Bar), 1)), Unit: "% annualized"},
		"dataFreshness": {Provenance: Provenance{Source: source, AsOf: observed, Quality: assetQuality(a)},
			Value: sp(dec(math.Max(0, freshness), 0)), Unit: "minutes since last sample"},
		"spreadDepth":        unavailableMetric("No order-book source connected", "bps"),
		"venueConcentration": unavailableMetric("Single aggregate source", "%"),
	}
	return out, nil
}

// Structure returns candles, zones, breaks and swings.
func (e *Engine) Structure(ctx context.Context, assetID, timeframe string) (StructureResponse, error) {
	run, a, err := e.assetRun(ctx, assetID, timeframe)
	if err != nil {
		return StructureResponse{}, err
	}
	st := a.Assessment.Structure
	if !a.Assessment.Ranked {
		st = AnalyzeStructure(a.Candles)
	}
	out := StructureResponse{
		Provenance:   Provenance{Source: e.sourceName() + " (OHLC approximated from sampled prices)", AsOf: isoPtr(a.LastSample), Quality: assetQuality(a)},
		Timeframe:    run.Timeframe.Key,
		RulesVersion: RulesVersion,
		Candles:      make([]CandleDTO, 0, len(a.Candles)),
		Zones:        []ZoneDTO{},
		Structure:    []BreakDTO{},
		Swings:       []SwingDTO{},
	}
	for _, c := range a.Candles {
		out.Candles = append(out.Candles, CandleDTO{T: iso(c.T), O: FormatPrice(c.O), H: FormatPrice(c.H), L: FormatPrice(c.L), C: FormatPrice(c.C)})
	}
	for _, z := range st.LatestZones() {
		dto := ZoneDTO{ID: z.ID, Kind: z.Kind, Direction: z.Direction, Status: z.Status,
			Top: FormatPrice(z.Top), Bottom: FormatPrice(z.Bottom), From: iso(z.From)}
		if z.To != nil {
			dto.To = isoPtr(*z.To)
		}
		out.Zones = append(out.Zones, dto)
	}
	for _, b := range st.Breaks {
		out.Structure = append(out.Structure, BreakDTO{Kind: b.Kind, Direction: b.Direction, Price: FormatPrice(b.Price), At: iso(b.At)})
	}
	for _, s := range st.Swings {
		out.Swings = append(out.Swings, SwingDTO{Kind: s.Kind, Price: FormatPrice(s.Price), At: iso(s.At)})
	}
	return out, nil
}

// Liquidity lists resting-liquidity levels at unswept swing highs and lows.
func (e *Engine) Liquidity(ctx context.Context, assetID string) (LiquidityResponse, error) {
	_, a, err := e.assetRun(ctx, assetID, DefaultTimeframe)
	if err != nil {
		return LiquidityResponse{}, err
	}
	st := a.Assessment.Structure
	if !a.Assessment.Ranked {
		st = AnalyzeStructure(a.Candles)
	}
	levels := []LevelDTO{}
	for _, s := range st.Swings {
		swept := false
		for _, c := range a.Candles[s.Index+1:] {
			if (s.Kind == "high" && c.H > s.Price) || (s.Kind == "low" && c.L < s.Price) {
				swept = true
				break
			}
		}
		if swept {
			continue
		}
		if s.Kind == "high" {
			levels = append(levels, LevelDTO{Price: FormatPrice(s.Price), Side: "ask", Label: "Buy-side liquidity above unswept swing high"})
		} else {
			levels = append(levels, LevelDTO{Price: FormatPrice(s.Price), Side: "bid", Label: "Sell-side liquidity below unswept swing low"})
		}
	}
	source := e.sourceName()
	return LiquidityResponse{
		Provenance: Provenance{Source: source, AsOf: isoPtr(a.LastSample), Quality: assetQuality(a)},
		Levels:     levels,
		SpotVolume: unavailableMetric("Aggregate price source has no volume", "USD / 24h"),
		CVD:        nil,
		Depth:      nil,
		Notice:     "Levels are structural (unswept swing highs/lows). Volume, CVD and order-book depth need a venue data source and are not estimated.",
	}, nil
}

// Scenario frames the current rule-based plan.
func (e *Engine) Scenario(ctx context.Context, assetID string) (ScenarioResponse, error) {
	run, a, err := e.assetRun(ctx, assetID, DefaultTimeframe)
	if err != nil {
		return ScenarioResponse{}, err
	}
	out := ScenarioResponse{
		Provenance: Provenance{Source: e.sourceName(), AsOf: isoPtr(a.LastSample), Quality: assetQuality(a)},
		Status:     "none", ModelVersion: sp(ModelVersion), Timeframe: run.Timeframe.Key,
	}
	plan, ok := BuildPlan(a.Assessment)
	if !ok {
		return out, nil
	}
	out.Status = "available"
	if a.Stale {
		out.Status = "stale"
	}
	last := a.Candles[len(a.Candles)-1]
	out.ReferencePrice = sp(FormatPrice(last.C))
	out.EntryArea = &RangeDTO{Low: FormatPrice(plan.EntryLow), High: FormatPrice(plan.EntryHigh)}
	out.Target1 = sp(FormatPrice(plan.Target1))
	out.Target2 = sp(FormatPrice(plan.Target2))
	out.Invalidation = sp(FormatPrice(plan.Stop))
	out.RiskReward = sp(dec(plan.RiskReward, 1))
	distance := 0.0
	if last.C > plan.EntryHigh {
		distance = (last.C - plan.EntryHigh) / last.C * 100
	} else if last.C < plan.EntryLow {
		distance = (plan.EntryLow - last.C) / last.C * 100
	}
	out.Distance = sp(dec(distance, 2))
	out.ExpiresAt = isoPtr(last.T.Add(run.Timeframe.Bar * time.Duration(run.Timeframe.Horizon+1)))
	return out, nil
}

// History lists journal entries with outcomes graded on current candles.
func (e *Engine) History(ctx context.Context, f JournalFilter) (HistoryResponse, error) {
	f.normalize()
	entries, total, err := e.journal.List(ctx, f)
	if err != nil {
		return HistoryResponse{}, err
	}
	runs := map[string]*Run{}
	quality := "live"
	for _, tf := range Timeframes {
		if run, runErr := e.Get(ctx, tf); runErr == nil {
			runs[tf.Key] = run
		} else {
			quality = "partial"
		}
	}
	items := make([]HistoryItemDTO, 0, len(entries))
	var closed, wins int
	for _, entry := range entries {
		item := HistoryItemDTO{
			ID: entry.ID, GeneratedAt: iso(entry.GeneratedAt), AssetID: entry.AssetID, Symbol: entry.Symbol,
			Chain: entry.Chain, Direction: entry.Direction, Score: dec(entry.Score, 1), Timeframe: entry.Timeframe,
			ModelVersion: entry.ModelVersion, Outcome: "open",
			Timeline: []TimelineDTO{{At: iso(entry.GeneratedAt), Label: fmt.Sprintf("Signal published at %s (stop %s, target %s)",
				FormatPrice(entry.Entry), FormatPrice(entry.Stop), FormatPrice(entry.Target2))}},
		}
		if run, ok := runs[entry.Timeframe]; ok {
			if a, found := run.Find(entry.AssetID); found {
				idx := -1
				for i, c := range a.Candles {
					if c.T.Equal(entry.BarTime) {
						idx = i
						break
					}
				}
				if idx >= 0 {
					plan := Plan{Direction: entry.Direction, Entry: entry.Entry, Stop: entry.Stop, Target2: entry.Target2}
					oc := Evaluate(plan, a.Candles, idx, run.Timeframe.Horizon)
					item.Outcome = oc.Result
					item.MFE, item.MAE = sp(dec(oc.MFE, 2)), sp(dec(oc.MAE, 2))
					if oc.Result != "open" {
						item.Timeline = append(item.Timeline, TimelineDTO{At: iso(oc.At), Label: outcomeLabel(oc.Result)})
						closed++
						if oc.Result == "target_hit" {
							wins++
						}
					}
				}
			}
		}
		items = append(items, item)
	}
	resp := HistoryResponse{
		Provenance: Provenance{Source: "Signal journal (" + e.sourceName() + " outcomes)", AsOf: isoPtr(e.now()), Quality: quality},
		Items:      items, Page: f.Page, PageSize: f.PageSize, Total: total,
		Filters: map[string][]string{"timeframe": TimeframeKeys()},
	}
	if closed >= 30 {
		resp.Aggregate = &AggregateDTO{
			Label:       "Target hit rate (this page, closed signals)",
			Value:       dec(float64(wins)/float64(closed)*100, 1) + "%",
			Methodology: "Share of closed signals on this page that reached the 3R target before the stop; expired signals count as misses.",
			SampleSize:  closed,
		}
	}
	return resp, nil
}

func outcomeLabel(result string) string {
	switch result {
	case "target_hit":
		return "Target reached"
	case "stop_hit":
		return "Stop reached"
	default:
		return "Expired without reaching target or stop"
	}
}

// Backtests returns a walk-forward backtest per served timeframe.
func (e *Engine) Backtests(ctx context.Context) (BacktestsResponse, error) {
	resp := BacktestsResponse{Provenance: Provenance{Source: e.sourceName(), AsOf: isoPtr(e.now()), Quality: "live"}, Items: []BacktestDTO{}}
	for _, tf := range Timeframes {
		run, err := e.Get(ctx, tf)
		if err != nil {
			resp.Quality = "partial"
			continue
		}
		bt := run.Backtest()
		if bt.Trades == 0 {
			continue
		}
		dto := BacktestDTO{
			ID:          fmt.Sprintf("%s-%s", ModelVersion, tf.Key),
			Model:       ModelVersion,
			Period:      map[string]string{"from": iso(bt.From), "to": iso(bt.To)},
			Universe:    fmt.Sprintf("%d of %d universe assets with enough history", bt.Assets, len(Universe)),
			Timeframe:   tf.Key,
			SampleSize:  bt.Trades,
			Costs:       fmt.Sprintf("%d bps round trip", backtestCostBps),
			Benchmark:   fmt.Sprintf("Equal-weight buy and hold: %s%%", dec(bt.BenchReturn, 2)),
			MaxDrawdown: sp(dec(bt.MaxDDR, 2) + "R"),
			Metrics: []MetricPair{
				{Label: "Trades", Value: fmt.Sprint(bt.Trades)},
				{Label: "Target hits", Value: fmt.Sprint(bt.Wins)},
				{Label: "Stops", Value: fmt.Sprint(bt.Losses)},
				{Label: "Expired", Value: fmt.Sprint(bt.Expired)},
				{Label: "Average R (net)", Value: dec(bt.AvgR, 2)},
				{Label: "Total R (net)", Value: dec(bt.TotalR, 2)},
				{Label: "Profit factor", Value: dec(bt.ProfitFact, 2)},
			},
			Methodology:    backtestMethodology(tf),
			CostsAssumed:   fmt.Sprintf("%d bps of notional per round trip, converted to R per trade; no slippage beyond that.", backtestCostBps),
			MethodologyURL: sp(MethodologyURL),
		}
		resp.Items = append(resp.Items, dto)
	}
	if len(resp.Items) == 0 && resp.Quality == "partial" {
		return resp, ErrUnavailable
	}
	return resp, nil
}
