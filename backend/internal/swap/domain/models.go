// Package domain contains the provider-agnostic swap models, exact decimal
// math, lifecycle state machine, and typed errors.
//
// It has no dependency on HTTP, PostgreSQL, Redis, or any provider. This keeps
// the business rules testable in isolation and reusable by the API and worker.
package domain

import (
	"math/big"
	"time"
)

// Asset is a tradeable token known to the catalogue.
type Asset struct {
	ID              string
	ProviderAssetID string
	Chain           string
	Symbol          string
	Name            string
	Decimals        int
	ContractAddress string
	Source          string
	ObservedAt      time.Time
}

// QuoteMode describes where a quote came from.
type QuoteMode string

const (
	QuoteModeSimulation         QuoteMode = "simulation"
	QuoteModeProviderDry        QuoteMode = "provider_dry_quote"
	QuoteModeProviderExecutable QuoteMode = "provider_executable_quote"
)

// Quote is a normalized provider or simulation quote.
type Quote struct {
	ID                      string
	SessionID               string
	ProviderQuoteID         string
	ProviderName            string
	Mode                    QuoteMode
	OriginAssetID           string
	DestinationAssetID      string
	AmountInAtomic          *big.Int
	AmountInDecimal         string
	AmountOutEstimateAtomic *big.Int
	MinAmountOutAtomic      *big.Int
	FeeBPS                  int
	SlippageBPS             int
	DepositAddress          string
	DepositMemo             string
	ExpiresAt               time.Time
	RawResponse             []byte
	CreatedAt               time.Time
}

// TransactionRole identifies a transaction attached to a swap.
type TransactionRole string

const (
	TransactionRoleDeposit     TransactionRole = "deposit"
	TransactionRoleDestination TransactionRole = "destination"
	TransactionRoleRefund      TransactionRole = "refund"
)

// Deposit records a source-chain transaction reported by the user.
type Deposit struct {
	SwapID     string
	ChainKey   string
	TxHash     string
	Role       TransactionRole
	ObservedAt time.Time
}

// Swap is the persistent swap lifecycle record.
type Swap struct {
	ID               string
	QuoteID          string
	IdempotencyKey   string
	SessionID        string
	State            State
	ProviderStatus   ProviderStatus
	RealSettlement   bool
	OriginChain      string
	DestinationChain string
	DepositAddress   string
	DepositMemo      string
	RecipientAddress string
	RefundAddress    string
	SourceTxHash     string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Event is an immutable audit entry for a swap state change. OldState is empty
// for the initial CREATED event.
type Event struct {
	SwapID         string
	OldState       State
	NewState       State
	ProviderStatus ProviderStatus
	Payload        map[string]any
	CreatedAt      time.Time
}
