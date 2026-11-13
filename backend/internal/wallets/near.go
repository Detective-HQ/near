package wallets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"universal-defi-intelligence/internal/chains/near"
)

// MaxMemoBytes bounds provider memos embedded in wallet payloads.
const MaxMemoBytes = 512

// NEARTransferRequest describes a native NEAR or NEP-141 deposit.
type NEARTransferRequest struct {
	DepositAddress string
	Amount         string
	TokenContract  string
	Memo           string
}

// NEARAction mirrors the NEAR wallet-selector action shape.
type NEARAction struct {
	Type   string            `json:"type"`
	Params map[string]string `json:"params"`
}

// NEARPayload is an unsigned NEAR transaction action list.
type NEARPayload struct {
	ReceiverID string       `json:"receiverId"`
	Actions    []NEARAction `json:"actions"`
}

// BuildNEARTransfer returns native Transfer or NEP-141 ft_transfer actions.
func BuildNEARTransfer(req NEARTransferRequest) (NEARPayload, error) {
	deposit, err := near.ParseAccountID(req.DepositAddress)
	if err != nil {
		return NEARPayload{}, fmt.Errorf("deposit address: %w", err)
	}
	amount, err := parsePositiveDecimal(req.Amount, 128)
	if err != nil {
		return NEARPayload{}, err
	}
	if len(req.Memo) > MaxMemoBytes {
		return NEARPayload{}, fmt.Errorf("wallets: memo exceeds %d bytes", MaxMemoBytes)
	}
	if req.TokenContract == "" {
		if req.Memo != "" {
			return NEARPayload{}, errors.New("wallets: native NEAR transfers cannot carry a memo")
		}
		return NEARPayload{
			ReceiverID: deposit,
			Actions: []NEARAction{{
				Type:   "Transfer",
				Params: map[string]string{"deposit": amount.String()},
			}},
		}, nil
	}

	contract, err := near.ParseAccountID(req.TokenContract)
	if err != nil {
		return NEARPayload{}, fmt.Errorf("token contract: %w", err)
	}
	args := map[string]string{"receiver_id": deposit, "amount": amount.String()}
	if req.Memo != "" {
		args["memo"] = req.Memo
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return NEARPayload{}, err
	}
	return NEARPayload{
		ReceiverID: contract,
		Actions: []NEARAction{{
			Type: "FunctionCall",
			Params: map[string]string{
				"methodName": "ft_transfer",
				"args":       base64.StdEncoding.EncodeToString(encoded),
				"gas":        "30000000000000",
				"deposit":    "1",
			},
		}},
	}, nil
}
