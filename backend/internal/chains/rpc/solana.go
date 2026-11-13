package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
	"universal-defi-intelligence/internal/encoding/base58"
)

// SolanaVerifier checks getSignatureStatuses.
type SolanaVerifier struct {
	RPC *jsonrpc.Client
}

type solanaSignatureStatus struct {
	Slot               uint64          `json:"slot"`
	Confirmations      *uint64         `json:"confirmations"`
	Err                json.RawMessage `json:"err"`
	ConfirmationStatus string          `json:"confirmationStatus"`
}

type solanaStatusResult struct {
	Value []*solanaSignatureStatus `json:"value"`
}

// Verify checks a Solana signature status. Destination cannot be inferred from
// the status endpoint, so ExpectedTo is ignored.
func (v *SolanaVerifier) Verify(ctx context.Context, req Request) (Result, error) {
	if v == nil || v.RPC == nil {
		return Result{}, fmt.Errorf("%w: Solana RPC is not configured", ErrInvalidRequest)
	}
	if _, err := parseSolanaSignature(req.TxHash); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	var status solanaStatusResult
	params := []any{[]string{req.TxHash}, map[string]any{"searchTransactionHistory": true}}
	if err := v.RPC.Call(ctx, "getSignatureStatuses", params, &status); err != nil {
		return Result{}, err
	}
	if len(status.Value) == 0 || status.Value[0] == nil {
		return Result{}, ErrTxNotFound
	}
	entry := status.Value[0]
	result := Result{
		Found:     true,
		Success:   true,
		Finalized: entry.ConfirmationStatus == "finalized",
		Block:     entry.Slot,
	}
	if entry.Confirmations != nil {
		result.Confirmations = *entry.Confirmations
	}
	if len(entry.Err) > 0 && string(entry.Err) != "null" {
		result.Success = false
		return result, fmt.Errorf("%w: %s", ErrTransactionFailed, entry.Err)
	}
	if !result.Finalized && result.Confirmations == 0 && entry.ConfirmationStatus != "" {
		var latest uint64
		if err := v.RPC.Call(ctx, "getSlot", []any{map[string]string{"commitment": "finalized"}}, &latest); err == nil && latest >= entry.Slot {
			result.Confirmations = latest - entry.Slot + 1
		}
	}
	if !result.Finalized && result.Confirmations < req.MinConfirmations {
		return result, fmt.Errorf("%w: have %d, want %d", ErrInsufficientConfirmations, result.Confirmations, req.MinConfirmations)
	}
	return result, nil
}

func parseSolanaSignature(value string) ([]byte, error) {
	raw, err := base58.Decode(value)
	if err != nil {
		return nil, err
	}
	if len(raw) != 64 {
		return nil, fmt.Errorf("signature must decode to 64 bytes, got %d", len(raw))
	}
	return raw, nil
}
