package service

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

// DryQuoteParams holds the input parameters for a dry quote request.
type DryQuoteParams struct {
	OriginAssetID      string `json:"originAssetId"`
	DestinationAssetID string `json:"destinationAssetId"`
	AmountIn           string `json:"amountIn"`
	Recipient          string `json:"recipient"`
	RefundTo           string `json:"refundTo"`
	SlippageBPS        int    `json:"slippageBps"`
	SessionID          string `json:"sessionId,omitempty"`
}

// DryQuote executes the dry quoting pipeline. It performs exact decimal math,
// multi-chain address checks, requests a quote from the provider (or calculates
// demo simulation values), and stores the result in PostgreSQL. It never creates
// a swap record.
func (s *Service) DryQuote(ctx context.Context, params DryQuoteParams) (domain.Quote, error) {
	originID := strings.TrimSpace(params.OriginAssetID)
	destID := strings.TrimSpace(params.DestinationAssetID)

	if originID == "" || destID == "" {
		return domain.Quote{}, domain.New(domain.CodeValidationFailed, "Origin and destination assets are required.")
	}
	if strings.EqualFold(originID, destID) {
		return domain.Quote{}, domain.New(domain.CodeInvalidAssetPair, "Choose two different assets from the catalogue.")
	}

	slippage := params.SlippageBPS
	if slippage < 0 || slippage > 5000 {
		return domain.Quote{}, domain.New(domain.CodeInvalidSlippage, "Slippage must be between 0 and 5000 basis points.")
	}
	if slippage == 0 {
		slippage = 100 // Default 1%
	}

	isDemo := s.quoteMode == config.QuoteModeDemo ||
		strings.HasPrefix(originID, "demo:") ||
		strings.HasPrefix(destID, "demo:")

	originAsset, err := s.resolveAsset(ctx, originID, isDemo)
	if err != nil {
		return domain.Quote{}, err
	}
	destAsset, err := s.resolveAsset(ctx, destID, isDemo)
	if err != nil {
		return domain.Quote{}, err
	}

	amountAtomic, err := domain.ParseUnits(params.AmountIn, originAsset.Decimals)
	if err != nil {
		return domain.Quote{}, domain.Wrap(domain.CodeInvalidAmount, "Enter a valid positive decimal amount.", err)
	}
	if amountAtomic.Sign() <= 0 {
		return domain.Quote{}, domain.New(domain.CodeInvalidAmount, "Amount must be strictly greater than zero.")
	}

	recipient := strings.TrimSpace(params.Recipient)
	refundTo := strings.TrimSpace(params.RefundTo)

	if recipient == "" {
		return domain.Quote{}, domain.New(domain.CodeRecipientRequired, "Recipient address is required.")
	}
	if refundTo == "" {
		return domain.Quote{}, domain.New(domain.CodeRefundRequired, "Refund address is required.")
	}

	normalizedRecipient, err := ValidateChainAddress(s.registry, destAsset.Chain, recipient)
	if err != nil {
		return domain.Quote{}, err
	}
	normalizedRefund, err := ValidateChainAddress(s.registry, originAsset.Chain, refundTo)
	if err != nil {
		return domain.Quote{}, err
	}

	now := s.now()

	if isDemo {
		return s.createDemoQuote(ctx, originAsset, destAsset, amountAtomic, params.AmountIn, normalizedRecipient, normalizedRefund, slippage, params.SessionID, now)
	}

	providerReq := providers.DryQuoteRequest{
		OriginAsset:      originAsset,
		DestinationAsset: destAsset,
		AmountAtomic:     amountAtomic,
		Recipient:        normalizedRecipient,
		RefundTo:         normalizedRefund,
		SlippageBPS:      slippage,
	}

	quoteRes, err := s.provider.GetDryQuote(ctx, providerReq)
	if err != nil {
		return domain.Quote{}, err
	}

	quote := quoteRes.Quote
	if quote.ID == "" {
		quote.ID = "qte_" + uuid.NewString()
	}
	quote.SessionID = params.SessionID
	quote.Mode = domain.QuoteModeProviderDry
	quote.OriginAssetID = originAsset.ID
	quote.DestinationAssetID = destAsset.ID
	quote.AmountInAtomic = amountAtomic
	quote.AmountInDecimal = params.AmountIn
	quote.FeeBPS = slippage
	quote.SlippageBPS = slippage
	if quote.ExpiresAt.IsZero() {
		quote.ExpiresAt = now.Add(s.quoteDeadline)
	}
	quote.CreatedAt = now
	quote.UpdatedAt = now

	return s.store.SaveQuote(ctx, quote)
}

func (s *Service) resolveAsset(ctx context.Context, id string, isDemo bool) (domain.Asset, error) {
	if isDemo {
		return demoAsset(id), nil
	}

	asset, err := s.store.GetAssetByID(ctx, id)
	if err == nil {
		return asset, nil
	}

	// Fallback to querying provider catalogue directly if the DB cache is cold
	tokens, tokErr := s.provider.GetTokens(ctx)
	if tokErr == nil {
		for _, tok := range tokens {
			if strings.EqualFold(tok.ID, id) || strings.EqualFold(tok.ProviderAssetID, id) {
				return tok, nil
			}
		}
	}

	return domain.Asset{}, domain.Newf(domain.CodeUnknownAsset, "Asset %q was not found in the live catalogue.", id)
}

func demoAsset(id string) domain.Asset {
	switch strings.ToLower(id) {
	case "demo:near:near", "near":
		return domain.Asset{
			ID:              "demo:near:near",
			ProviderAssetID: "demo:near:near",
			Chain:           "near",
			Symbol:          "NEAR",
			Name:            "NEAR",
			Decimals:        24,
			Enabled:         true,
		}
	case "demo:base:usdc", "usdc":
		return domain.Asset{
			ID:              "demo:base:usdc",
			ProviderAssetID: "demo:base:usdc",
			Chain:           "base",
			Symbol:          "USDC",
			Name:            "USD Coin",
			Decimals:        6,
			Enabled:         true,
		}
	default:
		chain := "eth"
		if strings.Contains(id, ":") {
			parts := strings.Split(id, ":")
			if len(parts) >= 2 {
				chain = parts[1]
			}
		}
		return domain.Asset{
			ID:              id,
			ProviderAssetID: id,
			Chain:           chain,
			Symbol:          "DEMO",
			Name:            "Demo Asset",
			Decimals:        18,
			Enabled:         true,
		}
	}
}

func (s *Service) createDemoQuote(
	ctx context.Context,
	origin, dest domain.Asset,
	amountAtomic *big.Int,
	amountInDecimal string,
	recipient, refundTo string,
	slippage int,
	sessionID string,
	now time.Time,
) (domain.Quote, error) {
	// Simple illustrative demo conversion: 1 Origin Unit = 4 Destination Units
	// Normalize to atomic units
	estAtomic := new(big.Int).Mul(amountAtomic, big.NewInt(4))
	diffDecimals := dest.Decimals - origin.Decimals
	if diffDecimals > 0 {
		estAtomic.Mul(estAtomic, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(diffDecimals)), nil))
	} else if diffDecimals < 0 {
		estAtomic.Div(estAtomic, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-diffDecimals)), nil))
	}

	// Min amount out accounting for slippage: est * (10000 - slippage) / 10000
	factor := big.NewInt(int64(10000 - slippage))
	minAtomic := new(big.Int).Mul(estAtomic, factor)
	minAtomic.Div(minAtomic, big.NewInt(10000))

	quote := domain.Quote{
		ID:                      fmt.Sprintf("qte_demo_%d", now.UnixNano()),
		SessionID:               sessionID,
		ProviderName:            "demo_simulation",
		Mode:                    domain.QuoteModeSimulation,
		OriginAssetID:           origin.ID,
		DestinationAssetID:      dest.ID,
		AmountInAtomic:          amountAtomic,
		AmountInDecimal:         amountInDecimal,
		AmountOutEstimateAtomic: estAtomic,
		MinAmountOutAtomic:      minAtomic,
		FeeBPS:                  20,
		SlippageBPS:             slippage,
		ExpiresAt:               now.Add(2 * time.Minute),
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	return s.store.SaveQuote(ctx, quote)
}
