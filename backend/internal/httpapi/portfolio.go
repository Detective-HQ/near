package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"universal-defi-intelligence/internal/httpapi/middleware"
	"universal-defi-intelligence/internal/portfolio"
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

// PortfolioResponseDTO is the envelope for GET /api/v1/portfolio. It mirrors
// PortfolioResponse in frontend/src/lib/trading-contracts.ts.
type PortfolioResponseDTO struct {
	Source        string                `json:"source"`
	AsOf          string                `json:"asOf"`
	Quality       string                `json:"quality"`
	Address       *string               `json:"address"`
	Chain         *string               `json:"chain"`
	TotalValueUSD *string               `json:"totalValueUsd"`
	Balances      []PortfolioBalanceDTO `json:"balances"`
	Notice        string                `json:"notice"`
}

const portfolioNotice = "Read-only on-chain balances: native assets on every chain family the address matches, plus USDC on EVM chains. Other tokens need an indexer and are not shown."

// Portfolio reads real on-chain balances for ?address= (or the signed-in
// wallet), optionally limited to ?chain=. USD values come from DeFiLlama only
// when live market data is enabled; otherwise they are null.
func (h *Handler) Portfolio(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	address := strings.TrimSpace(q.Get("address"))
	if address == "" {
		if sess := middleware.GetSession(r); sess != nil {
			address = sess.WalletAddress
		}
	}
	chain := strings.ToLower(strings.TrimSpace(q.Get("chain")))
	var chainPtr *string
	if chain != "" {
		chainPtr = &chain
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if address == "" {
		writeJSON(w, http.StatusOK, PortfolioResponseDTO{
			Source: "On-chain RPC", AsOf: now, Quality: "unavailable", Chain: chainPtr,
			Balances: []PortfolioBalanceDTO{}, Notice: "Connect a wallet or pass ?address= to read balances.",
		})
		return
	}
	if h.portfolio == nil {
		middleware.WriteError(w, r, http.StatusServiceUnavailable, "BALANCE_UNAVAILABLE", "Balance reads are not configured.")
		return
	}
	if len(address) > 128 {
		middleware.WriteError(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "Address is too long.")
		return
	}

	res, err := h.portfolio.Read(r.Context(), address, chain)
	if err != nil {
		switch {
		case errors.Is(err, portfolio.ErrInvalidAddress):
			middleware.WriteError(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "The address is not valid for a supported chain.")
		case errors.Is(err, portfolio.ErrUnavailable):
			middleware.WriteError(w, r, http.StatusServiceUnavailable, "BALANCE_UNAVAILABLE", "Chain RPC endpoints did not respond; try again shortly.")
		default:
			h.writeError(w, r, err)
		}
		return
	}

	balances := make([]PortfolioBalanceDTO, 0, len(res.Holdings))
	var total float64
	priced := false
	for _, hld := range res.Holdings {
		asOf := hld.AsOf.UTC().Format(time.RFC3339)
		dto := PortfolioBalanceDTO{
			AssetID: hld.AssetID, Symbol: hld.Symbol, Chain: hld.Chain,
			Balance: hld.Balance(), Source: hld.Source, AsOf: &asOf,
		}
		if hld.ValueUSD != nil {
			v := fmt.Sprintf("%.2f", *hld.ValueUSD)
			dto.ValueUSD = &v
			total += *hld.ValueUSD
			priced = true
		}
		balances = append(balances, dto)
	}
	source := "On-chain RPC"
	if res.Priced {
		source += " + DeFiLlama prices"
	}
	notice := portfolioNotice
	if len(res.Failed) > 0 {
		notice += " Unavailable right now: " + strings.Join(res.Failed, ", ") + "."
	}
	out := PortfolioResponseDTO{
		Source: source, AsOf: res.AsOf.UTC().Format(time.RFC3339), Quality: res.Quality,
		Address: &res.Address, Chain: chainPtr, Balances: balances, Notice: notice,
	}
	if priced {
		t := fmt.Sprintf("%.2f", total)
		out.TotalValueUSD = &t
	}
	writeJSON(w, http.StatusOK, out)
}
