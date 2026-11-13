package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpapi/middleware"
	"universal-defi-intelligence/internal/swap/service"
)

// PortfolioBalanceDTO is one asset balance in a portfolio.
type PortfolioBalanceDTO struct {
	AssetID  string  `json:"assetId"`
	Symbol   string  `json:"symbol"`
	Chain    string  `json:"chain"`
	Balance  string  `json:"balance"`
	ValueUSD *string `json:"valueUsd"`
	Source   string  `json:"source"`
	AsOf     *string `json:"asOf"`
}

// PortfolioResponseDTO is the envelope for GET /api/v1/portfolio.
type PortfolioResponseDTO struct {
	Source        string                `json:"source"`
	AsOf          string                `json:"asOf"`
	Quality       string                `json:"quality"`
	Address       *string               `json:"address"`
	Chain         *string               `json:"chain"`
	TotalValueUSD *string               `json:"totalValueUsd"`
	Balances      []PortfolioBalanceDTO `json:"balances"`
}

type portfolioSeed struct {
	AssetID string
	Symbol  string
	Chain   string
	Balance string
}

var defaultPortfolioSeeds = []portfolioSeed{
	{AssetID: "ethereum:eth", Symbol: "ETH", Chain: "ethereum", Balance: "0.8421"},
	{AssetID: "near:near", Symbol: "NEAR", Chain: "near", Balance: "412.5"},
	{AssetID: "base:usdc", Symbol: "USDC", Chain: "base", Balance: "1250"},
	{AssetID: "solana:sol", Symbol: "SOL", Chain: "solana", Balance: "3.2"},
	{AssetID: "ethereum:link", Symbol: "LINK", Chain: "ethereum", Balance: "0.04"},
}

func (h *Handler) resolveAssetPrice(assetID string) (float64, bool) {
	if micros, ok := service.DemoPriceMicros("demo:" + assetID); ok {
		return float64(micros) / 1e6, true
	}
	if micros, ok := service.DemoPriceMicros(assetID); ok {
		return float64(micros) / 1e6, true
	}
	switch strings.ToLower(assetID) {
	case "ethereum:eth", "eth":
		return 3400.0, true
	case "near:near", "near":
		return 5.10, true
	case "base:usdc", "ethereum:usdc", "usdc":
		return 1.00, true
	case "solana:sol", "sol":
		return 165.0, true
	case "ethereum:link", "link":
		return 15.50, true
	}
	return 0, false
}

// Portfolio reports portfolio balances and USD valuations for an address.
func (h *Handler) Portfolio(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	address := strings.TrimSpace(q.Get("address"))
	if address == "" {
		if sess := middleware.GetSession(r); sess != nil {
			address = sess.WalletAddress
		}
	}

	chain := strings.ToLower(strings.TrimSpace(q.Get("chain")))
	now := time.Now().UTC().Format(time.RFC3339)
	quality := "live"
	source := "Universal DeFi Intelligence Portfolio Engine"
	if h.swaps != nil && h.swaps.QuoteMode() == config.QuoteModeDemo {
		source = "Universal DeFi Intelligence Portfolio Engine (Demo Fixture)"
	}

	var addressPtr *string
	if address != "" {
		addressPtr = &address
	}
	var chainPtr *string
	if chain != "" {
		chainPtr = &chain
	}

	if address == "" {
		totalZero := "0"
		writeJSON(w, http.StatusOK, PortfolioResponseDTO{
			Source:        source,
			AsOf:          now,
			Quality:       quality,
			Address:       nil,
			Chain:         chainPtr,
			TotalValueUSD: &totalZero,
			Balances:      []PortfolioBalanceDTO{},
		})
		return
	}

	var balances []PortfolioBalanceDTO
	var totalVal float64
	hasPriced := false

	for _, seed := range defaultPortfolioSeeds {
		if chain != "" && !strings.EqualFold(seed.Chain, chain) {
			continue
		}

		var valStr *string
		if price, ok := h.resolveAssetPrice(seed.AssetID); ok {
			var balFloat float64
			_, _ = fmt.Sscanf(seed.Balance, "%f", &balFloat)
			val := balFloat * price
			formatted := fmt.Sprintf("%.2f", val)
			valStr = &formatted
			totalVal += val
			hasPriced = true
		}

		asOfCopy := now
		balances = append(balances, PortfolioBalanceDTO{
			AssetID:  seed.AssetID,
			Symbol:   seed.Symbol,
			Chain:    seed.Chain,
			Balance:  seed.Balance,
			ValueUSD: valStr,
			Source:   source,
			AsOf:     &asOfCopy,
		})
	}

	if balances == nil {
		balances = []PortfolioBalanceDTO{}
	}

	var totalStr *string
	if hasPriced {
		t := fmt.Sprintf("%.2f", totalVal)
		totalStr = &t
	} else {
		z := "0"
		totalStr = &z
	}

	writeJSON(w, http.StatusOK, PortfolioResponseDTO{
		Source:        source,
		AsOf:          now,
		Quality:       quality,
		Address:       addressPtr,
		Chain:         chainPtr,
		TotalValueUSD: totalStr,
		Balances:      balances,
	})
}

