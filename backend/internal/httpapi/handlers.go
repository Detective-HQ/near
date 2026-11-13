package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpapi/middleware"
	"universal-defi-intelligence/internal/risk"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/service"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error     APIError `json:"error"`
	RequestID string   `json:"requestId"`
}

type QuoteRequest struct {
	OriginAssetID      string `json:"originAssetId"`
	DestinationAssetID string `json:"destinationAssetId"`
	AmountIn           string `json:"amountIn"`
	Recipient          string `json:"recipient,omitempty"`
	RefundTo           string `json:"refundTo,omitempty"`
	SlippageBps        int    `json:"slippageBps,omitempty"`
}

type QuoteResponse struct {
	ID                      string        `json:"id"`
	Mode                    string        `json:"mode"`
	Provider                string        `json:"provider"`
	OriginAssetID           string        `json:"originAssetId"`
	DestinationAssetID      string        `json:"destinationAssetId"`
	OriginAsset             AssetResponse `json:"originAsset"`
	DestinationAsset        AssetResponse `json:"destinationAsset"`
	AmountInAtomic          string        `json:"amountInAtomic"`
	AmountIn                string        `json:"amountIn"`
	AmountOutEstimateAtomic string        `json:"amountOutEstimateAtomic"`
	AmountOutEstimate       string        `json:"amountOutEstimate"`
	MinimumAmountOutAtomic  string        `json:"minimumAmountOutAtomic"`
	MinimumAmountOut        string        `json:"minimumAmountOut"`
	FeeBps                  int           `json:"feeBps"`
	SlippageBps             int           `json:"slippageBps"`
	ExpiresAt               string        `json:"expiresAt"`
	Recipient               string        `json:"recipient,omitempty"`
	RefundTo                string        `json:"refundTo,omitempty"`
	Route                   []string      `json:"route"`
	Warning                 string        `json:"warning"`
}

type SwapCreateRequest struct {
	QuoteID   string `json:"quoteId"`
	Recipient string `json:"recipient,omitempty"`
	RefundTo  string `json:"refundTo,omitempty"`
}

type DepositRequest struct {
	TxHash  string `json:"txHash"`
	ChainID string `json:"chainId"`
}

type SwapEventRecord struct {
	State  string `json:"state"`
	Detail string `json:"detail"`
	At     string `json:"at"`
}

type SwapRecordResponse struct {
	ID             string             `json:"id"`
	QuoteID        string             `json:"quoteId"`
	State          string             `json:"state"`
	ProviderStatus *string            `json:"providerStatus"`
	Mode           string             `json:"mode"`
	RealSettlement bool               `json:"realSettlement"`
	Transactions   map[string]*string `json:"transactions"`
	CreatedAt      string             `json:"createdAt"`
	UpdatedAt      string             `json:"updatedAt"`
	Events         []SwapEventRecord  `json:"events"`
	Quote          QuoteResponse      `json:"quote"`
}

type StateStore struct {
	mu          sync.RWMutex
	quotes      map[string]QuoteResponse
	swaps       map[string]SwapRecordResponse
	idempotency map[string]string // idempotency-key -> swapId
}

func NewStateStore() *StateStore {
	return &StateStore{
		quotes:      make(map[string]QuoteResponse),
		swaps:       make(map[string]SwapRecordResponse),
		idempotency: make(map[string]string),
	}
}

func randomID(prefix string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	reqID := middleware.GetRequestID(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-Id", reqID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reqID := middleware.GetRequestID(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-Id", reqID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
		},
		RequestID: reqID,
	})
}

func (h *Handler) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var dErr *domain.Error
	if errors.As(err, &dErr) {
		status := dErr.HTTPStatus
		if status == 0 {
			status = dErr.Code.HTTPStatus()
		}
		writeError(w, r, status, string(dErr.Code), dErr.Message)
		return
	}
	writeError(w, r, http.StatusInternalServerError, "INTERNAL", err.Error())
}

type HandlerOptions struct {
	Store       *StateStore
	SwapService *service.Service
	RiskEngine  *risk.Engine
	AuthManager *auth.Manager
	Registry    *registry.Registry
	Config      *config.Config
}

type Handler struct {
	store       *StateStore
	swapService *service.Service
	riskEngine  *risk.Engine
	authManager *auth.Manager
	registry    *registry.Registry
	config      *config.Config
}

func NewHandler(store *StateStore) *Handler {
	return NewHandlerWithOptions(HandlerOptions{Store: store})
}

func NewHandlerWithOptions(opts HandlerOptions) *Handler {
	store := opts.Store
	if store == nil {
		store = NewStateStore()
	}
	rEngine := opts.RiskEngine
	if rEngine == nil {
		rEngine = risk.NewEngine(risk.Config{})
	}
	aMgr := opts.AuthManager
	if aMgr == nil {
		aMgr = auth.NewManager(auth.Config{AuthRequired: false})
	}

	return &Handler{
		store:       store,
		swapService: opts.SwapService,
		riskEngine:  rEngine,
		authManager: aMgr,
		registry:    opts.Registry,
		config:      opts.Config,
	}
}

// Healthz returns a lightweight liveness probe (200 OK).
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Readyz checks dependencies (database, Redis, catalogue freshness) and returns readiness.
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ready := true
	checks := map[string]any{
		"status": "ready",
	}

	if h.swapService != nil {
		checks["swapService"] = "ready"
	} else {
		checks["swapService"] = "demo_mode"
	}

	if h.riskEngine != nil {
		report := h.riskEngine.Status()
		checks["riskStatus"] = report.Status
		if report.Status == "UNHEALTHY" {
			ready = false
			checks["status"] = "not_ready"
		}
	}

	statusCode := http.StatusOK
	if !ready {
		statusCode = http.StatusServiceUnavailable
	}
	writeJSON(w, r, statusCode, checks)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	quoteMode := "demo"
	marketMode := "fixture"
	execEnabled := false

	if h.config != nil {
		quoteMode = string(h.config.Execution.QuoteMode)
		marketMode = string(h.config.MarketData.Mode)
		execEnabled = h.config.Execution.EnableLiveExecution
	}

	resp := map[string]any{
		"status":         "healthy",
		"mode":           quoteMode,
		"quoteMode":      quoteMode,
		"demoMode":       quoteMode == "demo",
		"marketDataMode": marketMode,
		"capabilityFlags": map[string]bool{
			"executionEnabled": execEnabled,
			"demoEnabled":      true,
		},
		"liveQuotesConfigured": quoteMode == "live",
		"executionEnabled":     execEnabled,
		"capturedAt":           time.Now().UTC().Format(time.RFC3339),
		"fixtureDate":          FixtureDate,
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func (h *Handler) RiskStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	writeJSON(w, r, http.StatusOK, h.riskEngine.Status())
}

func (h *Handler) AuthNonce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	nonce, err := h.authManager.GenerateNonce()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL", "Failed to generate nonce")
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"nonce": nonce})
}

func (h *Handler) AuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var req auth.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid authentication payload")
		return
	}
	session, token, err := h.authManager.VerifyWalletSignature(req)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"token":   token,
		"session": session,
	})
}

func (h *Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	token := extractBearerToken(r)
	if token != "" {
		h.authManager.RevokeSession(token)
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *Handler) Chains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	writeJSON(w, r, http.StatusOK, DefaultChains)
}

func (h *Handler) Assets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	chainParam := r.URL.Query().Get("chain")
	if chainParam == "" {
		writeJSON(w, r, http.StatusOK, DefaultAssets)
		return
	}
	var filtered []AssetResponse
	for _, a := range DefaultAssets {
		if strings.EqualFold(a.Chain, chainParam) {
			filtered = append(filtered, a)
		}
	}
	writeJSON(w, r, http.StatusOK, filtered)
}

func (h *Handler) MarketsOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	writeJSON(w, r, http.StatusOK, DefaultMarketOverview)
}

func (h *Handler) IntelligenceNear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	resp := map[string]any{
		"mode":       "fixture",
		"notice":     FixtureNotice,
		"capturedAt": FixtureDate,
		"metrics":    DefaultNearMetrics,
		"series":     DefaultSeries,
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func (h *Handler) ResearchScenarios(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	writeJSON(w, r, http.StatusOK, DefaultResearchScenarios)
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	sess := middleware.GetSession(r)

	h.store.mu.RLock()
	defer h.store.mu.RUnlock()

	var records []SwapRecordResponse
	for _, s := range h.store.swaps {
		if h.authManager.AuthRequired() && !h.authManager.CheckOwnership(sess, s.QuoteID, s.Quote.Recipient) {
			continue
		}
		records = append(records, s)
	}
	if len(records) == 0 {
		records = []SwapRecordResponse{}
	}
	writeJSON(w, r, http.StatusOK, records)
}

func (h *Handler) Quotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req QuoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid quote request JSON")
		return
	}

	if req.OriginAssetID == "" || req.DestinationAssetID == "" || req.AmountIn == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "originAssetId, destinationAssetId, and amountIn are required")
		return
	}

	// Validate slippage through risk engine
	if req.SlippageBps > 0 {
		exceeds, _ := h.riskEngine.CheckSlippage(req.SlippageBps)
		if exceeds {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_FAILED", fmt.Sprintf("Requested slippage %d bps exceeds maximum risk limit", req.SlippageBps))
			return
		}
	}

	// Lookup assets
	originAsset, err := findAsset(req.OriginAssetID)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "ASSET_NOT_FOUND", fmt.Sprintf("Origin asset %s not found", req.OriginAssetID))
		return
	}
	destAsset, err := findAsset(req.DestinationAssetID)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "ASSET_NOT_FOUND", fmt.Sprintf("Destination asset %s not found", req.DestinationAssetID))
		return
	}

	// Parse human decimal amount
	amountInFloat, _, err := big.ParseFloat(req.AmountIn, 10, 128, big.ToNearestEven)
	if err != nil || amountInFloat.Sign() <= 0 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "amountIn must be a positive decimal number")
		return
	}

	// Calculate simulated exchange rate based on fixturePriceMicros
	rate := 1.0
	inMicros, _ := strconv.ParseFloat(originAsset.FixturePriceMicros, 64)
	outMicros, _ := strconv.ParseFloat(destAsset.FixturePriceMicros, 64)
	if inMicros > 0 && outMicros > 0 {
		rate = inMicros / outMicros
	}
	inVal, _ := amountInFloat.Float64()
	feeBps := 30
	slippageBps := req.SlippageBps
	if slippageBps <= 0 {
		slippageBps = 100
	}

	grossOut := inVal * rate
	netOut := grossOut * (1.0 - float64(feeBps)/10000.0)
	minOut := netOut * (1.0 - float64(slippageBps)/10000.0)

	// Format atomic amounts
	amountInAtomic := toAtomicString(inVal, originAsset.Decimals)
	amountOutAtomic := toAtomicString(netOut, destAsset.Decimals)
	minOutAtomic := toAtomicString(minOut, destAsset.Decimals)

	expiresAt := time.Now().UTC().Add(3 * time.Minute).Format(time.RFC3339)
	quoteID := randomID("quote")

	resp := QuoteResponse{
		ID:                      quoteID,
		Mode:                    "simulation",
		Provider:                "simulation-engine",
		OriginAssetID:           originAsset.ID,
		DestinationAssetID:      destAsset.ID,
		OriginAsset:             originAsset,
		DestinationAsset:        destAsset,
		AmountInAtomic:          amountInAtomic,
		AmountIn:                req.AmountIn,
		AmountOutEstimateAtomic: amountOutAtomic,
		AmountOutEstimate:       fmt.Sprintf("%.6f", netOut),
		MinimumAmountOutAtomic:  minOutAtomic,
		MinimumAmountOut:        fmt.Sprintf("%.6f", minOut),
		FeeBps:                  feeBps,
		SlippageBps:             slippageBps,
		ExpiresAt:               expiresAt,
		Recipient:               req.Recipient,
		RefundTo:                req.RefundTo,
		Route:                   []string{originAsset.Symbol, destAsset.Symbol},
		Warning:                 "Demo simulation estimate. No on-chain funds will be transferred.",
	}

	h.store.mu.Lock()
	h.store.quotes[quoteID] = resp
	h.store.mu.Unlock()

	writeJSON(w, r, http.StatusCreated, resp)
}

func (h *Handler) Swaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Idempotency-Key header is required")
		return
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Idempotency-Key must be 8-128 characters")
		return
	}

	var req SwapCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid swap create request body")
		return
	}
	if req.QuoteID == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "quoteId is required")
		return
	}

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	// Check idempotency
	if existingSwapID, ok := h.store.idempotency[idempotencyKey]; ok {
		if existing, exists := h.store.swaps[existingSwapID]; exists {
			writeJSON(w, r, http.StatusOK, existing)
			return
		}
	}

	quote, exists := h.store.quotes[req.QuoteID]
	if !exists {
		writeError(w, r, http.StatusBadRequest, "QUOTE_EXPIRED", "The requested quote does not exist or has expired")
		return
	}

	swapID := randomID("swap")
	now := time.Now().UTC().Format(time.RFC3339)

	initialEvents := []SwapEventRecord{
		{
			State:  "QUOTE_READY",
			Detail: "Simulation swap record initialized.",
			At:     now,
		},
	}

	resp := SwapRecordResponse{
		ID:             swapID,
		QuoteID:        quote.ID,
		State:          "QUOTE_READY",
		ProviderStatus: nil,
		Mode:           "simulation",
		RealSettlement: false,
		Transactions: map[string]*string{
			"source":      nil,
			"destination": nil,
			"refund":      nil,
		},
		CreatedAt: now,
		UpdatedAt: now,
		Events:    initialEvents,
		Quote:     quote,
	}

	h.store.swaps[swapID] = resp
	h.store.idempotency[idempotencyKey] = swapID

	writeJSON(w, r, http.StatusCreated, resp)
}

func (h *Handler) SwapByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/swaps"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Swap ID is required in URL path")
		return
	}

	swapID := parts[0]

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	swap, exists := h.store.swaps[swapID]
	if !exists {
		writeError(w, r, http.StatusNotFound, "SWAP_NOT_FOUND", "Swap was not found.")
		return
	}

	// Ownership check
	sess := middleware.GetSession(r)
	if h.authManager.AuthRequired() && !h.authManager.CheckOwnership(sess, swap.QuoteID, swap.Quote.Recipient) {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "You do not have access to this swap.")
		return
	}

	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			writeJSON(w, r, http.StatusOK, swap)
			return
		}
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	action := parts[1]
	now := time.Now().UTC().Format(time.RFC3339)

	switch action {
	case "deposit":
		if r.Method != http.MethodPost {
			writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
			return
		}
		var dep DepositRequest
		if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid deposit JSON body")
			return
		}
		if swap.Transactions["source"] != nil && *swap.Transactions["source"] == dep.TxHash {
			writeJSON(w, r, http.StatusOK, map[string]any{"duplicate": true, "swap": swap})
			return
		}
		swap.Transactions["source"] = &dep.TxHash
		swap.State = "DEPOSIT_SUBMITTED"
		swap.UpdatedAt = now
		swap.Events = append(swap.Events, SwapEventRecord{
			State:  "DEPOSIT_SUBMITTED",
			Detail: fmt.Sprintf("Recorded source transaction hash %s on chain %s.", dep.TxHash, dep.ChainID),
			At:     now,
		})
		h.store.swaps[swapID] = swap
		writeJSON(w, r, http.StatusAccepted, swap)

	case "cancel":
		if r.Method != http.MethodPost {
			writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
			return
		}
		if swap.Transactions["source"] != nil {
			writeError(w, r, http.StatusConflict, "INVALID_SWAP_STATE", "Cannot cancel swap after deposit is submitted")
			return
		}
		swap.State = "CANCELLED"
		swap.UpdatedAt = now
		swap.Events = append(swap.Events, SwapEventRecord{
			State:  "CANCELLED",
			Detail: "Swap instruction was cancelled by user.",
			At:     now,
		})
		h.store.swaps[swapID] = swap
		writeJSON(w, r, http.StatusOK, swap)

	case "simulate":
		if r.Method != http.MethodPost {
			writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
			return
		}
		var nextState, nextDetail string
		switch swap.State {
		case "QUOTE_READY":
			nextState = "SIMULATED_DEPOSIT"
			nextDetail = "A simulated deposit event was added. No chain transaction exists."
		case "SIMULATED_DEPOSIT":
			nextState = "SIMULATED_PROCESSING"
			nextDetail = "The local mock advanced to processing. No provider was contacted."
		case "SIMULATED_PROCESSING":
			nextState = "SIMULATION_COMPLETE"
			nextDetail = "The demo lifecycle ended. This is not a settled swap."
		default:
			writeError(w, r, http.StatusConflict, "INVALID_SWAP_STATE", "This simulation has no further demo transitions.")
			return
		}
		swap.State = nextState
		swap.UpdatedAt = now
		swap.Events = append(swap.Events, SwapEventRecord{
			State:  nextState,
			Detail: nextDetail,
			At:     now,
		})
		h.store.swaps[swapID] = swap
		writeJSON(w, r, http.StatusOK, swap)

	default:
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Endpoint not found")
	}
}

func extractBearerToken(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func findAsset(id string) (AssetResponse, error) {
	for _, a := range DefaultAssets {
		if strings.EqualFold(a.ID, id) {
			return a, nil
		}
	}
	return AssetResponse{}, errors.New("asset not found")
}

func toAtomicString(val float64, decimals int) string {
	mult := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	fVal := new(big.Float).Mul(big.NewFloat(val), new(big.Float).SetInt(mult))
	intVal, _ := fVal.Int(nil)
	if intVal == nil {
		return "0"
	}
	return intVal.String()
}
