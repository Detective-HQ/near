package solana

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
	"universal-defi-intelligence/internal/encoding/base58"
)

func TestParseAddress(t *testing.T) {
	t.Parallel()
	valid := []string{
		SystemProgramID,
		TokenProgramID,
		"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		"7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU",
		base58.Encode(make([]byte, PublicKeyLength)),
	}
	for _, address := range valid {
		got, err := ParseAddress(address)
		if err != nil {
			t.Fatalf("ParseAddress(%q): %v", address, err)
		}
		if got != address {
			t.Fatalf("ParseAddress(%q) = %q", address, got)
		}
	}
}

func TestParseAddressRejectsInvalid(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"",
		"0OIl",
		"111",
		strings.Repeat("1", 33),
		"not a key",
		strings.Repeat("1", 45),
	} {
		if _, err := ParseAddress(value); !errors.Is(err, ErrInvalidAddress) {
			t.Fatalf("ParseAddress(%q) error = %v, want ErrInvalidAddress", value, err)
		}
	}
	for _, size := range []int{0, 31, 33} {
		if _, err := EncodeAddress(make([]byte, size)); !errors.Is(err, ErrInvalidAddress) {
			t.Fatalf("EncodeAddress(%d bytes) error = %v", size, err)
		}
	}
}

func TestDeriveAssociatedTokenAddress(t *testing.T) {
	t.Parallel()
	vectors := []struct {
		owner string
		mint  string
		want  string
	}{
		{
			owner: "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU",
			mint:  "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			want:  "C4PRXFV6Gf5mytVZb6RoeLsG8CjcFWzR2EJ3dvwPTUJH",
		},
		{
			owner: "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
			mint:  "So11111111111111111111111111111111111111112",
			want:  "Gvdu5vziankrzZkxPecKtxpH881iRMVKAsNqocJYyEtF",
		},
		{
			owner: SystemProgramID,
			mint:  "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			want:  "HJt8Tjdsc9ms9i4WCZEzhzr4oyf3ANcdzXrNdLPFqm3M",
		},
	}
	for _, vector := range vectors {
		got, err := DeriveAssociatedTokenAddress(vector.owner, vector.mint)
		if err != nil {
			t.Fatalf("DeriveAssociatedTokenAddress(%q, %q): %v", vector.owner, vector.mint, err)
		}
		if got != vector.want {
			t.Fatalf("DeriveAssociatedTokenAddress(%q, %q) = %q, want %q", vector.owner, vector.mint, got, vector.want)
		}
	}
	if _, err := DeriveAssociatedTokenAddress("bad", vectors[0].mint); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("DeriveAssociatedTokenAddress(bad owner) error = %v", err)
	}
}

func TestTokenAccountResolverPrefersATA(t *testing.T) {
	t.Parallel()
	owner := "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"
	mint := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	ata := "C4PRXFV6Gf5mytVZb6RoeLsG8CjcFWzR2EJ3dvwPTUJH"
	other := SystemProgramID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "getTokenAccountsByOwner" {
			t.Fatalf("method = %q", req.Method)
		}
		if len(req.Params) != 3 {
			t.Fatalf("params = %d", len(req.Params))
		}
		var gotOwner string
		_ = json.Unmarshal(req.Params[0], &gotOwner)
		if gotOwner != owner {
			t.Fatalf("owner param = %q", gotOwner)
		}
		var opts map[string]string
		_ = json.Unmarshal(req.Params[1], &opts)
		if opts["mint"] != mint {
			t.Fatalf("mint param = %q", opts["mint"])
		}
		_ = json.Unmarshal(req.Params[2], &opts)
		if opts["encoding"] != "jsonParsed" {
			t.Fatalf("encoding param = %q", opts["encoding"])
		}
		writeTokenAccounts(t, w, []string{other, ata}, mint, owner)
	}))
	defer server.Close()

	resolver := &TokenAccountResolver{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
	account, err := resolver.Resolve(context.Background(), owner, mint)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	if account.Address != ata || account.Mint != mint || account.Owner != owner || account.Amount != 12345 {
		t.Fatalf("Resolve() = %+v", account)
	}
}

func TestTokenAccountResolverErrors(t *testing.T) {
	t.Parallel()
	owner := "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"
	mint := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeTokenAccounts(t, w, nil, mint, owner)
		}))
		defer server.Close()
		resolver := &TokenAccountResolver{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		if _, err := resolver.Resolve(context.Background(), owner, mint); !errors.Is(err, ErrTokenAccountNotFound) {
			t.Fatalf("Resolve() error = %v", err)
		}
	})

	t.Run("bad amount", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"value":[{"pubkey":"` + SystemProgramID + `","account":{"data":{"parsed":{"info":{"mint":"` + mint + `","owner":"` + owner + `","tokenAmount":{"amount":"oops"}}}}}}]}}`))
		}))
		defer server.Close()
		resolver := &TokenAccountResolver{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		if _, err := resolver.Resolve(context.Background(), owner, mint); err == nil || !strings.Contains(err.Error(), "invalid token amount") {
			t.Fatalf("Resolve() error = %v", err)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad params"}}`))
		}))
		defer server.Close()
		resolver := &TokenAccountResolver{RPC: &jsonrpc.Client{URLs: []string{server.URL}}}
		var rpcErr *jsonrpc.Error
		if _, err := resolver.Resolve(context.Background(), owner, mint); !errors.As(err, &rpcErr) {
			t.Fatalf("Resolve() error = %v", err)
		}
	})

	if _, err := (&TokenAccountResolver{}).Resolve(context.Background(), owner, mint); err == nil {
		t.Fatal("Resolve() with nil RPC succeeded")
	}
}

func writeTokenAccounts(t *testing.T, w http.ResponseWriter, addresses []string, mint, owner string) {
	t.Helper()
	value := make([]map[string]any, 0, len(addresses))
	for _, address := range addresses {
		value = append(value, map[string]any{
			"pubkey": address,
			"account": map[string]any{
				"data": map[string]any{
					"parsed": map[string]any{
						"info": map[string]any{
							"mint":        mint,
							"owner":       owner,
							"tokenAmount": map[string]string{"amount": "12345"},
						},
					},
				},
			},
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"result":  map[string]any{"value": value},
	})
}

func FuzzParseAddress(f *testing.F) {
	f.Add("11111111111111111111111111111111")
	f.Add("0OIl")
	f.Fuzz(func(t *testing.T, value string) {
		normalized, err := ParseAddress(value)
		if err != nil {
			return
		}
		again, err := ParseAddress(normalized)
		if err != nil || again != normalized {
			t.Fatalf("reparse(%q) = %q, %v", normalized, again, err)
		}
	})
}
