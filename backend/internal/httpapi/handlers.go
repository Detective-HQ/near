package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	analyticsservice "universal-defi-intelligence/internal/analytics/service"
	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
	"universal-defi-intelligence/internal/chains/stellar"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpapi/middleware"
	"universal-defi-intelligence/internal/portfolio"
	"universal-defi-intelligence/internal/research/scenarios"
	"universal-defi-intelligence/internal/risk"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/service"
	"universal-defi-intelligence/internal/trading"
	"universal-defi-intelligence/internal/wallets"
)

// Probe is one readiness dependency check.
type Probe struct {
	Name  string
	Check func(ctx context.Context) error
}

// HandlerOptions wires the API to its dependencies.
type HandlerOptions struct {
	Swaps   *service.Service
	Auth    *auth.Manager
	Risk    *risk.Engine
	Market  *analyticsservice.Reader // nil in fixture mode
	Trading *trading.Engine          // nil in fixture mode
	// Portfolio reads on-chain balances; nil disables /portfolio reads.
	Portfolio *portfolio.Reader
	Research  *scenarios.Engine
	Config    *config.Config
	Probes    []Probe
	Logger    *slog.Logger
	// SecureCookies marks the session cookie Secure (off only for local HTTP).
	SecureCookies bool
}

// Handler serves the /api/v1 surface.
type Handler struct {
	swaps         *service.Service
	auth          *auth.Manager
	risk          *risk.Engine
	market        *analyticsservice.Reader
	trading       *trading.Engine
	portfolio     *portfolio.Reader
	research      *scenarios.Engine
	cfg           *config.Config
	probes        []Probe
	logger        *slog.Logger
	secureCookies bool

	staticOnce   sync.Once
	chainsJSON   []byte
	researchJSON []byte

	// assetsCache holds encoded /assets bodies for the current catalogue
	// snapshot, keyed by chain filter. It is dropped whenever the catalogue
	// reloads, so it never serves stale assets.
	assetsMu       sync.RWMutex
	assetsLoadedAt time.Time
	assetsCache    map[string][]byte
}

// NewHandler validates and assembles a Handler.
func NewHandler(opts HandlerOptions) (*Handler, error) {
	if opts.Swaps == nil {
		return nil, errors.New("httpapi: swap service is required")
	}
	if opts.Auth == nil {
		opts.Auth = auth.NewManager(auth.Config{Registry: opts.Swaps.Registry()})
	}
	if opts.Risk == nil {
		opts.Risk = risk.NewEngine(risk.Config{})
	}
	if opts.Research == nil {
		opts.Research = scenarios.New()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Handler{
		swaps:         opts.Swaps,
		auth:          opts.Auth,
		risk:          opts.Risk,
		market:        opts.Market,
		trading:       opts.Trading,
		portfolio:     opts.Portfolio,
		research:      opts.Research,
		cfg:           opts.Config,
		probes:        opts.Probes,
		logger:        opts.Logger,
		secureCookies: opts.SecureCookies,
	}, nil
}

// ---- response helpers -------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeRawJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeError maps any error to the JSON envelope. Typed domain errors carry a
// client-safe message; anything else becomes a generic 500 and is logged.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if typed, ok := domain.AsError(err); ok {
		status := domain.HTTPStatusOf(typed)
		if typed.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(typed.RetryAfter.Seconds()+0.5)))
		}
		if status >= 500 {
			h.logger.Warn("request failed", "requestId", middleware.GetRequestID(r), "code", string(typed.Code), "error", err)
		}
		middleware.WriteError(w, r, status, string(typed.Code), typed.Error())
		return
	}
	h.logger.Error("unhandled error", "requestId", middleware.GetRequestID(r), "path", r.URL.Path, "error", err)
	middleware.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "An unexpected internal error occurred.")
}

// decodeJSON reads a bounded JSON body into dst.
func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return domain.New(domain.CodeInvalidJSON, "A JSON request body is required.")
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return domain.New(domain.CodeBodyTooLarge, "Request body is too large.")
		}
		if errors.Is(err, io.EOF) {
			return domain.New(domain.CodeInvalidJSON, "A JSON request body is required.")
		}
		return domain.New(domain.CodeInvalidJSON, "The request body is not valid JSON.")
	}
	if decoder.More() {
		return domain.New(domain.CodeInvalidJSON, "The request body must contain a single JSON object.")
	}
	return nil
}

// ---- probes and status -----------------------------------------------------

// Healthz is the liveness probe: the process is up and serving.
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeRawJSON(w, http.StatusOK, []byte(`{"status":"ok"}`))
}

// Readyz checks every dependency concurrently with a short deadline.
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	results := make(map[string]string, len(h.probes))
	var mu sync.Mutex
	ready := true
	var group errgroup.Group
	for _, probe := range h.probes {
		probe := probe
		group.Go(func() error {
			status := "ok"
			if err := probe.Check(ctx); err != nil {
				status = "failed"
				h.logger.Warn("readiness probe failed", "probe", probe.Name, "error", err)
			}
			mu.Lock()
			results[probe.Name] = status
			if status != "ok" {
				ready = false
			}
			mu.Unlock()
			return nil
		})
	}
	_ = group.Wait()

	report := h.risk.Status()
	statusCode, overall := http.StatusOK, "ready"
	if !ready {
		statusCode, overall = http.StatusServiceUnavailable, "not_ready"
	}
	writeJSON(w, statusCode, map[string]any{
		"status":     overall,
		"checks":     results,
		"riskStatus": report.Status,
	})
}

// Health is the frontend-facing capability report.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	quoteMode := h.swaps.QuoteMode()
	marketMode := config.MarketDataFixture
	if h.market != nil {
		marketMode = config.MarketDataLive
	}
	execution := h.swaps.LiveExecutionEnabled()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "healthy",
		"mode":                 string(quoteMode),
		"quoteMode":            string(quoteMode),
		"demoMode":             quoteMode == config.QuoteModeDemo,
		"liveQuotesConfigured": quoteMode == config.QuoteModeLive,
		"marketDataMode":       string(marketMode),
		"executionEnabled":     execution,
		"authRequired":         h.auth.AuthRequired(),
		"capabilityFlags": map[string]bool{
			"executionEnabled": execution,
			"demoEnabled":      quoteMode == config.QuoteModeDemo,
		},
		"capturedAt":  time.Now().UTC().Format(time.RFC3339),
		"fixtureDate": FixtureDate,
	})
}

// RiskStatus reports the live operational risk state.
func (h *Handler) RiskStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.risk.Status())
}

// ---- auth ------------------------------------------------------------------

// AuthNonce issues a single-use sign-in nonce.
func (h *Handler) AuthNonce(w http.ResponseWriter, r *http.Request) {
	nonce, err := h.auth.GenerateNonce(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"nonce":   nonce,
		"message": auth.SignInMessage(nonce),
	})
}

// AuthVerify verifies a wallet signature and starts a session.
func (h *Handler) AuthVerify(w http.ResponseWriter, r *http.Request) {
	var req auth.VerifyRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	session, token, err := h.auth.VerifyWalletSignature(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidNonce), errors.Is(err, auth.ErrInvalidSignature), errors.Is(err, auth.ErrUnsupportedChain):
			// Generic message: the precise failure reason helps attackers more than users.
			middleware.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Wallet signature could not be verified. Request a new nonce and sign again.")
		default:
			h.writeError(w, r, err)
		}
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "session": session})
}

// AuthSession reports the caller's current session, so a reloaded UI can
// tell whether it is still signed in. 401 when there is none.
func (h *Handler) AuthSession(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		middleware.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "No active session.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session, "authRequired": h.auth.AuthRequired()})
}

// AuthLogout revokes the current session.
func (h *Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	if err := h.auth.RevokeSession(r.Context(), middleware.ExtractToken(r)); err != nil {
		h.writeError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: middleware.SessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// ---- catalogue and intelligence ---------------------------------------------

func (h *Handler) buildStatic() {
	reg := h.swaps.Registry()
	live := h.swaps.QuoteMode() == config.QuoteModeLive && h.swaps.LiveExecutionEnabled()
	all := reg.All()
	chains := make([]ChainDTO, 0, len(all))
	for _, chain := range all {
		chains = append(chains, chainDTO(chain, live))
	}
	h.chainsJSON, _ = json.Marshal(map[string]any{"chains": chains})

	standard, err := h.research.GetStandardScenarios()
	if err != nil {
		h.logger.Error("research scenarios failed", "error", err)
	}
	research := ResearchDTO{Unit: standard.Unit, Note: standard.Note, Formula: standard.Formula}
	for _, scenario := range standard.Scenarios {
		research.Scenarios = append(research.Scenarios, ResearchScenarioDTO{
			Key:                    scenario.Key,
			Label:                  scenario.Label,
			DailyVolumeUSD:         scenario.Inputs.DailyVolumeUSD,
			FeeRatePct:             scenario.Inputs.FeeRatePct,
			CapturePct:             scenario.Inputs.CapturePct,
			ActivityGrowthPct:      scenario.Inputs.ActivityGrowthPct,
			ProjectedAnnualFeesUSD: scenario.ProjectedAnnualFees,
		})
	}
	h.researchJSON, _ = json.Marshal(research)
}

// Chains lists the chain registry. The body never changes at runtime, so it is
// encoded once.
func (h *Handler) Chains(w http.ResponseWriter, _ *http.Request) {
	h.staticOnce.Do(h.buildStatic)
	writeRawJSON(w, http.StatusOK, h.chainsJSON)
}

// ResearchScenarios returns the standard scenarios computed by the engine.
func (h *Handler) ResearchScenarios(w http.ResponseWriter, _ *http.Request) {
	h.staticOnce.Do(h.buildStatic)
	writeRawJSON(w, http.StatusOK, h.researchJSON)
}

// Assets lists the enabled catalogue. Encoded bodies are cached per catalogue
// snapshot and chain filter; a live catalogue has hundreds of tokens and
// re-encoding it per request was the dominant cost of this endpoint.
func (h *Handler) Assets(w http.ResponseWriter, r *http.Request) {
	chain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("chain")))
	assets, info, err := h.swaps.ListAssets(r.Context(), chain)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.assetsMu.RLock()
	cached, ok := h.assetsCache[chain]
	fresh := h.assetsLoadedAt.Equal(info.LoadedAt)
	h.assetsMu.RUnlock()
	if ok && fresh {
		writeRawJSON(w, http.StatusOK, cached)
		return
	}

	out := make([]AssetDTO, 0, len(assets))
	for _, asset := range assets {
		out = append(out, h.assetDTO(asset))
	}
	mode, notice := "live", "Provider token catalogue."
	if h.swaps.QuoteMode() == config.QuoteModeDemo {
		mode, notice = "fixture", FixtureNotice
	}
	body, err := json.Marshal(map[string]any{
		"mode":        mode,
		"notice":      notice,
		"servedAt":    info.LoadedAt.UTC().Format(time.RFC3339),
		"catalogueAt": info.LoadedAt.UTC().Format(time.RFC3339),
		"assets":      out,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.assetsMu.Lock()
	if !h.assetsLoadedAt.Equal(info.LoadedAt) {
		h.assetsLoadedAt = info.LoadedAt
		h.assetsCache = make(map[string][]byte)
	}
	if len(h.assetsCache) < 256 { // bound distinct ?chain= values
		h.assetsCache[chain] = body
	}
	h.assetsMu.Unlock()
	writeRawJSON(w, http.StatusOK, body)
}

var comparisonChains = []string{"near", "base", "arbitrum"}

// MarketsOverview serves live DeFiLlama data when enabled, otherwise fixtures.
func (h *Handler) MarketsOverview(w http.ResponseWriter, r *http.Request) {
	if h.market == nil {
		writeJSON(w, http.StatusOK, DefaultMarketOverview)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "true"
	snapshots := make([]*DataSnapshotDTO, len(comparisonChains))
	var comparisons [3]ChainComparisonDTO
	var group errgroup.Group
	for i, chain := range comparisonChains {
		i, chain := i, chain
		group.Go(func() error {
			snapshot, err := h.market.Snapshot(r.Context(), chain, refresh && i == 0)
			if err != nil {
				return err
			}
			comparisons[i] = ChainComparisonDTO{
				Chain:          snapshot.Chain,
				TVLUSD:         metricValue(snapshot, "tvl"),
				DailyVolumeUSD: metricValue(snapshot, "dex_volume_24h"),
				Source:         snapshot.Provider,
			}
			if i == 0 {
				dto := liveSnapshotDTO(snapshot)
				snapshots[0] = &dto
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil || snapshots[0] == nil {
		h.logger.Warn("live market data unavailable; serving fixtures", "error", err)
		fallback := DefaultMarketOverview
		fallback.Notice = "Live market data is temporarily unavailable. " + FixtureNotice
		writeJSON(w, http.StatusOK, fallback)
		return
	}
	overview := *snapshots[0]
	overview.ChainComparisons = comparisons[:]
	writeJSON(w, http.StatusOK, overview)
}

// IntelligenceNear serves the NEAR snapshot.
func (h *Handler) IntelligenceNear(w http.ResponseWriter, r *http.Request) {
	if h.market != nil {
		snapshot, err := h.market.Snapshot(r.Context(), "near", false)
		if err == nil {
			writeJSON(w, http.StatusOK, liveSnapshotDTO(snapshot))
			return
		}
		h.logger.Warn("live NEAR snapshot unavailable; serving fixtures", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":       "fixture",
		"notice":     FixtureNotice,
		"capturedAt": FixtureDate,
		"metrics":    DefaultNearMetrics,
		"series":     DefaultSeries,
	})
}

// ---- swaps -----------------------------------------------------------------

type quoteRequest struct {
	OriginAssetID      string `json:"originAssetId"`
	DestinationAssetID string `json:"destinationAssetId"`
	AmountIn           string `json:"amountIn"`
	Recipient          string `json:"recipient"`
	RefundTo           string `json:"refundTo"`
	SlippageBps        int    `json:"slippageBps"`
}

// Quotes creates a dry quote.
func (h *Handler) Quotes(w http.ResponseWriter, r *http.Request) {
	var req quoteRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	if req.SlippageBps > 0 {
		if exceeds, _ := h.risk.CheckSlippage(req.SlippageBps); exceeds {
			h.writeError(w, r, domain.Newf(domain.CodeInvalidSlippage,
				"Requested slippage of %d bps exceeds the risk limit.", req.SlippageBps))
			return
		}
	}
	quote, err := h.swaps.DryQuote(r.Context(), service.DryQuoteParams{
		OriginAssetID:      req.OriginAssetID,
		DestinationAssetID: req.DestinationAssetID,
		AmountIn:           req.AmountIn,
		Recipient:          req.Recipient,
		RefundTo:           req.RefundTo,
		SlippageBPS:        req.SlippageBps,
		SessionID:          middleware.Principal(r),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"quote": h.quoteDTO(r.Context(), quote)})
}

type swapCreateRequest struct {
	QuoteID   string `json:"quoteId"`
	Recipient string `json:"recipient"`
	RefundTo  string `json:"refundTo"`
}

// Swaps creates (or idempotently replays) a swap from a quote.
func (h *Handler) Swaps(w http.ResponseWriter, r *http.Request) {
	var req swapCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	result, err := h.swaps.CreateSwap(r.Context(), service.CreateSwapParams{
		QuoteID:        req.QuoteID,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		SessionID:      middleware.Principal(r),
		Recipient:      req.Recipient,
		RefundTo:       req.RefundTo,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views, err := h.swaps.BuildViews(r.Context(), []domain.Swap{result.Swap})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"swap": h.swapDTO(r.Context(), views[0]), "created": !result.Replayed})
}

// SwapByID returns one swap.
func (h *Handler) SwapByID(w http.ResponseWriter, r *http.Request) {
	view, err := h.swaps.GetSwapView(r.Context(), r.PathValue("id"), middleware.Principal(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"swap": h.swapDTO(r.Context(), view)})
}

// SwapEvents streams real-time swap updates using Server-Sent Events (SSE).
func (h *Handler) SwapEvents(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		h.writeError(w, r, domain.New(domain.CodeValidationFailed, "Swap ID is required."))
		return
	}
	principal := middleware.Principal(r)
	view, err := h.swaps.GetSwapView(r.Context(), id, principal)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()

	sendEvent := func(v service.SwapView) error {
		dto := h.swapDTO(r.Context(), v)
		data, err := json.Marshal(dto)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: swap\ndata: %s\n\n", data); err != nil {
			return err
		}
		return rc.Flush()
	}

	if err := sendEvent(view); err != nil {
		return
	}
	if view.Swap.State.Terminal() {
		return
	}

	sub, unsub := h.swaps.EventBus().Subscribe(id)
	defer unsub()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	pollTicker := time.NewTicker(2 * time.Second)
	defer pollTicker.Stop()

	lastState := view.Swap.State

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case updatedSwap := <-sub:
			updatedView, err := h.swaps.GetSwapView(r.Context(), id, principal)
			if err != nil {
				views, bErr := h.swaps.BuildViews(r.Context(), []domain.Swap{updatedSwap})
				if bErr == nil && len(views) > 0 {
					updatedView = views[0]
				}
			}
			lastState = updatedView.Swap.State
			if err := sendEvent(updatedView); err != nil {
				return
			}
			if lastState.Terminal() {
				return
			}
		case <-pollTicker.C:
			currentView, err := h.swaps.GetSwapView(r.Context(), id, principal)
			if err == nil && currentView.Swap.State != lastState {
				lastState = currentView.Swap.State
				if err := sendEvent(currentView); err != nil {
					return
				}
				if lastState.Terminal() {
					return
				}
			}
		}
	}
}

type depositRequest struct {
	TxHash  string `json:"txHash"`
	ChainID string `json:"chainId"`
}

// Deposit records the user's source transaction.
func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	var req depositRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	result, err := h.swaps.RecordDeposit(r.Context(), service.RecordDepositParams{
		SwapID:    r.PathValue("id"),
		TxHash:    req.TxHash,
		ChainID:   req.ChainID,
		SessionID: middleware.Principal(r),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	extra := map[string]any{"duplicate": result.Duplicate}
	if result.Verification != nil {
		extra["verification"] = map[string]any{
			"status":        result.Verification.Status,
			"confirmations": result.Verification.Confirmations,
			"checkedAt":     result.Verification.CheckedAt,
		}
	}
	h.respondSwap(w, r, result.Swap, http.StatusAccepted, extra)
}

// Cancel discards an unfunded swap.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	swap, err := h.swaps.CancelSwap(r.Context(), r.PathValue("id"), middleware.Principal(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.respondSwap(w, r, swap, http.StatusOK, nil)
}

// Simulate advances a simulation record by one step.
func (h *Handler) Simulate(w http.ResponseWriter, r *http.Request) {
	swap, err := h.swaps.AdvanceSimulation(r.Context(), r.PathValue("id"), middleware.Principal(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.respondSwap(w, r, swap, http.StatusOK, nil)
}

// Claim claims an anonymous swap for the authenticated session.
func (h *Handler) Claim(w http.ResponseWriter, r *http.Request) {
	sess := middleware.GetSession(r)
	if sess == nil || sess.Principal() == "" {
		h.writeError(w, r, domain.New(domain.CodeUnauthorized, "Sign in with your wallet to claim swaps."))
		return
	}
	swap, err := h.swaps.ClaimSwap(r.Context(), r.PathValue("id"), sess.Principal(), sess.WalletAddress)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.respondSwap(w, r, swap, http.StatusOK, nil)
}

func (h *Handler) respondSwap(w http.ResponseWriter, r *http.Request, swap domain.Swap, status int, extra map[string]any) {
	views, err := h.swaps.BuildViews(r.Context(), []domain.Swap{swap})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	body := map[string]any{"swap": h.swapDTO(r.Context(), views[0])}
	for key, value := range extra {
		body[key] = value
	}
	writeJSON(w, status, body)
}

// Activity lists the caller's swaps (anonymous swaps for anonymous callers).
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			h.writeError(w, r, domain.New(domain.CodeValidationFailed, "limit must be between 1 and 100."))
			return
		}
		limit = parsed
	}
	views, next, err := h.swaps.ListActivity(r.Context(), middleware.Principal(r), limit, strings.TrimSpace(r.URL.Query().Get("cursor")))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	swaps := make([]SwapRecordDTO, 0, len(views))
	for _, view := range views {
		swaps = append(swaps, h.swapDTO(r.Context(), view))
	}
	body := map[string]any{"swaps": swaps}
	if next != "" {
		body["nextCursor"] = next
	}
	writeJSON(w, http.StatusOK, body)
}

// DepositTx builds an unsigned, non-custodial transaction payload for the swap deposit.
func (h *Handler) DepositTx(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	principal := middleware.Principal(r)
	view, err := h.swaps.GetSwapView(r.Context(), id, principal)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if view.Swap.State != domain.StateAwaitingUserDeposit {
		h.writeError(w, r, domain.Newf(domain.CodeInvalidSwapState, "Swap %s is in state %s, not awaiting deposit.", id, view.Swap.State))
		return
	}

	from := strings.TrimSpace(r.URL.Query().Get("from"))
	if from == "" {
		if sess := middleware.GetSession(r); sess != nil {
			from = sess.WalletAddress
		}
	}

	origin, _ := h.swaps.LookupAsset(r.Context(), view.Quote.OriginAssetID)
	chainKey := origin.Chain
	if chainKey == "" {
		chainKey = view.Swap.OriginChain
	}
	chain, ok := h.swaps.Registry().Get(chainKey)
	if !ok {
		h.writeError(w, r, domain.Newf(domain.CodeValidationFailed, "Unknown origin chain %s", chainKey))
		return
	}

	depositAddr := view.Swap.DepositAddress
	if depositAddr == "" {
		depositAddr = view.Quote.DepositAddress
	}
	if depositAddr == "" || depositAddr == "simulation-only" {
		switch chain.Family {
		case registry.FamilyEVM:
			depositAddr = "0x000000000000000000000000000000000000dead"
		case registry.FamilyNEAR:
			depositAddr = "simulation.near"
		case registry.FamilySolana:
			depositAddr = "11111111111111111111111111111111"
		case registry.FamilyStellar:
			depositAddr = "GA7QYNF7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74P7UJVTEN"
		case registry.FamilyMove:
			depositAddr = "0x1"
		}
	}
	if depositAddr == "" {
		h.writeError(w, r, domain.New(domain.CodeInvalidSwapState, "Deposit address is not yet assigned."))
		return
	}

	memo := view.Swap.DepositMemo
	if memo == "" {
		memo = view.Quote.DepositMemo
	}

	amountAtomic := view.Quote.AmountInAtomic
	if amountAtomic == nil {
		amountAtomic = big.NewInt(0)
	}

	var payload any
	var buildErr error

	switch chain.Family {
	case registry.FamilyEVM:
		payload, buildErr = wallets.BuildEVMTransfer(wallets.EVMTransferRequest{
			ChainID: chain.ChainID,
			To:      depositAddr,
			Token:   origin.ContractAddress,
			Amount:  amountAtomic.String(),
			Memo:    memo,
		})
	case registry.FamilyNEAR:
		payload, buildErr = wallets.BuildNEARTransfer(wallets.NEARTransferRequest{
			DepositAddress: depositAddr,
			Amount:         amountAtomic.String(),
			TokenContract:  origin.ContractAddress,
			Memo:           memo,
		})
	case registry.FamilySolana:
		if from == "" {
			h.writeError(w, r, domain.New(domain.CodeValidationFailed, "Query parameter 'from' (Solana wallet address) is required."))
			return
		}
		blockhash := "11111111111111111111111111111111"
		if len(chain.RPCURLs) > 0 {
			solClient := &jsonrpc.Client{URLs: chain.RPCURLs, HTTPClient: &http.Client{Timeout: 3 * time.Second}}
			var bhRes struct {
				Value struct {
					Blockhash string `json:"blockhash"`
				} `json:"value"`
			}
			if err := solClient.Call(r.Context(), "getLatestBlockhash", []any{map[string]string{"commitment": "confirmed"}}, &bhRes); err == nil && bhRes.Value.Blockhash != "" {
				blockhash = bhRes.Value.Blockhash
			}
		}
		payload, buildErr = wallets.BuildSolanaTransfer(wallets.SolanaTransferRequest{
			From:            from,
			To:              depositAddr,
			Mint:            origin.ContractAddress,
			Memo:            memo,
			RecentBlockhash: blockhash,
			Amount:          amountAtomic.Uint64(),
		})
	case registry.FamilyStellar:
		if from == "" {
			h.writeError(w, r, domain.New(domain.CodeValidationFailed, "Query parameter 'from' (Stellar account) is required."))
			return
		}
		var stellarMemo stellar.Memo
		if memo != "" {
			if sm, err := stellar.TextMemo(memo); err == nil {
				stellarMemo = sm
			}
		}
		var asset wallets.StellarAsset
		if origin.ContractAddress != "" {
			parts := strings.Split(origin.ContractAddress, ":")
			if len(parts) == 2 {
				asset = wallets.StellarAsset{Code: parts[0], Issuer: parts[1]}
			}
		}
		payload, buildErr = wallets.BuildStellarTransfer(wallets.StellarTransferRequest{
			SourceAccount: from,
			Sequence:      0,
			BaseFee:       100,
			Destination:   depositAddr,
			Asset:         asset,
			Amount:        amountAtomic.Int64(),
			Memo:          stellarMemo,
		})
	case registry.FamilyMove:
		payload, buildErr = wallets.BuildAptosTransfer(wallets.AptosTransferRequest{
			Recipient: depositAddr,
			Amount:    amountAtomic.String(),
			CoinType:  origin.ContractAddress,
		})
	default:
		buildErr = fmt.Errorf("unsupported chain family %s", chain.Family)
	}

	if buildErr != nil {
		h.writeError(w, r, domain.Newf(domain.CodeValidationFailed, "Could not build deposit transaction: %v", buildErr))
		return
	}

	dto := DepositTxDTO{
		Family:    string(chain.Family),
		ChainKey:  chain.Key,
		Payload:   payload,
		Memo:      strPtr(memo),
		ExpiresAt: timePtr(view.Swap.ExpiresAt),
	}
	writeJSON(w, http.StatusOK, dto)
}

// ---- admin operations (protected by ADMIN_TOKEN) ---------------------------

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.cfg == nil || h.cfg.Security.AdminToken == "" {
		middleware.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "API route was not found.")
		return false
	}
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if subtle.ConstantTimeCompare([]byte(token), []byte(h.cfg.Security.AdminToken)) != 1 {
		middleware.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid admin token.")
		return false
	}
	return true
}

// AdminSwaps lists swaps with stuck_at set.
func (h *Handler) AdminSwaps(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	h.logger.Info("admin listed stuck swaps")
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	swaps, err := h.swaps.ListStuckSwaps(r.Context(), limit)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	out := make([]AdminSwapDTO, 0, len(swaps))
	for _, sw := range swaps {
		var providerStatus *string
		if sw.ProviderStatus != "" {
			ps := string(sw.ProviderStatus)
			providerStatus = &ps
		}
		var lastErr *string
		if sw.LastErrorCode != "" {
			le := string(sw.LastErrorCode)
			lastErr = &le
		}
		out = append(out, AdminSwapDTO{
			ID:             sw.ID,
			QuoteID:        sw.QuoteID,
			State:          string(sw.State),
			ProviderStatus: providerStatus,
			StuckAt:        sw.StuckAt.UTC().Format(time.RFC3339),
			LastErrorCode:  lastErr,
			OriginChain:    sw.OriginChain,
			DestChain:      sw.DestinationChain,
			CreatedAt:      sw.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"swaps": out})
}

// AdminRepoll clears stuck_at and sets next_poll_at = now().
func (h *Handler) AdminRepoll(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	swap, err := h.swaps.RepollSwap(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.logger.Info("admin repolled swap", "swapId", id)
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "repoll_scheduled",
		"swapId": swap.ID,
	})
}

// AdminProviderHealth returns provider error rates and worker health.
func (h *Handler) AdminProviderHealth(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	h.logger.Info("admin checked provider health")
	status := h.risk.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                 status.Status,
		"circuitBreaker":         status.CircuitBreaker,
		"providerErrorRate":      status.ProviderErrorRate,
		"lastSuccessfulPoll":     status.LastSuccessfulPoll,
		"catalogueStale":         status.CatalogueStale,
		"catalogueLastRefreshed": status.CatalogueLastRefreshed,
		"activeAlerts":           status.ActiveAlerts,
	})
}
