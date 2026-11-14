package portfolio

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
)

// Multicall3Address is the canonical Multicall3 deployment address.
const Multicall3Address = "0xcA11bde05977b3631167028862bE2a173976CA11"

// Multicall3Aggregate3Selector is keccak256("aggregate3((address,bool,bytes)[])")[:4]
var Multicall3Aggregate3Selector = [4]byte{0x82, 0xad, 0x56, 0xcb}

// ERC20BalanceOfSelector is keccak256("balanceOf(address)")[:4]
var ERC20BalanceOfSelector = [4]byte{0x70, 0xa0, 0x82, 0x31}

// encodeMulticall3BalanceOf encodes aggregate3(Call3[]) for balanceOf(owner) calls.
func encodeMulticall3BalanceOf(tokens []TokenAsset, ownerAddress string) (string, error) {
	ownerBytes, err := evm.DecodeAddress(ownerAddress)
	if err != nil {
		return "", fmt.Errorf("invalid owner address: %w", err)
	}

	n := len(tokens)
	if n == 0 {
		return "", fmt.Errorf("no tokens to query")
	}

	// 4 bytes selector + 32 bytes array offset + 32 bytes array len + n*32 bytes head offsets + n*192 bytes body
	totalBytes := 4 + 32 + 32 + n*32 + n*192
	buf := make([]byte, totalBytes)

	// Selector
	copy(buf[:4], Multicall3Aggregate3Selector[:])

	// Param 0 offset: 32 (0x20)
	buf[35] = 0x20

	// Array len: n
	new(big.Int).SetInt64(int64(n)).FillBytes(buf[36:68])

	// Array offsets: each Call3 struct starts at headLen + i*structLen (structLen = 192)
	headLen := n * 32
	structLen := 192
	for i := 0; i < n; i++ {
		offset := headLen + i*structLen
		new(big.Int).SetInt64(int64(offset)).FillBytes(buf[68+i*32 : 68+(i+1)*32])
	}

	// Call3 structs
	bodyStart := 68 + headLen
	for i, t := range tokens {
		pos := bodyStart + i*structLen
		tokenBytes, err := evm.DecodeAddress(t.ContractAddress)
		if err != nil {
			tokenBytes = make([]byte, 20)
		}

		// 1. target address (20 bytes right-aligned in 32 bytes)
		copy(buf[pos+12:pos+32], tokenBytes)

		// 2. allowFailure: bool (true = 1)
		buf[pos+63] = 0x01

		// 3. offset to callData: 0x60 (96 bytes from pos)
		buf[pos+95] = 0x60

		// 4. callData length: 0x24 (36 bytes: 4 bytes selector + 32 bytes address)
		buf[pos+127] = 0x24

		// 5. callData (36 bytes + 28 bytes padding = 64 bytes)
		copy(buf[pos+128:pos+132], ERC20BalanceOfSelector[:])
		copy(buf[pos+132+12:pos+164], ownerBytes)
	}

	return "0x" + hex.EncodeToString(buf), nil
}

// decodeMulticall3Results decodes aggregate3 return bytes (Result[]) into big.Int balances.
func decodeMulticall3Results(rawHex string, n int) ([]*big.Int, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(rawHex), "0x"))
	if err != nil {
		return nil, err
	}
	if len(raw) < 64 {
		return nil, fmt.Errorf("response too short: %d bytes", len(raw))
	}

	// offset 0..32: offset to Result[] array
	// offset 32..64: array length
	arrLen := int(new(big.Int).SetBytes(raw[32:64]).Int64())
	if arrLen <= 0 || arrLen > 1000 {
		return nil, fmt.Errorf("invalid multicall result length: %d", arrLen)
	}
	if arrLen > n {
		arrLen = n
	}

	results := make([]*big.Int, arrLen)
	if len(raw) < 64+arrLen*32 {
		return nil, fmt.Errorf("response truncated in head table")
	}

	for i := 0; i < arrLen; i++ {
		offset := int(new(big.Int).SetBytes(raw[64+i*32 : 64+(i+1)*32]).Int64())
		structPos := 32 + offset // offsets in Solidity are relative to the array's data start (byte 32)
		if structPos+64 > len(raw) {
			continue
		}
		success := raw[structPos+31] == 1
		if !success {
			continue
		}
		dataOffset := int(new(big.Int).SetBytes(raw[structPos+32 : structPos+64]).Int64())
		dataPos := structPos + dataOffset
		if dataPos+32 > len(raw) {
			continue
		}
		dataLen := int(new(big.Int).SetBytes(raw[dataPos : dataPos+32]).Int64())
		if dataLen < 32 || dataPos+32+32 > len(raw) {
			continue
		}
		bal := new(big.Int).SetBytes(raw[dataPos+32 : dataPos+64])
		results[i] = bal
	}

	return results, nil
}

// readEVMTokens queries ERC-20 balances for contracts using Multicall3.
func (r *Reader) readEVMTokens(ctx context.Context, client *jsonrpc.Client, chain registry.Chain, address string, tokens []TokenAsset) []Holding {
	if len(tokens) == 0 {
		return nil
	}
	if len(tokens) > 200 {
		tokens = tokens[:200]
	}
	calldata, err := encodeMulticall3BalanceOf(tokens, address)
	if err != nil {
		return nil
	}
	var hexResult string
	err = client.Call(ctx, "eth_call", []any{
		map[string]string{
			"to":   Multicall3Address,
			"data": calldata,
		},
		"latest",
	}, &hexResult)
	if err != nil {
		return nil
	}
	bals, err := decodeMulticall3Results(hexResult, len(tokens))
	if err != nil {
		return nil
	}
	source := chain.Name + " Multicall3"
	var out []Holding
	for i, bal := range bals {
		if bal == nil || bal.Sign() <= 0 {
			continue
		}
		t := tokens[i]
		priceKey := t.PriceKey
		if priceKey == "" {
			priceKey = chain.Key + ":" + strings.ToLower(t.ContractAddress)
		}
		out = append(out, r.holding(chain, t.Symbol, t.ID, bal, t.Decimals, priceKey, source))
	}
	return out
}

// readSolanaSPLTokens queries SPL token accounts for a Solana owner.
func (r *Reader) readSolanaSPLTokens(ctx context.Context, client *jsonrpc.Client, chain registry.Chain, address string, catalogue []TokenAsset) []Holding {
	type splResponse struct {
		Value []struct {
			Account struct {
				Data struct {
					Parsed struct {
						Info struct {
							Mint        string `json:"mint"`
							TokenAmount struct {
								Amount   string `json:"amount"`
								Decimals int    `json:"decimals"`
							} `json:"tokenAmount"`
						} `json:"info"`
					} `json:"parsed"`
				} `json:"data"`
			} `json:"account"`
		} `json:"value"`
	}

	var res splResponse
	err := client.Call(ctx, "getTokenAccountsByOwner", []any{
		address,
		map[string]string{"programId": "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"},
		map[string]string{"encoding": "jsonParsed"},
	}, &res)
	if err != nil {
		return nil
	}

	byMint := make(map[string]TokenAsset, len(catalogue))
	for _, t := range catalogue {
		byMint[t.ContractAddress] = t
	}

	source := chain.Name + " RPC"
	var out []Holding
	for _, item := range res.Value {
		info := item.Account.Data.Parsed.Info
		amount, ok := new(big.Int).SetString(info.TokenAmount.Amount, 10)
		if !ok || amount.Sign() <= 0 {
			continue
		}
		cat, inCat := byMint[info.Mint]
		symbol := info.Mint
		if len(symbol) > 8 {
			symbol = symbol[:4] + "..." + symbol[len(symbol)-4:]
		}
		decimals := info.TokenAmount.Decimals
		assetID := "solana:" + info.Mint
		priceKey := "solana:" + info.Mint
		if inCat {
			symbol = cat.Symbol
			decimals = cat.Decimals
			assetID = cat.ID
			if cat.PriceKey != "" {
				priceKey = cat.PriceKey
			}
		}
		out = append(out, r.holding(chain, symbol, assetID, amount, decimals, priceKey, source))
	}
	return out
}
