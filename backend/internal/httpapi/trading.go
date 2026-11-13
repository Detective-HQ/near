package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"universal-defi-intelligence/internal/httpapi/middleware"
	"universal-defi-intelligence/internal/trading"
)

// tradingError maps engine errors onto the API envelope.
func (h *Handler) tradingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trading.ErrNotFound):
		middleware.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "This asset is not in the trading universe.")
	case errors.Is(err, trading.ErrInvalidInput):
		middleware.WriteError(w, r, http.StatusBadRequest, "VALIDATION_FAILED",
			"Unsupported parameter. Timeframes: "+strings.Join(trading.TimeframeKeys(), ", ")+".")
	case errors.Is(err, trading.ErrUnavailable):
		middleware.WriteError(w, r, http.StatusServiceUnavailable, "MARKET_DATA_UNAVAILABLE",
			"Price data is unavailable right now; try again shortly.")
	default:
		h.writeError(w, r, err)
	}
}

func (h *Handler) requireTrading(w http.ResponseWriter, r *http.Request) bool {
	if h.trading == nil {
		middleware.WriteError(w, r, http.StatusServiceUnavailable, "MARKET_DATA_UNAVAILABLE",
			"Trading analytics need live market data (MARKET_DATA_MODE=live).")
		return false
	}
	return true
}

func (h *Handler) TradingOverview(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	out, err := h.trading.Overview(r.Context())
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) TradingSignals(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	q := r.URL.Query()
	out, err := h.trading.Signals(r.Context(), trading.SignalFilter{
		Timeframe: q.Get("timeframe"), Chain: q.Get("chain"), Venue: q.Get("venue"),
		Quote: q.Get("quote"), LiquidityTier: q.Get("liquidityTier"), Asset: q.Get("asset"),
	})
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// TradingTopMovers lists the universe sorted by absolute 24h change.
func (h *Handler) TradingTopMovers(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	out, err := h.trading.Signals(r.Context(), trading.SignalFilter{Timeframe: r.URL.Query().Get("timeframe")})
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	movers := append(append([]trading.SignalDTO{}, out.Bullish...), out.Bearish...)
	abs := func(s trading.SignalDTO) float64 {
		if s.Change == nil {
			return 0
		}
		v, _ := strconv.ParseFloat(strings.TrimPrefix(*s.Change, "-"), 64)
		return v
	}
	for i := 1; i < len(movers); i++ {
		for j := i; j > 0 && abs(movers[j]) > abs(movers[j-1]); j-- {
			movers[j], movers[j-1] = movers[j-1], movers[j]
		}
	}
	if len(movers) > 10 {
		movers = movers[:10]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source": out.Source, "asOf": out.AsOf, "quality": out.Quality, "items": movers,
	})
}

func (h *Handler) TradingAsset(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	out, err := h.trading.AssetDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) TradingStructure(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	out, err := h.trading.Structure(r.Context(), r.PathValue("id"), r.URL.Query().Get("timeframe"))
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) TradingLiquidity(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	out, err := h.trading.Liquidity(r.Context(), r.PathValue("id"))
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) TradingScenario(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	out, err := h.trading.Scenario(r.Context(), r.PathValue("id"))
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) TradingHistory(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("pageSize"))
	direction := q.Get("direction")
	if direction != "" && direction != "bullish" && direction != "bearish" {
		h.tradingError(w, r, trading.ErrInvalidInput)
		return
	}
	if tf := q.Get("timeframe"); tf != "" {
		if _, ok := trading.FindTimeframe(tf); !ok {
			h.tradingError(w, r, trading.ErrInvalidInput)
			return
		}
	}
	filter := trading.JournalFilter{
		Asset: q.Get("asset"), Direction: direction, Timeframe: q.Get("timeframe"), Page: page, PageSize: size,
	}
	for key, dst := range map[string]*time.Time{"from": &filter.From, "to": &filter.To} {
		raw := q.Get(key)
		if raw == "" {
			continue
		}
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			if parsed, err = time.Parse(time.RFC3339, raw); err != nil {
				h.tradingError(w, r, trading.ErrInvalidInput)
				return
			}
		} else if key == "to" {
			parsed = parsed.Add(24 * time.Hour) // a "to" date includes that whole day
		}
		*dst = parsed
	}
	out, err := h.trading.History(r.Context(), filter)
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	if outcome := q.Get("outcome"); outcome != "" {
		filtered := out.Items[:0]
		for _, item := range out.Items {
			if item.Outcome == outcome {
				filtered = append(filtered, item)
			}
		}
		out.Items = filtered
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) TradingBacktests(w http.ResponseWriter, r *http.Request) {
	if !h.requireTrading(w, r) {
		return
	}
	out, err := h.trading.Backtests(r.Context())
	if err != nil {
		h.tradingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
