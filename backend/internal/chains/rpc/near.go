package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"universal-defi-intelligence/internal/chains/near"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
	"universal-defi-intelligence/internal/encoding/base58"
)

// NEARVerifier checks JSON-RPC tx status. Sender is required by the NEAR RPC.
type NEARVerifier struct {
	RPC *jsonrpc.Client
}

type nearTxResult struct {
	Status               map[string]json.RawMessage `json:"status"`
	FinalExecutionStatus string                     `json:"final_execution_status"`
	Transaction          struct {
		SignerID   string `json:"signer_id"`
		ReceiverID string `json:"receiver_id"`
	} `json:"transaction"`
}

// Verify checks a NEAR transaction's final execution status.
func (v *NEARVerifier) Verify(ctx context.Context, req Request) (Result, error) {
	if v == nil || v.RPC == nil {
		return Result{}, fmt.Errorf("%w: NEAR RPC is not configured", ErrInvalidRequest)
	}
	hash, err := parseNEARHash(req.TxHash)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if req.Sender == "" {
		return Result{}, fmt.Errorf("%w: NEAR verification requires Sender", ErrInvalidRequest)
	}
	sender, err := near.ParseAccountID(req.Sender)
	if err != nil {
		return Result{}, fmt.Errorf("%w: sender: %v", ErrInvalidRequest, err)
	}
	var expectedTo string
	if req.ExpectedTo != "" {
		expectedTo, err = near.ParseAccountID(req.ExpectedTo)
		if err != nil {
			return Result{}, fmt.Errorf("%w: expected to: %v", ErrInvalidRequest, err)
		}
	}

	var result nearTxResult
	params := map[string]string{
		"tx_hash":           hash,
		"sender_account_id": sender,
		"wait_until":        "FINAL",
	}
	if err := v.RPC.Call(ctx, "tx", params, &result); err != nil {
		if isRPCNotFound(err) {
			return Result{}, ErrTxNotFound
		}
		return Result{}, err
	}
	observation := Result{Found: true}
	failure, hasFailure := result.Status["Failure"]
	_, hasSuccess := result.Status["SuccessValue"]
	switch {
	case hasFailure && len(failure) > 0 && string(failure) != "null":
		observation.Success = false
		return observation, fmt.Errorf("%w: %s", ErrTransactionFailed, failure)
	case hasSuccess:
		observation.Success = true
	default:
		return Result{}, fmt.Errorf("%w: unknown NEAR status", ErrInvalidResponse)
	}
	if result.Transaction.SignerID != "" && result.Transaction.SignerID != sender {
		return observation, fmt.Errorf("%w: signer %q, want %q", ErrVerificationMismatch, result.Transaction.SignerID, sender)
	}
	if expectedTo != "" {
		if result.Transaction.ReceiverID == "" || result.Transaction.ReceiverID != expectedTo {
			return observation, fmt.Errorf("%w: receiver %q, want %q", ErrVerificationMismatch, result.Transaction.ReceiverID, expectedTo)
		}
		observation.To = result.Transaction.ReceiverID
	}
	if result.FinalExecutionStatus == "FINAL" {
		observation.Finalized = true
		observation.Confirmations = req.MinConfirmations
		if observation.Confirmations == 0 {
			observation.Confirmations = 1
		}
	}
	if !observation.Finalized && observation.Confirmations < req.MinConfirmations {
		return observation, fmt.Errorf("%w: have %d, want %d", ErrInsufficientConfirmations, observation.Confirmations, req.MinConfirmations)
	}
	return observation, nil
}

func parseNEARHash(value string) (string, error) {
	raw, err := base58.Decode(value)
	if err != nil {
		return "", err
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("transaction hash must decode to 32 bytes, got %d", len(raw))
	}
	return base58.Encode(raw), nil
}

func isRPCNotFound(err error) bool {
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		return false
	}
	message := strings.ToLower(rpcErr.Message)
	return strings.Contains(message, "not found") ||
		strings.Contains(message, "does not exist") ||
		strings.Contains(message, "doesn't exist") ||
		strings.Contains(message, "unknown transaction")
}
