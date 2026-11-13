// Package rpc verifies deposit transactions against chain RPC endpoints. It is
// read-only: it never signs or broadcasts.
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
)

var (
	// ErrTxNotFound means the chain has not observed the transaction yet.
	ErrTxNotFound = errors.New("transaction not found")
	// ErrTransactionFailed means the transaction is mined but failed.
	ErrTransactionFailed = errors.New("transaction failed on chain")
	// ErrVerificationMismatch means the receipt does not match the expectation.
	ErrVerificationMismatch = errors.New("transaction receipt mismatch")
	// ErrInsufficientConfirmations means the receipt is not yet final enough.
	ErrInsufficientConfirmations = errors.New("insufficient confirmations")
	// ErrUnsupportedChain means the registry has no verifier for the chain.
	ErrUnsupportedChain = errors.New("unsupported chain")
	// ErrInvalidRequest means the verification request is malformed.
	ErrInvalidRequest = errors.New("invalid verification request")
	// ErrInvalidResponse means the RPC returned malformed data.
	ErrInvalidResponse = errors.New("invalid RPC response")
)

// Request describes the deposit transaction being verified.
type Request struct {
	Chain            string
	TxHash           string
	ExpectedTo       string
	ExpectedData     string
	Sender           string
	MinConfirmations uint64
}

// Result is the normalized on-chain observation.
type Result struct {
	Found         bool            `json:"found"`
	Success       bool            `json:"success"`
	Finalized     bool            `json:"finalized"`
	Confirmations uint64          `json:"confirmations"`
	Block         uint64          `json:"block"`
	To            string          `json:"to,omitempty"`
	Data          string          `json:"data,omitempty"`
	Raw           json.RawMessage `json:"-"`
}

// Confirmed reports whether the result satisfies a confirmation floor.
func (r Result) Confirmed(min uint64) bool {
	if r.Finalized {
		return r.Found && r.Success
	}
	return r.Found && r.Success && r.Confirmations >= min
}

// ReceiptVerifier verifies a transaction across supported chain families.
type ReceiptVerifier interface {
	Verify(ctx context.Context, req Request) (Result, error)
}

// MultiChainVerifier dispatches to a family-specific verifier using registry
// RPC URLs. Endpoints may be overridden for tests or private nodes.
type MultiChainVerifier struct {
	Registry   *registry.Registry
	HTTPClient *http.Client
	Timeout    time.Duration
	Endpoints  map[string][]string
}

// Verify dispatches by chain family.
func (v *MultiChainVerifier) Verify(ctx context.Context, req Request) (Result, error) {
	if v == nil || v.Registry == nil {
		return Result{}, fmt.Errorf("%w: registry is not configured", ErrUnsupportedChain)
	}
	chain, ok := v.Registry.Get(req.Chain)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrUnsupportedChain, req.Chain)
	}
	urls := v.endpoints(chain)
	timeout := v.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	switch chain.Family {
	case registry.FamilyEVM:
		return (&EVMVerifier{RPC: &jsonrpc.Client{URLs: urls, HTTPClient: v.HTTPClient, Timeout: timeout}}).Verify(ctx, req)
	case registry.FamilySolana:
		return (&SolanaVerifier{RPC: &jsonrpc.Client{URLs: urls, HTTPClient: v.HTTPClient, Timeout: timeout}}).Verify(ctx, req)
	case registry.FamilyStellar:
		return (&StellarVerifier{HTTP: &HTTPFetcher{URLs: urls, Client: v.HTTPClient, Timeout: timeout}}).Verify(ctx, req)
	case registry.FamilyMove:
		return (&AptosVerifier{HTTP: &HTTPFetcher{URLs: urls, Client: v.HTTPClient, Timeout: timeout}}).Verify(ctx, req)
	case registry.FamilyNEAR:
		return (&NEARVerifier{RPC: &jsonrpc.Client{URLs: urls, HTTPClient: v.HTTPClient, Timeout: timeout}}).Verify(ctx, req)
	default:
		return Result{}, fmt.Errorf("%w: family %q", ErrUnsupportedChain, chain.Family)
	}
}

func (v *MultiChainVerifier) endpoints(chain registry.Chain) []string {
	if len(v.Endpoints[chain.Key]) > 0 {
		return append([]string(nil), v.Endpoints[chain.Key]...)
	}
	return append([]string(nil), chain.RPCURLs...)
}
