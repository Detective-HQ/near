package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"universal-defi-intelligence/internal/chains/near"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
)

// NEARAccessKeyVerifier checks that a public key is a full-access key of a
// NEAR account by calling the view_access_key RPC query.
type NEARAccessKeyVerifier struct {
	RPC *jsonrpc.Client
}

type nearAccessKeyView struct {
	Permission json.RawMessage `json:"permission"`
}

// VerifyAccessKey returns nil only when publicKeyBase58 is a FullAccess key of
// accountID. Function-call keys are rejected: they are commonly held by dApps
// and must not be able to sign in as the account owner.
func (v *NEARAccessKeyVerifier) VerifyAccessKey(ctx context.Context, accountID, publicKeyBase58 string) error {
	if v == nil || v.RPC == nil {
		return fmt.Errorf("%w: NEAR RPC is not configured", ErrInvalidRequest)
	}
	account, err := near.ParseAccountID(accountID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	var view nearAccessKeyView
	params := map[string]string{
		"request_type": "view_access_key",
		"finality":     "final",
		"account_id":   account,
		"public_key":   "ed25519:" + publicKeyBase58,
	}
	if err := v.RPC.Call(ctx, "query", params, &view); err != nil {
		return fmt.Errorf("view_access_key: %w", err)
	}
	var permission string
	if err := json.Unmarshal(view.Permission, &permission); err != nil || permission != "FullAccess" {
		return errors.New("access key is not a full-access key")
	}
	return nil
}
