// Package repository implements the PostgreSQL persistence boundary for
// quotes, swaps, transactions, and immutable swap events.
//
// All amount columns use NUMERIC(78, 0) and are converted to/from *big.Int.
// Floating point is never used for an on-chain amount. Multi-row lifecycle
// writes (a state update plus its audit event, or an idempotent swap create
// plus its initial event) run in one transaction.
package repository

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/domain"
)

const (
	// DefaultListLimit is the page size used when the caller omits a limit.
	DefaultListLimit = 50
	// MaxListLimit caps a single keyset page.
	MaxListLimit = 100
)

// Store is the PostgreSQL repository for the swap bounded context.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store from an initialized pgx pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool exposes the underlying pool for callers that need to compose their own
// transaction around several repository writes.
func (s *Store) Pool() *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.pool
}

func (s *Store) ready() error {
	if s == nil || s.pool == nil {
		return domain.New(domain.CodeInternal, "The database is not configured.")
	}
	return nil
}

// Cursor is the keyset position for swap listing: created_at DESC, id DESC.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// EncodeCursor serializes a keyset position into an opaque URL-safe token.
func EncodeCursor(cursor Cursor) string {
	payload := cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + cursor.ID
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// DecodeCursor parses an opaque keyset token.
func DecodeCursor(raw string) (Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return Cursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	createdAt, id, ok := strings.Cut(string(decoded), "|")
	if !ok || strings.TrimSpace(id) == "" {
		return Cursor{}, errors.New("cursor is missing a record id")
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Cursor{}, fmt.Errorf("parse cursor timestamp: %w", err)
	}
	return Cursor{CreatedAt: parsed.UTC(), ID: id}, nil
}

// rowScanner is implemented by pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

const quoteColumns = `id,
	COALESCE(session_id, ''),
	COALESCE(provider_quote_id, ''),
	provider_name,
	mode,
	origin_asset_id,
	destination_asset_id,
	amount_in_atomic,
	amount_in_decimal,
	amount_out_estimate_atomic,
	min_amount_out_atomic,
	fee_bps,
	slippage_bps,
	COALESCE(fee_data, '{}'::jsonb),
	COALESCE(deposit_address, ''),
	COALESCE(deposit_memo, ''),
	expires_at,
	COALESCE(raw_response_json, '{}'::jsonb),
	created_at,
	updated_at`

const swapColumns = `id,
	quote_id,
	idempotency_key,
	idempotency_payload_hash,
	COALESCE(session_id, ''),
	state,
	COALESCE(provider_status, ''),
	real_settlement,
	origin_chain,
	destination_chain,
	COALESCE(deposit_address, ''),
	COALESCE(deposit_memo, ''),
	recipient_address,
	refund_address,
	COALESCE(source_tx_hash, ''),
	expires_at,
	next_poll_at,
	poll_attempts,
	last_polled_at,
	created_at,
	updated_at`

const transactionColumns = `id, swap_id, chain_key, tx_hash, tx_role, observed_at, created_at`

const eventColumns = `id,
	swap_id,
	COALESCE(old_state, ''),
	new_state,
	COALESCE(provider_status, ''),
	COALESCE(event_payload, '{}'::jsonb),
	created_at`

func scanQuote(row rowScanner) (domain.Quote, error) {
	var quote domain.Quote
	var mode string
	var amountIn, amountOut, minAmountOut pgtype.Numeric
	var feeData, rawResponse []byte

	err := row.Scan(
		&quote.ID,
		&quote.SessionID,
		&quote.ProviderQuoteID,
		&quote.ProviderName,
		&mode,
		&quote.OriginAssetID,
		&quote.DestinationAssetID,
		&amountIn,
		&quote.AmountInDecimal,
		&amountOut,
		&minAmountOut,
		&quote.FeeBPS,
		&quote.SlippageBPS,
		&feeData,
		&quote.DepositAddress,
		&quote.DepositMemo,
		&quote.ExpiresAt,
		&rawResponse,
		&quote.CreatedAt,
		&quote.UpdatedAt,
	)
	if err != nil {
		return domain.Quote{}, err
	}

	quote.Mode = domain.QuoteMode(mode)
	if quote.AmountInAtomic, err = bigIntFromNumeric(amountIn); err != nil {
		return domain.Quote{}, err
	}
	if quote.AmountOutEstimateAtomic, err = bigIntFromNumeric(amountOut); err != nil {
		return domain.Quote{}, err
	}
	if quote.MinAmountOutAtomic, err = bigIntFromNumeric(minAmountOut); err != nil {
		return domain.Quote{}, err
	}
	if len(feeData) > 0 {
		if err := json.Unmarshal(feeData, &quote.FeeData); err != nil {
			return domain.Quote{}, domain.Wrap(domain.CodeInternal, "The stored quote fee data is invalid.", err)
		}
	}
	quote.RawResponse = rawResponse
	quote.ExpiresAt = quote.ExpiresAt.UTC()
	quote.CreatedAt = quote.CreatedAt.UTC()
	quote.UpdatedAt = quote.UpdatedAt.UTC()
	return quote, nil
}

func scanSwap(row rowScanner) (domain.Swap, error) {
	var swap domain.Swap
	var state, providerStatus string
	var lastPolledAt pgtype.Timestamptz

	err := row.Scan(
		&swap.ID,
		&swap.QuoteID,
		&swap.IdempotencyKey,
		&swap.IdempotencyPayloadHash,
		&swap.SessionID,
		&state,
		&providerStatus,
		&swap.RealSettlement,
		&swap.OriginChain,
		&swap.DestinationChain,
		&swap.DepositAddress,
		&swap.DepositMemo,
		&swap.RecipientAddress,
		&swap.RefundAddress,
		&swap.SourceTxHash,
		&swap.ExpiresAt,
		&swap.NextPollAt,
		&swap.PollAttempts,
		&lastPolledAt,
		&swap.CreatedAt,
		&swap.UpdatedAt,
	)
	if err != nil {
		return domain.Swap{}, err
	}

	swap.State = domain.State(state)
	swap.ProviderStatus = domain.ProviderStatus(providerStatus)
	if lastPolledAt.Valid {
		swap.LastPolledAt = lastPolledAt.Time.UTC()
	}
	swap.ExpiresAt = swap.ExpiresAt.UTC()
	swap.NextPollAt = swap.NextPollAt.UTC()
	swap.CreatedAt = swap.CreatedAt.UTC()
	swap.UpdatedAt = swap.UpdatedAt.UTC()
	return swap, nil
}

func scanDeposit(row rowScanner) (domain.Deposit, error) {
	var deposit domain.Deposit
	var role string
	err := row.Scan(
		&deposit.ID,
		&deposit.SwapID,
		&deposit.ChainKey,
		&deposit.TxHash,
		&role,
		&deposit.ObservedAt,
		&deposit.CreatedAt,
	)
	if err != nil {
		return domain.Deposit{}, err
	}
	deposit.Role = domain.TransactionRole(role)
	deposit.ObservedAt = deposit.ObservedAt.UTC()
	deposit.CreatedAt = deposit.CreatedAt.UTC()
	return deposit, nil
}

func scanEvent(row rowScanner) (domain.Event, error) {
	var event domain.Event
	var oldState, newState, providerStatus string
	var payload []byte
	err := row.Scan(
		&event.ID,
		&event.SwapID,
		&oldState,
		&newState,
		&providerStatus,
		&payload,
		&event.CreatedAt,
	)
	if err != nil {
		return domain.Event{}, err
	}
	event.OldState = domain.State(oldState)
	event.NewState = domain.State(newState)
	event.ProviderStatus = domain.ProviderStatus(providerStatus)
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &event.Payload); err != nil {
			return domain.Event{}, domain.Wrap(domain.CodeInternal, "The stored swap event payload is invalid.", err)
		}
	}
	event.CreatedAt = event.CreatedAt.UTC()
	return event, nil
}

func numericFromBigInt(value *big.Int) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	return pgtype.Numeric{Int: new(big.Int).Set(value), Exp: 0, Valid: true}
}

func bigIntFromNumeric(value pgtype.Numeric) (*big.Int, error) {
	if !value.Valid {
		return nil, nil
	}
	if value.NaN || value.InfinityModifier != pgtype.Finite {
		return nil, domain.New(domain.CodeInvalidAmount, "The stored amount is not a finite number.")
	}
	if value.Int == nil {
		return big.NewInt(0), nil
	}

	result := new(big.Int).Set(value.Int)
	switch {
	case value.Exp == 0:
		return result, nil
	case value.Exp > 0:
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(value.Exp)), nil)
		return result.Mul(result, scale), nil
	default:
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-value.Exp)), nil)
		quotient, remainder := new(big.Int).QuoRem(result, divisor, new(big.Int))
		if remainder.Sign() != 0 {
			return nil, domain.New(domain.CodeInvalidAmount, "The stored amount has fractional precision.")
		}
		return quotient, nil
	}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableJSON(value map[string]any) any {
	if value == nil {
		return nil
	}
	return value
}

func nullableRawJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return json.RawMessage(value)
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func notFound(err error, code domain.Code, message string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wrap(code, message, err)
	}
	return postgres.MapError(err)
}

func normalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultListLimit, nil
	}
	if limit < 1 || limit > MaxListLimit {
		return 0, domain.Newf(
			domain.CodeValidationFailed,
			"limit must be between 1 and %d.",
			MaxListLimit,
		)
	}
	return limit, nil
}

func normalizePayloadHash(hash string) (string, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if len(hash) != 64 {
		return "", domain.New(domain.CodeValidationFailed, "Idempotency payload hash must be a 64-character SHA-256 hex digest.")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", domain.New(domain.CodeValidationFailed, "Idempotency payload hash must be hexadecimal.")
	}
	return hash, nil
}

func normalizeObservedAt(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}
