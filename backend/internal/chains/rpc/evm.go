package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
)

// EVMVerifier checks eth_getTransactionReceipt and eth_blockNumber.
type EVMVerifier struct {
	RPC *jsonrpc.Client
}

type evmReceipt struct {
	TransactionHash string  `json:"transactionHash"`
	BlockNumber     string  `json:"blockNumber"`
	Status          string  `json:"status"`
	To              *string `json:"to"`
	Input           string  `json:"input"`
}

// Verify fetches an EVM receipt, checks status/to/calldata, and computes
// confirmations from the latest block.
func (v *EVMVerifier) Verify(ctx context.Context, req Request) (Result, error) {
	if v == nil || v.RPC == nil {
		return Result{}, fmt.Errorf("%w: EVM RPC is not configured", ErrInvalidRequest)
	}
	txHash, err := evm.ParseTxHash(req.TxHash)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	var expectedTo string
	if req.ExpectedTo != "" {
		expectedTo, err = evm.ParseAddress(req.ExpectedTo)
		if err != nil {
			return Result{}, fmt.Errorf("%w: expected to: %v", ErrInvalidRequest, err)
		}
	}
	var expectedData string
	if req.ExpectedData != "" {
		expectedData, err = evm.NormalizeCallData(req.ExpectedData)
		if err != nil {
			return Result{}, fmt.Errorf("%w: expected data: %v", ErrInvalidRequest, err)
		}
	}

	var raw json.RawMessage
	if err := v.RPC.Call(ctx, "eth_getTransactionReceipt", []string{txHash}, &raw); err != nil {
		return Result{}, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return Result{}, ErrTxNotFound
	}
	var receipt evmReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	if receipt.TransactionHash != "" && !strings.EqualFold(receipt.TransactionHash, txHash) {
		return Result{}, fmt.Errorf("%w: receipt hash %q does not match %q", ErrInvalidResponse, receipt.TransactionHash, txHash)
	}
	block, err := parseHexQuantity(receipt.BlockNumber)
	if err != nil {
		return Result{}, fmt.Errorf("%w: block number: %v", ErrInvalidResponse, err)
	}
	result := Result{
		Found:   true,
		Success: receipt.Status == "0x1",
		Block:   block,
		Raw:     raw,
	}
	if receipt.To != nil {
		result.To = *receipt.To
	}
	result.Data = receipt.Input
	if !result.Success {
		return result, fmt.Errorf("%w: status %q", ErrTransactionFailed, receipt.Status)
	}
	if expectedTo != "" {
		if result.To == "" || !strings.EqualFold(result.To, expectedTo) {
			return result, fmt.Errorf("%w: to %q, want %q", ErrVerificationMismatch, result.To, expectedTo)
		}
	}
	if expectedData != "" {
		actual, err := evm.NormalizeCallData(result.Data)
		if err != nil {
			actual = result.Data
		}
		if !strings.EqualFold(actual, expectedData) {
			return result, fmt.Errorf("%w: calldata does not match", ErrVerificationMismatch)
		}
	}

	var latestRaw string
	if err := v.RPC.Call(ctx, "eth_blockNumber", []any{}, &latestRaw); err != nil {
		return result, fmt.Errorf("confirmations: %w", err)
	}
	latest, err := parseHexQuantity(latestRaw)
	if err != nil {
		return result, fmt.Errorf("%w: latest block: %v", ErrInvalidResponse, err)
	}
	if latest >= block {
		result.Confirmations = latest - block + 1
	}
	if result.Confirmations < req.MinConfirmations {
		return result, fmt.Errorf("%w: have %d, want %d", ErrInsufficientConfirmations, result.Confirmations, req.MinConfirmations)
	}
	return result, nil
}

func parseHexQuantity(value string) (uint64, error) {
	if !strings.HasPrefix(value, "0x") || len(value) == 2 || len(value) > 18 {
		return 0, fmt.Errorf("invalid hex quantity %q", value)
	}
	parsed, err := strconv.ParseUint(value[2:], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hex quantity %q", value)
	}
	return parsed, nil
}
