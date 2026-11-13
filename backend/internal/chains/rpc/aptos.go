package rpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"universal-defi-intelligence/internal/chains/aptos"
)

// AptosVerifier checks Aptos transaction success and ledger version.
type AptosVerifier struct {
	HTTP *HTTPFetcher
}

type aptosTransaction struct {
	Hash    string `json:"hash"`
	Success bool   `json:"success"`
	Version string `json:"version"`
	Type    string `json:"type"`
	Payload *struct {
		Type      string   `json:"type"`
		Function  string   `json:"function"`
		Arguments []string `json:"arguments"`
	} `json:"payload"`
}

type aptosLedger struct {
	LedgerVersion string `json:"ledger_version"`
}

// Verify fetches an Aptos transaction and compares the optional recipient.
func (v *AptosVerifier) Verify(ctx context.Context, req Request) (Result, error) {
	if v == nil || v.HTTP == nil {
		return Result{}, fmt.Errorf("%w: Aptos HTTP client is not configured", ErrInvalidRequest)
	}
	hash, err := parseAptosHash(req.TxHash)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	var tx aptosTransaction
	if err := v.HTTP.Get(ctx, "/transactions/by_hash/"+hash, &tx); err != nil {
		var status *StatusError
		if errors.As(err, &status) && status.NotFound() {
			return Result{}, ErrTxNotFound
		}
		return Result{}, err
	}
	version, err := strconv.ParseUint(tx.Version, 10, 64)
	if err != nil {
		return Result{}, fmt.Errorf("%w: version %q", ErrInvalidResponse, tx.Version)
	}
	result := Result{
		Found:   true,
		Success: tx.Success,
		Block:   version,
	}
	if !result.Success {
		return result, ErrTransactionFailed
	}
	if req.ExpectedTo != "" && tx.Payload != nil && len(tx.Payload.Arguments) > 0 {
		want, err := aptos.ParseAddress(req.ExpectedTo)
		if err != nil {
			return Result{}, fmt.Errorf("%w: expected to: %v", ErrInvalidRequest, err)
		}
		got, err := aptos.ParseAddress(tx.Payload.Arguments[0])
		if err != nil || got != want {
			return result, fmt.Errorf("%w: recipient %q, want %q", ErrVerificationMismatch, tx.Payload.Arguments[0], want)
		}
		result.To = got
	}
	var ledger aptosLedger
	if err := v.HTTP.Get(ctx, "/", &ledger); err == nil {
		if latest, err := strconv.ParseUint(ledger.LedgerVersion, 10, 64); err == nil && latest >= version {
			result.Confirmations = latest - version + 1
		}
	}
	if result.Confirmations < req.MinConfirmations {
		return result, fmt.Errorf("%w: have %d, want %d", ErrInsufficientConfirmations, result.Confirmations, req.MinConfirmations)
	}
	return result, nil
}

func parseAptosHash(value string) (string, error) {
	body := strings.TrimPrefix(value, "0x")
	if len(body) != 64 {
		return "", fmt.Errorf("hash must be 64 hex characters")
	}
	if _, err := hex.DecodeString(body); err != nil {
		return "", fmt.Errorf("hash must be hex")
	}
	return "0x" + strings.ToLower(body), nil
}
