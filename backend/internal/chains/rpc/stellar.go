package rpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// StellarVerifier checks Horizon transactions and ledger close height.
type StellarVerifier struct {
	HTTP *HTTPFetcher
}

type horizonTransaction struct {
	Hash          string `json:"hash"`
	Successful    bool   `json:"successful"`
	Ledger        uint64 `json:"ledger"`
	SourceAccount string `json:"source_account"`
}

type horizonLedgers struct {
	Embedded struct {
		Records []struct {
			Sequence uint64 `json:"sequence"`
		} `json:"records"`
	} `json:"_embedded"`
}

// Verify fetches a Horizon transaction and computes ledger confirmations.
func (v *StellarVerifier) Verify(ctx context.Context, req Request) (Result, error) {
	if v == nil || v.HTTP == nil {
		return Result{}, fmt.Errorf("%w: Stellar HTTP client is not configured", ErrInvalidRequest)
	}
	hash, err := parseStellarHash(req.TxHash)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	var tx horizonTransaction
	if err := v.HTTP.Get(ctx, "/transactions/"+hash, &tx); err != nil {
		var status *StatusError
		if errors.As(err, &status) && status.NotFound() {
			return Result{}, ErrTxNotFound
		}
		return Result{}, err
	}
	if tx.Hash != "" && !strings.EqualFold(tx.Hash, hash) {
		return Result{}, fmt.Errorf("%w: hash %q does not match %q", ErrInvalidResponse, tx.Hash, hash)
	}
	if tx.Ledger == 0 {
		return Result{}, fmt.Errorf("%w: transaction has no ledger", ErrInvalidResponse)
	}
	result := Result{
		Found:     true,
		Success:   tx.Successful,
		Finalized: tx.Successful,
		Block:     tx.Ledger,
	}
	if !result.Success {
		return result, ErrTransactionFailed
	}
	if latest, err := latestStellarLedger(ctx, v.HTTP); err == nil && latest >= tx.Ledger {
		result.Confirmations = latest - tx.Ledger + 1
	}
	if !result.Finalized && result.Confirmations < req.MinConfirmations {
		return result, fmt.Errorf("%w: have %d, want %d", ErrInsufficientConfirmations, result.Confirmations, req.MinConfirmations)
	}
	return result, nil
}

func latestStellarLedger(ctx context.Context, fetcher *HTTPFetcher) (uint64, error) {
	var ledgers horizonLedgers
	if err := fetcher.Get(ctx, "/ledgers?order=desc&limit=1", &ledgers); err != nil {
		return 0, err
	}
	if len(ledgers.Embedded.Records) == 0 {
		return 0, ErrInvalidResponse
	}
	return ledgers.Embedded.Records[0].Sequence, nil
}

func parseStellarHash(value string) (string, error) {
	value = strings.TrimPrefix(value, "0x")
	if len(value) != 64 {
		return "", fmt.Errorf("hash must be 64 hex characters")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("hash must be hex")
	}
	return strings.ToLower(value), nil
}
