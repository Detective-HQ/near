// Package wallets builds unsigned, non-custodial transfer payloads. It never
// accepts private keys and never signs.
package wallets

import (
	"errors"
	"fmt"
	"strconv"

	"universal-defi-intelligence/internal/chains/evm"
)

// EVMTransferRequest describes an EVM native or ERC-20 deposit.
type EVMTransferRequest struct {
	ChainID string
	To      string
	Token   string
	Amount  string
	Memo    string
}

// EVMPayload is the unsigned transaction a browser wallet must sign.
type EVMPayload struct {
	To      string `json:"to"`
	Value   string `json:"value"`
	Data    string `json:"data"`
	ChainID string `json:"chainId"`
}

// BuildEVMTransfer returns unsigned EVM transaction fields. Memos are rejected
// because the EVM deposit path cannot safely attach them.
func BuildEVMTransfer(req EVMTransferRequest) (EVMPayload, error) {
	chainID, err := evm.ParseChainID(req.ChainID)
	if err != nil {
		return EVMPayload{}, err
	}
	to, err := evm.ParseAddress(req.To)
	if err != nil {
		return EVMPayload{}, fmt.Errorf("deposit address: %w", err)
	}
	amount, err := parsePositiveDecimal(req.Amount, 256)
	if err != nil {
		return EVMPayload{}, err
	}
	if req.Memo != "" {
		return EVMPayload{}, errors.New("wallets: EVM deposits cannot carry a memo")
	}
	payload := EVMPayload{
		To:      to,
		Value:   amount.String(),
		Data:    "0x",
		ChainID: strconv.FormatUint(chainID, 10),
	}
	if req.Token == "" {
		return payload, nil
	}
	token, err := evm.ParseAddress(req.Token)
	if err != nil {
		return EVMPayload{}, fmt.Errorf("token address: %w", err)
	}
	data, err := evm.ERC20TransferDataHex(to, amount)
	if err != nil {
		return EVMPayload{}, err
	}
	payload.To = token
	payload.Value = "0"
	payload.Data = data
	return payload, nil
}
