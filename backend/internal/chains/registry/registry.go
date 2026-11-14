// Package registry loads the supported-chain allowlist from data rather than
// hard-coding chain capabilities in application logic. RPC endpoints and
// explorer links are configuration, never secrets.
package registry

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

// Family identifies the address and transaction format used by a chain.
type Family string

const (
	FamilyEVM     Family = "evm"
	FamilySolana  Family = "solana"
	FamilyStellar Family = "stellar"
	FamilyMove    Family = "move"
	FamilyNEAR    Family = "near"
)

// Valid reports whether the family is known.
func (f Family) Valid() bool {
	switch f {
	case FamilyEVM, FamilySolana, FamilyStellar, FamilyMove, FamilyNEAR:
		return true
	default:
		return false
	}
}

// Features is the route capability matrix for a chain.
type Features struct {
	SwapEnabled      bool `yaml:"swap_enabled"`
	DepositSupported bool `yaml:"deposit_supported"`
	MemoRequired     bool `yaml:"memo_required"`
	WalletOnly       bool `yaml:"wallet_only"`
	Planned          bool `yaml:"planned"`
}

// NativeCurrency describes the chain's fee/transfer asset.
type NativeCurrency struct {
	Symbol   string `yaml:"symbol"`
	Decimals int    `yaml:"decimals"`
}

// Chain is one entry in the registry.
type Chain struct {
	Key                string         `yaml:"key"`
	Aliases            []string       `yaml:"aliases"`
	Name               string         `yaml:"name"`
	Family             Family         `yaml:"family"`
	ChainID            string         `yaml:"chain_id"`
	RPCURLs            []string       `yaml:"rpc_urls"`
	ExplorerTxURL      string         `yaml:"explorer_tx_url"`
	ExplorerAddressURL string         `yaml:"explorer_address_url"`
	NativeCurrency     NativeCurrency `yaml:"native_currency"`
	Features           Features       `yaml:"features"`
}

// Registry is an immutable, validated set of chains.
type Registry struct {
	chains []Chain
	byKey  map[string]int
}

//go:embed chains.yaml
var embeddedYAML []byte

var (
	defaultOnce sync.Once
	defaultReg  *Registry
	defaultErr  error
)

// Load parses the embedded chains.yaml. The result is cached.
func Load() (*Registry, error) {
	defaultOnce.Do(func() {
		defaultReg, defaultErr = Parse(embeddedYAML)
	})
	return defaultReg, defaultErr
}

type document struct {
	Chains []Chain `yaml:"chains"`
}

// Parse validates and indexes a registry document.
func Parse(data []byte) (*Registry, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var doc document
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse chain registry: %w", err)
	}
	if len(doc.Chains) == 0 {
		return nil, errors.New("chain registry: no chains configured")
	}

	registry := &Registry{
		chains: make([]Chain, 0, len(doc.Chains)),
		byKey:  make(map[string]int, len(doc.Chains)),
	}
	chainIDs := make(map[string]string, len(doc.Chains))
	for i, chain := range doc.Chains {
		chain.Key = strings.TrimSpace(chain.Key)
		if err := validateChain(chain); err != nil {
			return nil, fmt.Errorf("chain registry: entry %d: %w", i+1, err)
		}
		if _, exists := registry.byKey[chain.Key]; exists {
			return nil, fmt.Errorf("chain registry: duplicate key %q", chain.Key)
		}
		if chain.Family == FamilyEVM {
			if other, exists := chainIDs[chain.ChainID]; exists {
				return nil, fmt.Errorf("chain registry: chain ID %s used by both %q and %q", chain.ChainID, other, chain.Key)
			}
			chainIDs[chain.ChainID] = chain.Key
		}
		chain.RPCURLs = append([]string(nil), chain.RPCURLs...)
		chain.Aliases = append([]string(nil), chain.Aliases...)
		for _, alias := range chain.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if !keyPattern.MatchString(alias) {
				return nil, fmt.Errorf("chain registry: chain %q: invalid alias %q", chain.Key, alias)
			}
			if _, exists := registry.byKey[alias]; exists {
				return nil, fmt.Errorf("chain registry: duplicate key or alias %q", alias)
			}
			registry.byKey[alias] = len(registry.chains)
		}
		registry.byKey[chain.Key] = len(registry.chains)
		registry.chains = append(registry.chains, chain)
	}
	return registry, nil
}

var (
	keyPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	symbolPattern = regexp.MustCompile(`^[A-Z0-9]{2,10}$`)
)

func validateChain(chain Chain) error {
	if !keyPattern.MatchString(chain.Key) {
		return fmt.Errorf("invalid key %q", chain.Key)
	}
	if strings.TrimSpace(chain.Name) == "" || len(chain.Name) > 64 {
		return fmt.Errorf("chain %q: name must be 1-64 characters", chain.Key)
	}
	if !chain.Family.Valid() {
		return fmt.Errorf("chain %q: unknown family %q", chain.Key, chain.Family)
	}
	if chain.Family == FamilyEVM {
		id, err := strconv.ParseUint(chain.ChainID, 10, 64)
		if err != nil || id == 0 {
			return fmt.Errorf("chain %q: chain_id must be a positive decimal integer", chain.Key)
		}
		if strconv.FormatUint(id, 10) != chain.ChainID {
			return fmt.Errorf("chain %q: chain_id must be canonical decimal", chain.Key)
		}
	} else if chain.ChainID != "" {
		return fmt.Errorf("chain %q: chain_id is only valid for EVM chains", chain.Key)
	}

	if len(chain.RPCURLs) == 0 {
		return fmt.Errorf("chain %q: at least one RPC URL is required", chain.Key)
	}
	seen := make(map[string]struct{}, len(chain.RPCURLs))
	for _, raw := range chain.RPCURLs {
		if err := validateRPCURL(raw); err != nil {
			return fmt.Errorf("chain %q: %w", chain.Key, err)
		}
		if _, duplicate := seen[raw]; duplicate {
			return fmt.Errorf("chain %q: duplicate RPC URL %q", chain.Key, raw)
		}
		seen[raw] = struct{}{}
	}
	for _, template := range []struct {
		name  string
		value string
		token string
	}{
		{name: "explorer_tx_url", value: chain.ExplorerTxURL, token: "{hash}"},
		{name: "explorer_address_url", value: chain.ExplorerAddressURL, token: "{address}"},
	} {
		if template.value == "" {
			continue
		}
		if !strings.Contains(template.value, template.token) {
			return fmt.Errorf("chain %q: %s must contain %s", chain.Key, template.name, template.token)
		}
		example := strings.ReplaceAll(template.value, template.token, "0")
		if err := validateExplorerURL(example); err != nil {
			return fmt.Errorf("chain %q: %s: %w", chain.Key, template.name, err)
		}
	}
	if !symbolPattern.MatchString(chain.NativeCurrency.Symbol) {
		return fmt.Errorf("chain %q: invalid native symbol %q", chain.Key, chain.NativeCurrency.Symbol)
	}
	if chain.NativeCurrency.Decimals < 0 || chain.NativeCurrency.Decimals > 36 {
		return fmt.Errorf("chain %q: native decimals must be 0-36", chain.Key)
	}

	features := chain.Features
	if features.Planned && features.SwapEnabled {
		return fmt.Errorf("chain %q: planned chains cannot have swap_enabled", chain.Key)
	}
	if features.WalletOnly && features.SwapEnabled {
		return fmt.Errorf("chain %q: wallet_only and swap_enabled are mutually exclusive", chain.Key)
	}
	return nil
}

func validateRPCURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid RPC URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("RPC URL %q must use https", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("RPC URL %q has no host", raw)
	}
	if parsed.User != nil {
		return fmt.Errorf("RPC URL %q must not contain credentials", raw)
	}
	return nil
}

func validateExplorerURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("URL %q must be an absolute https URL", raw)
	}
	return nil
}

// Get returns a chain by key.
func (r *Registry) Get(key string) (Chain, bool) {
	if r == nil {
		return Chain{}, false
	}
	index, ok := r.byKey[key]
	if !ok {
		index, ok = r.byKey[strings.ToLower(strings.TrimSpace(key))]
		if !ok {
			return Chain{}, false
		}
	}
	return cloneChain(r.chains[index]), true
}

// Canonical resolves a chain key or a provider alias (for example the 1Click
// blockchain name "eth") to the registry's canonical key.
func (r *Registry) Canonical(key string) (string, bool) {
	chain, ok := r.Get(key)
	if !ok {
		return "", false
	}
	return chain.Key, true
}

// ByChainID returns an EVM chain by its decimal chain ID.
func (r *Registry) ByChainID(chainID string) (Chain, bool) {
	if r == nil {
		return Chain{}, false
	}
	for _, chain := range r.chains {
		if chain.Family == FamilyEVM && chain.ChainID == chainID {
			return cloneChain(chain), true
		}
	}
	return Chain{}, false
}

// All returns every chain in registry order.
func (r *Registry) All() []Chain {
	if r == nil {
		return nil
	}
	out := make([]Chain, len(r.chains))
	for i, chain := range r.chains {
		out[i] = cloneChain(chain)
	}
	return out
}

// ByFamily returns chains for a family in registry order.
func (r *Registry) ByFamily(family Family) []Chain {
	if r == nil {
		return nil
	}
	var out []Chain
	for _, chain := range r.chains {
		if chain.Family == family {
			out = append(out, cloneChain(chain))
		}
	}
	return out
}

// SwapEnabled returns chains that may be routed for executable swaps.
func (r *Registry) SwapEnabled() []Chain {
	return r.filter(func(chain Chain) bool { return chain.Features.SwapEnabled })
}

// WalletOnly returns non-custodial chains that are not server-executable.
func (r *Registry) WalletOnly() []Chain {
	return r.filter(func(chain Chain) bool { return chain.Features.WalletOnly })
}

func (r *Registry) filter(match func(Chain) bool) []Chain {
	if r == nil {
		return nil
	}
	var out []Chain
	for _, chain := range r.chains {
		if match(chain) {
			out = append(out, cloneChain(chain))
		}
	}
	return out
}

// ExplorerTx returns the explorer URL for a transaction hash.
func (r *Registry) ExplorerTx(key, hash string) (string, bool) {
	chain, ok := r.Get(key)
	if !ok || chain.ExplorerTxURL == "" {
		return "", false
	}
	return strings.ReplaceAll(chain.ExplorerTxURL, "{hash}", hash), true
}

// ExplorerAddress returns the explorer URL for an address.
func (r *Registry) ExplorerAddress(key, address string) (string, bool) {
	chain, ok := r.Get(key)
	if !ok || chain.ExplorerAddressURL == "" {
		return "", false
	}
	return strings.ReplaceAll(chain.ExplorerAddressURL, "{address}", address), true
}

func cloneChain(chain Chain) Chain {
	chain.RPCURLs = append([]string(nil), chain.RPCURLs...)
	chain.Aliases = append([]string(nil), chain.Aliases...)
	return chain
}
