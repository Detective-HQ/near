package solana

import (
	"context"
	"fmt"
	"strconv"

	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
)

// TokenAccount is an on-chain SPL token account owned by a wallet.
type TokenAccount struct {
	Address string
	Mint    string
	Owner   string
	Amount  uint64
}

// TokenAccountResolver queries getTokenAccountsByOwner with jsonParsed
// encoding. When several accounts exist it prefers the canonical ATA.
type TokenAccountResolver struct {
	RPC *jsonrpc.Client
}

type tokenAccountsResult struct {
	Value []struct {
		Pubkey  string `json:"pubkey"`
		Account struct {
			Data struct {
				Parsed struct {
					Info struct {
						Mint        string `json:"mint"`
						Owner       string `json:"owner"`
						TokenAmount struct {
							Amount string `json:"amount"`
						} `json:"tokenAmount"`
					} `json:"info"`
				} `json:"parsed"`
			} `json:"data"`
		} `json:"account"`
	} `json:"value"`
}

// Resolve returns the owner's token account for mint, preferring the associated
// token address when multiple accounts exist.
func (r *TokenAccountResolver) Resolve(ctx context.Context, owner, mint string) (TokenAccount, error) {
	if r == nil || r.RPC == nil {
		return TokenAccount{}, fmt.Errorf("solana: token account resolver is not configured")
	}
	owner, err := ParseAddress(owner)
	if err != nil {
		return TokenAccount{}, fmt.Errorf("owner: %w", err)
	}
	mint, err = ParseAddress(mint)
	if err != nil {
		return TokenAccount{}, fmt.Errorf("mint: %w", err)
	}
	expected, err := DeriveAssociatedTokenAddress(owner, mint)
	if err != nil {
		return TokenAccount{}, err
	}
	var result tokenAccountsResult
	params := []any{
		owner,
		map[string]string{"mint": mint},
		map[string]string{"encoding": "jsonParsed"},
	}
	if err := r.RPC.Call(ctx, "getTokenAccountsByOwner", params, &result); err != nil {
		return TokenAccount{}, err
	}

	var fallback *TokenAccount
	for _, item := range result.Value {
		address, err := ParseAddress(item.Pubkey)
		if err != nil {
			return TokenAccount{}, fmt.Errorf("RPC returned invalid token account: %w", err)
		}
		info := item.Account.Data.Parsed.Info
		accountMint, err := ParseAddress(info.Mint)
		if err != nil || accountMint != mint {
			return TokenAccount{}, fmt.Errorf("RPC returned token account for unexpected mint %q", info.Mint)
		}
		accountOwner, err := ParseAddress(info.Owner)
		if err != nil || accountOwner != owner {
			return TokenAccount{}, fmt.Errorf("RPC returned token account for unexpected owner %q", info.Owner)
		}
		amount, err := strconv.ParseUint(info.TokenAmount.Amount, 10, 64)
		if err != nil {
			return TokenAccount{}, fmt.Errorf("RPC returned invalid token amount %q", info.TokenAmount.Amount)
		}
		account := TokenAccount{Address: address, Mint: accountMint, Owner: accountOwner, Amount: amount}
		if address == expected {
			return account, nil
		}
		if fallback == nil {
			copy := account
			fallback = &copy
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return TokenAccount{}, fmt.Errorf("%w: owner %s mint %s", ErrTokenAccountNotFound, owner, mint)
}
