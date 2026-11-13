package wallets

import (
	"fmt"

	"universal-defi-intelligence/internal/chains/aptos"
)

// AptosTransferRequest describes a native APT or coin deposit.
type AptosTransferRequest struct {
	Recipient string
	Amount    string
	CoinType  string
}

// AptosPayload is an unsigned Aptos entry-function payload.
type AptosPayload struct {
	Type          string   `json:"type"`
	Function      string   `json:"function"`
	TypeArguments []string `json:"type_arguments"`
	Arguments     []string `json:"arguments"`
}

// BuildAptosTransfer returns an entry-function payload for the browser wallet.
func BuildAptosTransfer(req AptosTransferRequest) (AptosPayload, error) {
	recipient, err := aptos.ParseAddress(req.Recipient)
	if err != nil {
		return AptosPayload{}, fmt.Errorf("recipient: %w", err)
	}
	amount, err := parsePositiveDecimal(req.Amount, 64)
	if err != nil {
		return AptosPayload{}, err
	}
	payload := AptosPayload{
		Type:          "entry_function_payload",
		Function:      "0x1::aptos_account::transfer",
		TypeArguments: []string{},
		Arguments:     []string{recipient, amount.String()},
	}
	if req.CoinType == "" {
		return payload, nil
	}
	parsed, err := aptos.ParseType(req.CoinType)
	if err != nil {
		return AptosPayload{}, fmt.Errorf("coin type: %w", err)
	}
	if parsed.Module == "" {
		return AptosPayload{}, fmt.Errorf("coin type: must be a struct type")
	}
	payload.Function = "0x1::coin::transfer"
	payload.TypeArguments = []string{parsed.String()}
	return payload, nil
}
