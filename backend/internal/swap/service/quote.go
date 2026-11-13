package service

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/providers"
)

const (
	defaultSlippageBPS = 100
	maxSlippageBPS     = 5000
	demoQuoteLifetime  = 2 * time.Minute
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

// DryQuote runs the dry quoting pipeline: exact decimal parsing, catalogue
// lookup, chain-specific address validation, then either an illustrative
// simulation estimate (demo mode) or a provider dry quote (live mode). The
// quote is persisted; no swap record is created.
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
	if slippage < 0 || slippage > maxSlippageBPS {
		return domain.Quote{}, domain.New(domain.CodeInvalidSlippage, "Slippage must be between 0 and 5000 basis points.")
	}
	if slippage == 0 {
		slippage = defaultSlippageBPS
	}

	originAsset, err := s.resolveAsset(ctx, originID)
	if err != nil {
		return domain.Quote{}, err
	}
	destAsset, err := s.resolveAsset(ctx, destID)
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

	demo := s.quoteMode == config.QuoteModeDemo
	recipient := strings.TrimSpace(params.Recipient)
	refundTo := strings.TrimSpace(params.RefundTo)
	if !demo {
		if recipient == "" {
			return domain.Quote{}, domain.New(domain.CodeRecipientRequired, "Recipient address is required.")
		}
		if refundTo == "" {
			return domain.Quote{}, domain.New(domain.CodeRefundRequired, "Refund address is required.")
		}
	}
	// Addresses are optional for an illustrative simulation, but anything the
	// user does supply must still be valid for its chain.
	if recipient != "" {
		if recipient, err = ValidateChainAddress(s.registry, destAsset.Chain, recipient); err != nil {
			return domain.Quote{}, err
		}
	}
	if refundTo != "" {
		if refundTo, err = ValidateChainAddress(s.registry, originAsset.Chain, refundTo); err != nil {
			return domain.Quote{}, domain.Wrap(domain.CodeInvalidRefund, "Invalid refund address.", err)
		}
	}

	now := s.now()
	if demo {
		return s.createDemoQuote(ctx, originAsset, destAsset, amountAtomic, params.AmountIn, recipient, refundTo, slippage, params.SessionID, now)
	}

	quoteRes, err := s.provider.GetDryQuote(ctx, providers.DryQuoteRequest{
		OriginAsset:      originAsset,
		DestinationAsset: destAsset,
		AmountAtomic:     amountAtomic,
		Recipient:        recipient,
		RefundTo:         refundTo,
		SlippageBPS:      slippage,
	})
	s.observe(err)
	if err != nil {
		return domain.Quote{}, err
	}

	quote := quoteRes.Quote
	quote.ID = "qte_" + uuid.NewString()
	quote.SessionID = params.SessionID
	quote.Mode = domain.QuoteModeProviderDry
	quote.OriginAssetID = originAsset.ID
	quote.DestinationAssetID = destAsset.ID
	quote.AmountInAtomic = amountAtomic
	quote.AmountInDecimal = strings.TrimSpace(params.AmountIn)
	quote.SlippageBPS = slippage
	quote.Recipient = recipient
	quote.RefundTo = refundTo
	if quote.ProviderName == "" {
		quote.ProviderName = s.provider.Name()
	}
	if quote.ExpiresAt.IsZero() {
		quote.ExpiresAt = now.Add(s.quoteDeadline)
	}
	quote.CreatedAt = now
	quote.UpdatedAt = now
	return s.store.SaveQuote(ctx, quote)
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
	estimate := demoAmountOut(amountAtomic, origin, dest, demoSimulationFeeBPS)
	quote := domain.Quote{
		ID:                      "qte_demo_" + uuid.NewString(),
		SessionID:               sessionID,
		ProviderName:            "simulation-engine",
		Mode:                    domain.QuoteModeSimulation,
		OriginAssetID:           origin.ID,
		DestinationAssetID:      dest.ID,
		AmountInAtomic:          amountAtomic,
		AmountInDecimal:         strings.TrimSpace(amountInDecimal),
		AmountOutEstimateAtomic: estimate,
		MinAmountOutAtomic:      applyBPS(estimate, slippage),
		FeeBPS:                  demoSimulationFeeBPS,
		SlippageBPS:             slippage,
		Recipient:               recipient,
		RefundTo:                refundTo,
		ExpiresAt:               now.Add(demoQuoteLifetime),
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	return s.store.SaveQuote(ctx, quote)
}
