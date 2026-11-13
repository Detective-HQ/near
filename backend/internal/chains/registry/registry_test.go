package registry

import (
	"strings"
	"testing"
)

const validChain = `
chains:
  - key: ethereum
    name: Ethereum
    family: evm
    chain_id: "1"
    rpc_urls:
      - https://rpc.example.com
    explorer_tx_url: https://explorer.example.com/tx/{hash}
    explorer_address_url: https://explorer.example.com/address/{address}
    native_currency:
      symbol: ETH
      decimals: 18
    features:
      swap_enabled: true
      deposit_supported: true
`

func TestLoadEmbeddedRegistry(t *testing.T) {
	t.Parallel()
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	chains := reg.All()
	if len(chains) != 11 {
		t.Fatalf("chain count = %d, want 11", len(chains))
	}
	seen := make(map[string]struct{}, len(chains))
	for _, chain := range chains {
		if _, duplicate := seen[chain.Key]; duplicate {
			t.Fatalf("duplicate chain %q", chain.Key)
		}
		seen[chain.Key] = struct{}{}
		if _, ok := reg.Get(chain.Key); !ok {
			t.Fatalf("Get(%q) failed", chain.Key)
		}
	}
	if got := len(reg.SwapEnabled()); got != 6 {
		t.Fatalf("swap-enabled chains = %d, want 6", got)
	}
	if got := len(reg.WalletOnly()); got != 4 {
		t.Fatalf("wallet-only chains = %d, want 4", got)
	}
	ethereum, _ := reg.Get("ethereum")
	if ethereum.NativeCurrency.Symbol != "ETH" || ethereum.ChainID != "1" {
		t.Fatalf("unexpected ethereum entry: %+v", ethereum)
	}
	stellar, _ := reg.Get("stellar")
	if !stellar.Features.MemoRequired {
		t.Fatal("stellar must require a memo")
	}
	if _, ok := reg.ByChainID("8453"); !ok {
		t.Fatal("ByChainID(8453) failed")
	}
}

func TestExplorerTemplates(t *testing.T) {
	t.Parallel()
	reg, err := Parse([]byte(validChain))
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	txURL, ok := reg.ExplorerTx("ethereum", "0xabc")
	if !ok || txURL != "https://explorer.example.com/tx/0xabc" {
		t.Fatalf("ExplorerTx() = %q, %v", txURL, ok)
	}
	addressURL, ok := reg.ExplorerAddress("ethereum", "0x123")
	if !ok || addressURL != "https://explorer.example.com/address/0x123" {
		t.Fatalf("ExplorerAddress() = %q, %v", addressURL, ok)
	}
}

func TestParseRejectsInvalidRegistries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"empty", "chains: []", "no chains"},
		{"unknown field", validChain + "    unknown: true\n", "unknown"},
		{"bad key", strings.Replace(validChain, "key: ethereum", "key: Ethereum", 1), "invalid key"},
		{"bad family", strings.Replace(validChain, "family: evm", "family: cosmos", 1), "unknown family"},
		{"missing chain id", strings.Replace(validChain, `chain_id: "1"`, `chain_id: ""`, 1), "chain_id"},
		{"bad chain id", strings.Replace(validChain, `chain_id: "1"`, `chain_id: "01"`, 1), "canonical"},
		{"http rpc", strings.Replace(validChain, "https://rpc.example.com", "http://rpc.example.com", 1), "https"},
		{"credentials", strings.Replace(validChain, "https://rpc.example.com", "https://user:pass@rpc.example.com", 1), "credentials"},
		{"explorer token", strings.Replace(validChain, "/tx/{hash}", "/tx/", 1), "must contain"},
		{"bad symbol", strings.Replace(validChain, "symbol: ETH", "symbol: eth", 1), "native symbol"},
		{"bad decimals", strings.Replace(validChain, "decimals: 18", "decimals: 99", 1), "decimals"},
		{"planned swap", strings.Replace(validChain, "swap_enabled: true", "swap_enabled: true\n      planned: true", 1), "planned"},
		{"wallet swap", strings.Replace(validChain, "swap_enabled: true", "swap_enabled: true\n      wallet_only: true", 1), "wallet_only"},
		{"duplicate key", validChain + strings.TrimPrefix(validChain, "\nchains:"), "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(tc.doc)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsDuplicateChainID(t *testing.T) {
	t.Parallel()
	second := `
  - key: other
    name: Other
    family: evm
    chain_id: "1"
    rpc_urls:
      - https://other.example.com
    native_currency:
      symbol: ETH
      decimals: 18
    features:
      swap_enabled: true
`
	if _, err := Parse([]byte(validChain + second)); err == nil || !strings.Contains(err.Error(), "chain ID") {
		t.Fatalf("Parse() error = %v, want duplicate chain ID", err)
	}
}
