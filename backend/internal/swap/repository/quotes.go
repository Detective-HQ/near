package repository

import (
	"context"
	"strings"
	"time"

	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/domain"
)

// SaveQuote persists a normalized quote. The atomic amounts are written as
// NUMERIC(78, 0); the provider's raw JSON is retained for audit and debugging.
func (s *Store) SaveQuote(ctx context.Context, quote domain.Quote) (domain.Quote, error) {
	if err := s.ready(); err != nil {
		return domain.Quote{}, err
	}
	if err := validateQuote(quote); err != nil {
		return domain.Quote{}, err
	}

	now := time.Now().UTC()
	if quote.CreatedAt.IsZero() {
		quote.CreatedAt = now
	} else {
		quote.CreatedAt = quote.CreatedAt.UTC()
	}
	if quote.UpdatedAt.IsZero() {
		quote.UpdatedAt = quote.CreatedAt
	} else {
		quote.UpdatedAt = quote.UpdatedAt.UTC()
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO quotes (
			id, session_id, provider_quote_id, provider_name, mode,
			origin_asset_id, destination_asset_id, amount_in_atomic,
			amount_in_decimal, amount_out_estimate_atomic, min_amount_out_atomic,
			fee_bps, slippage_bps, fee_data, deposit_address, deposit_memo,
			expires_at, raw_response_json, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, $14, $15, $16,
			$17, $18, $19, $20
		)
		RETURNING `+quoteColumns,
		quote.ID,
		nullableString(quote.SessionID),
		nullableString(quote.ProviderQuoteID),
		quote.ProviderName,
		string(quote.Mode),
		quote.OriginAssetID,
		quote.DestinationAssetID,
		numericFromBigInt(quote.AmountInAtomic),
		quote.AmountInDecimal,
		numericFromBigInt(quote.AmountOutEstimateAtomic),
		numericFromBigInt(quote.MinAmountOutAtomic),
		quote.FeeBPS,
		quote.SlippageBPS,
		nullableJSON(quote.FeeData),
		nullableString(quote.DepositAddress),
		nullableString(quote.DepositMemo),
		quote.ExpiresAt,
		nullableRawJSON(quote.RawResponse),
		quote.CreatedAt,
		quote.UpdatedAt,
	)

	saved, err := scanQuote(row)
	if err != nil {
		return domain.Quote{}, postgres.MapError(err)
	}
	return saved, nil
}

// GetQuote returns a quote by id. Expiry is a service concern, so an expired
// record is still returned for audit and replay decisions.
func (s *Store) GetQuote(ctx context.Context, id string) (domain.Quote, error) {
	if err := s.ready(); err != nil {
		return domain.Quote{}, err
	}
	if strings.TrimSpace(id) == "" {
		return domain.Quote{}, domain.New(domain.CodeValidationFailed, "Quote id is required.")
	}

	row := s.pool.QueryRow(ctx, `SELECT `+quoteColumns+` FROM quotes WHERE id = $1`, id)
	quote, err := scanQuote(row)
	if err != nil {
		return domain.Quote{}, notFound(err, domain.CodeQuoteNotFound, "Quote not found.")
	}
	return quote, nil
}

func validateQuote(quote domain.Quote) error {
	if strings.TrimSpace(quote.ID) == "" {
		return domain.New(domain.CodeValidationFailed, "Quote id is required.")
	}
	if strings.TrimSpace(quote.ProviderName) == "" {
		return domain.New(domain.CodeValidationFailed, "Quote provider name is required.")
	}
	switch quote.Mode {
	case domain.QuoteModeSimulation, domain.QuoteModeProviderDry, domain.QuoteModeProviderExecutable:
	default:
		return domain.Newf(domain.CodeValidationFailed, "Unknown quote mode %q.", quote.Mode)
	}
	if strings.TrimSpace(quote.OriginAssetID) == "" || strings.TrimSpace(quote.DestinationAssetID) == "" {
		return domain.New(domain.CodeValidationFailed, "Quote origin and destination assets are required.")
	}
	if quote.AmountInAtomic == nil || quote.AmountInAtomic.Sign() <= 0 {
		return domain.New(domain.CodeValidationFailed, "Quote amount_in_atomic must be greater than zero.")
	}
	if quote.AmountOutEstimateAtomic == nil || quote.AmountOutEstimateAtomic.Sign() < 0 {
		return domain.New(domain.CodeValidationFailed, "Quote amount_out_estimate_atomic must be zero or greater.")
	}
	if quote.MinAmountOutAtomic != nil && quote.MinAmountOutAtomic.Sign() < 0 {
		return domain.New(domain.CodeValidationFailed, "Quote min_amount_out_atomic must be zero or greater.")
	}
	if strings.TrimSpace(quote.AmountInDecimal) == "" {
		return domain.New(domain.CodeValidationFailed, "Quote amount_in_decimal is required.")
	}
	if quote.ExpiresAt.IsZero() {
		return domain.New(domain.CodeValidationFailed, "Quote expiry is required.")
	}
	if quote.FeeBPS < 0 || quote.FeeBPS > 10000 {
		return domain.New(domain.CodeValidationFailed, "Quote fee_bps must be between 0 and 10000.")
	}
	if quote.SlippageBPS < 0 || quote.SlippageBPS > 10000 {
		return domain.New(domain.CodeValidationFailed, "Quote slippage_bps must be between 0 and 10000.")
	}
	return nil
}
