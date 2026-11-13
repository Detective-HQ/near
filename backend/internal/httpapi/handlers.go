package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
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
	reqID := GetRequestID(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-Id", reqID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reqID := GetRequestID(r)
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

type Handler struct {
	store *StateStore
}

func NewHandler(store *StateStore) *Handler {
	return &Handler{store: store}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	resp := map[string]any{
		"status":         "healthy",
		"quoteMode":      "demo",
		"marketDataMode": "fixture",
		"capabilityFlags": map[string]bool{
			"executionEnabled": false,
			"demoEnabled":      true,
		},
		"fixtureDate": FixtureDate,
	}
	writeJSON(w, r, http.StatusOK, resp)
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
	h.store.mu.RLock()
	defer h.store.mu.RUnlock()

	items := make([]SwapRecordResponse, 0, len(h.store.swaps))
	for _, swap := range h.store.swaps {
		items = append(items, swap)
	}
	writeJSON(w, r, http.StatusOK, items)
}

func (h *Handler) Quotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	var req QuoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON.")
		return
	}

	assetMap := AssetMap()
	originAsset, hasOrigin := assetMap[req.OriginAssetID]
	destAsset, hasDest := assetMap[req.DestinationAssetID]
	if !hasOrigin || !hasDest {
		writeError(w, r, http.StatusBadRequest, "UNKNOWN_ASSET", "Select assets from the configured demo catalogue.")
		return
	}
	if originAsset.ID == destAsset.ID {
		writeError(w, r, http.StatusBadRequest, "SAME_ASSET", "Choose two different assets.")
		return
	}

	inputAtomic, err := domain.ParseUnits(req.AmountIn, originAsset.Decimals)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_AMOUNT", "Enter a positive decimal amount.")
		return
	}

	// Calculate using fixture prices
	originPrice, _ := new(big.Int).SetString(originAsset.FixturePriceMicros, 10)
	destPrice, _ := new(big.Int).SetString(destAsset.FixturePriceMicros, 10)
	originScale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(originAsset.Decimals)), nil)
	destScale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(destAsset.Decimals)), nil)

	inputUsdMicros := new(big.Int).Mul(inputAtomic, originPrice)
	inputUsdMicros.Div(inputUsdMicros, originScale)

	grossOutputAtomic := new(big.Int).Mul(inputUsdMicros, destScale)
	grossOutputAtomic.Div(grossOutputAtomic, destPrice)

	outputAtomic := new(big.Int).Mul(grossOutputAtomic, big.NewInt(997))
	outputAtomic.Div(outputAtomic, big.NewInt(1000))

	minOutputAtomic := new(big.Int).Mul(outputAtomic, big.NewInt(995))
	minOutputAtomic.Div(minOutputAtomic, big.NewInt(1000))

	if outputAtomic.Sign() <= 0 {
		writeError(w, r, http.StatusBadRequest, "AMOUNT_TOO_SMALL", "Amount is too small for this demo route.")
		return
	}

	maxFraction := destAsset.Decimals
	if maxFraction > 8 {
		maxFraction = 8
	}
	outEst, _ := domain.FormatUnitsMax(outputAtomic, destAsset.Decimals, maxFraction)
	minEst, _ := domain.FormatUnitsMax(minOutputAtomic, destAsset.Decimals, maxFraction)

	now := time.Now().UTC()
	quoteID := randomID("qte")
	quote := QuoteResponse{
		ID:                      quoteID,
		Mode:                    "simulation",
		Provider:                "local_fixture",
		OriginAssetID:           originAsset.ID,
		DestinationAssetID:      destAsset.ID,
		OriginAsset:             originAsset,
		DestinationAsset:        destAsset,
		AmountInAtomic:          inputAtomic.String(),
		AmountIn:                domain.FormatUnits(inputAtomic, originAsset.Decimals),
		AmountOutEstimateAtomic: outputAtomic.String(),
		AmountOutEstimate:       outEst,
		MinimumAmountOutAtomic:  minOutputAtomic.String(),
		MinimumAmountOut:        minEst,
		FeeBps:                  30,
		SlippageBps:             50,
		ExpiresAt:               now.Add(2 * time.Minute).Format(time.RFC3339),
		Route:                   []string{originAsset.Chain, destAsset.Chain},
		Warning:                 "Estimate uses fixture prices and a fixed demo fee. It is not a provider quote or an executable transaction.",
	}

	h.store.mu.Lock()
	h.store.quotes[quote.ID] = quote
	h.store.mu.Unlock()

	writeJSON(w, r, http.StatusCreated, quote)
}

func (h *Handler) Swaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		writeError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required (8-128 chars).")
		return
	}

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	if existingID, exists := h.store.idempotency[idempotencyKey]; exists {
		if swap, ok := h.store.swaps[existingID]; ok {
			writeJSON(w, r, http.StatusOK, swap)
			return
		}
	}

	var req SwapCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON.")
		return
	}

	quote, exists := h.store.quotes[req.QuoteID]
	if !exists {
		writeError(w, r, http.StatusNotFound, "QUOTE_NOT_FOUND", "Quote was not found.")
		return
	}

	expiresAt, _ := time.Parse(time.RFC3339, quote.ExpiresAt)
	if time.Now().UTC().After(expiresAt) {
		writeError(w, r, http.StatusGone, "QUOTE_EXPIRED", "Quote expired. Request a fresh simulation.")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	swapID := randomID("swp")
	swap := SwapRecordResponse{
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
		Events: []SwapEventRecord{
			{
				State:  "QUOTE_READY",
				Detail: "Local simulation record created; no deposit was requested or broadcast.",
				At:     now,
			},
		},
		Quote: quote,
	}

	h.store.swaps[swapID] = swap
	h.store.idempotency[idempotencyKey] = swapID

	writeJSON(w, r, http.StatusCreated, swap)
}

func (h *Handler) SwapByID(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /api/v1/swaps/{id} or /api/v1/swaps/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/swaps/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Swap ID missing")
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
			writeError(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid deposit JSON body")
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
			writeError(w, r, http.StatusConflict, "STATE_CONFLICT", "Cannot cancel funded swap")
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
			writeError(w, r, http.StatusConflict, "TERMINAL_STATE", "This simulation has no further demo transitions.")
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
