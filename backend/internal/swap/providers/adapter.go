// Package providers defines the provider-agnostic swap adapter contract.
//
// Every adapter speaks only in domain types and typed domain errors. It keeps
// HTTP details, credentials, and provider quirks behind the interface so the
// orchestration layer and the worker never depend on a specific vendor.
package providers

import (
	"context"
	"math/big"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

// Capabilities describes what an adapter can do. Callers must check these
// before assuming a route exists.
type Capabilities struct {
	Catalogue       bool
	DryQuote        bool
	ExecutableQuote bool
	Status          bool
	SubmitDeposit   bool
	// SignatureVerification reports whether the adapter has a configured key
	// and will fail closed on an invalid quote signature.
	SignatureVerification bool
}

// Health is a point-in-time view of adapter configuration. It deliberately does
// not track request statistics; the worker health tracker owns those.
type Health struct {
	// Configured reports whether the adapter has the credentials it needs for
	// authenticated operations.
	Configured bool
	// SignatureVerification mirrors Capabilities.SignatureVerification.
	SignatureVerification bool
}

// DryQuoteRequest is a request for a non-executable provider estimate.
type DryQuoteRequest struct {
	OriginAsset      domain.Asset
	DestinationAsset domain.Asset
	AmountAtomic     *big.Int
	Recipient        string
	RefundTo         string
	SlippageBPS      int
}

// ExecutableQuoteRequest asks for a quote that carries deposit instructions.
type ExecutableQuoteRequest struct {
	DryQuoteRequest
	Deadline    time.Time
	DepositMode string
}

// StatusRequest identifies the provider deposit being polled.
type StatusRequest struct {
	DepositAddress string
	DepositMemo    string
}

// DepositRequest notifies the provider that a source transaction was sent.
type DepositRequest struct {
	TxHash         string
	DepositAddress string
	DepositMemo    string
}

// StatusResult is a validated provider observation.
type StatusResult struct {
	ProviderStatus    domain.ProviderStatus
	UpdatedAt         time.Time
	SwapDetails       map[string]any
	SourceTxHash      string
	DestinationTxHash string
	RefundTxHash      string
	Raw               []byte
}

// QuoteResult is a normalized quote plus provider-side verification metadata.
type QuoteResult struct {
	Quote                 domain.Quote
	ProviderQuoteID       string
	SignatureVerified     bool
	ProviderTimestamp     time.Time
	TimeWhenInactive      time.Time
	TimeEstimateSeconds   int
	AmountOutFormatted    string
	MinAmountOutFormatted string
	Warning               string
	Raw                   []byte
}

// Adapter is the full provider contract.
type Adapter interface {
	// Name is the stable provider identifier used in logs, health, and storage.
	Name() string
	// Capabilities describes supported operations.
	Capabilities() Capabilities
	// Health reports configuration state without making a network call.
	Health() Health
	// GetTokens returns the validated provider catalogue.
	GetTokens(ctx context.Context) ([]domain.Asset, error)
	// GetDryQuote returns a non-executable estimate with no deposit data.
	GetDryQuote(ctx context.Context, req DryQuoteRequest) (QuoteResult, error)
	// GetExecutableQuote returns a quote with deposit instructions.
	GetExecutableQuote(ctx context.Context, req ExecutableQuoteRequest) (QuoteResult, error)
	// GetStatus returns the current provider status for a deposit.
	GetStatus(ctx context.Context, req StatusRequest) (StatusResult, error)
	// SubmitDepositTx is an accelerator; it must not be retried blindly.
	SubmitDepositTx(ctx context.Context, req DepositRequest) (StatusResult, error)
}
