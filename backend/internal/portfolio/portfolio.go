// Package portfolio reads real on-chain balances for a wallet address.
//
// It is read-only and non-custodial: it only calls public RPC endpoints from
// the chain registry. Native balances are read on every chain family; USDC is
// read on EVM chains from contract addresses verified on-chain (symbol and
// decimals). Amounts are exact (big.Int); USD values come from DeFiLlama
// current prices only when a price source is configured, and are null
// otherwise. Nothing is estimated or seeded.
package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"universal-defi-intelligence/internal/chains/aptos"
	"universal-defi-intelligence/internal/chains/evm"
	"universal-defi-intelligence/internal/chains/near"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/chains/rpc"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
	"universal-defi-intelligence/internal/chains/solana"
	"universal-defi-intelligence/internal/chains/stellar"
	"universal-defi-intelligence/internal/swap/domain"
)

// ErrInvalidAddress means the address matches no supported chain format.
var ErrInvalidAddress = errors.New("address is not valid for any supported chain")

// ErrUnavailable means every balance read failed.
var ErrUnavailable = errors.New("balances are unavailable")

// Holding is one asset balance.
type Holding struct {
	AssetID  string
	Symbol   string
	Chain    string
	Atomic   *big.Int
	Decimals int
	PriceKey string
	ValueUSD *float64
	Source   string
	AsOf     time.Time
}

// Balance returns the exact decimal balance.
func (h Holding) Balance() string { return domain.FormatUnits(h.Atomic, h.Decimals) }

// Result is a portfolio read.
type Result struct {
	Address  string
	Chain    string
	Holdings []Holding
	Quality  string // live | partial | unavailable
	AsOf     time.Time
	Priced   bool
	Failed   []string // chains whose reads failed
}

// PriceSource returns current USD prices for DeFiLlama coin keys.
type PriceSource interface {
	Current(ctx context.Context, keys []string) (map[string]float64, error)
}

// TokenAsset describes a non-native token available in the catalogue.
type TokenAsset struct {
	ID              string
	Symbol          string
	Chain           string
	Decimals        int
	ContractAddress string
	PriceKey        string
}

// TokenCatalogue supplies non-native assets for token balance discovery.
type TokenCatalogue interface {
	ListTokens(ctx context.Context, chain string) ([]TokenAsset, error)
}

// token is a non-native asset read on a chain.
type token struct {
	symbol   string
	contract string
	decimals int
	priceKey string
}

// usdcByChain lists USDC contracts verified on-chain (symbol() == "USDC" and
// decimals()) on 2026-11-14.
var usdcByChain = map[string]token{
	"ethereum":  {"USDC", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", 6, "coingecko:usd-coin"},
	"base":      {"USDC", "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", 6, "coingecko:usd-coin"},
	"arbitrum":  {"USDC", "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", 6, "coingecko:usd-coin"},
	"polygon":   {"USDC", "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", 6, "coingecko:usd-coin"},
	"avalanche": {"USDC", "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E", 6, "coingecko:usd-coin"},
	"bnb":       {"USDC", "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", 18, "coingecko:usd-coin"},
	"celo":      {"USDC", "0xcebA9300f2b948710d2653dD7B07f33A8B32118C", 6, "coingecko:usd-coin"},
}

// nativePriceKeys maps a chain to the DeFiLlama key of its native asset.
var nativePriceKeys = map[string]string{
	"ethereum": "coingecko:ethereum", "base": "coingecko:ethereum", "arbitrum": "coingecko:ethereum",
	"polygon": "coingecko:polygon-ecosystem-token", "avalanche": "coingecko:avalanche-2",
	"bnb": "coingecko:binancecoin", "celo": "coingecko:celo", "solana": "coingecko:solana",
	"stellar": "coingecko:stellar", "aptos": "coingecko:aptos", "near": "coingecko:near",
}

// Config configures a Reader.
type Config struct {
	Registry   *registry.Registry
	HTTPClient *http.Client
	Prices     PriceSource // nil = no USD values
	Catalogue  TokenCatalogue
	Timeout    time.Duration
	CacheTTL   time.Duration
	Now        func() time.Time
	// Endpoints overrides registry RPC URLs per chain (tests, private nodes).
	Endpoints map[string][]string
}

// Reader reads and caches portfolios.
type Reader struct {
	cfg    Config
	mu     sync.Mutex
	cache  map[string]cached
	flight singleflight.Group
}

type cached struct {
	result Result
	at     time.Time
}

const maxCacheEntries = 10_000

// NewReader creates a Reader.
func NewReader(cfg Config) *Reader {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Reader{cfg: cfg, cache: map[string]cached{}}
}

// Chains returns the registry chains an address can belong to, optionally
// restricted to one chain. The normalized address for each chain is returned.
func (r *Reader) Chains(address, only string) (map[string]string, error) {
	address = strings.TrimSpace(address)
	out := map[string]string{}
	add := func(family registry.Family, normalized string) {
		for _, c := range r.cfg.Registry.ByFamily(family) {
			out[c.Key] = normalized
		}
	}
	if a, err := evm.ParseAddress(address); err == nil {
		add(registry.FamilyEVM, a)
	}
	if a, err := solana.ParseAddress(address); err == nil {
		add(registry.FamilySolana, a)
	}
	if a, err := stellar.ParseAccountID(address); err == nil {
		add(registry.FamilyStellar, a)
	}
	if len(strings.TrimPrefix(address, "0x")) > 40 { // 32-byte Move addresses, not EVM
		if a, err := aptos.ParseAddress(address); err == nil {
			add(registry.FamilyMove, a)
		}
	}
	// EVM addresses (0x + 40 hex without dot) are not NEAR accounts.
	if !strings.HasPrefix(strings.ToLower(address), "0x") || strings.Contains(address, ".") {
		if a, err := near.ParseAccountID(address); err == nil {
			add(registry.FamilyNEAR, a)
		}
	}
	if only != "" {
		key, ok := r.cfg.Registry.Canonical(only)
		if !ok {
			return nil, fmt.Errorf("%w: unknown chain %q", ErrInvalidAddress, only)
		}
		normalized, match := out[key]
		if !match {
			return nil, fmt.Errorf("%w on chain %q", ErrInvalidAddress, key)
		}
		return map[string]string{key: normalized}, nil
	}
	if len(out) == 0 {
		return nil, ErrInvalidAddress
	}
	return out, nil
}

// Read returns balances for address (optionally one chain), cached briefly.
func (r *Reader) Read(ctx context.Context, address, only string) (Result, error) {
	return r.ReadTokens(ctx, address, only, "catalogue")
}

// ReadTokens returns balances for address (optionally one chain and token filter), cached briefly.
func (r *Reader) ReadTokens(ctx context.Context, address, only, tokens string) (Result, error) {
	targets, err := r.Chains(address, only)
	if err != nil {
		return Result{}, err
	}
	key := strings.ToLower(strings.TrimSpace(address)) + "|" + strings.ToLower(only) + "|" + strings.ToLower(tokens)
	now := r.cfg.Now()
	r.mu.Lock()
	if c, ok := r.cache[key]; ok && now.Sub(c.at) < r.cfg.CacheTTL {
		r.mu.Unlock()
		return c.result, nil
	}
	r.mu.Unlock()

	v, err, _ := r.flight.Do(key, func() (any, error) {
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*r.cfg.Timeout)
		defer cancel()
		res, readErr := r.read(readCtx, targets, tokens)
		if readErr != nil {
			return nil, readErr
		}
		res.Address = strings.TrimSpace(address)
		res.Chain = only
		r.mu.Lock()
		if len(r.cache) >= maxCacheEntries {
			r.cache = map[string]cached{}
		}
		r.cache[key] = cached{result: res, at: r.cfg.Now()}
		r.mu.Unlock()
		return res, nil
	})
	if err != nil {
		return Result{}, err
	}
	return v.(Result), nil
}

func (r *Reader) read(ctx context.Context, targets map[string]string, tokens string) (Result, error) {
	type outcome struct {
		holdings []Holding
		err      error
	}
	keys := make([]string, 0, len(targets))
	for k := range targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	outcomes := make([]outcome, len(keys))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(8)
	for i, key := range keys {
		i, key := i, key
		group.Go(func() error {
			chain, _ := r.cfg.Registry.Get(key)
			h, err := r.readChain(groupCtx, chain, targets[key], tokens)
			outcomes[i] = outcome{holdings: h, err: err}
			return nil
		})
	}
	_ = group.Wait()

	res := Result{AsOf: r.cfg.Now(), Quality: "live"}
	for i, o := range outcomes {
		if o.err != nil {
			res.Failed = append(res.Failed, keys[i])
			continue
		}
		res.Holdings = append(res.Holdings, o.holdings...)
	}
	if len(res.Failed) == len(keys) {
		return Result{}, fmt.Errorf("%w: %s", ErrUnavailable, strings.Join(res.Failed, ", "))
	}
	if len(res.Failed) > 0 {
		res.Quality = "partial"
	}

	if r.cfg.Prices != nil && len(res.Holdings) > 0 {
		need := map[string]bool{}
		for _, h := range res.Holdings {
			need[h.PriceKey] = true
		}
		list := make([]string, 0, len(need))
		for k := range need {
			list = append(list, k)
		}
		sort.Strings(list)
		if prices, err := r.cfg.Prices.Current(ctx, list); err == nil {
			res.Priced = true
			for i := range res.Holdings {
				if p, ok := prices[res.Holdings[i].PriceKey]; ok {
					v := toFloat(res.Holdings[i].Atomic, res.Holdings[i].Decimals) * p
					res.Holdings[i].ValueUSD = &v
				}
			}
		}
	}
	sort.SliceStable(res.Holdings, func(a, b int) bool {
		va, vb := res.Holdings[a].ValueUSD, res.Holdings[b].ValueUSD
		if va != nil && vb != nil {
			return *va > *vb
		}
		return va != nil
	})
	return res, nil
}

func toFloat(atomic *big.Int, decimals int) float64 {
	f, _ := new(big.Float).Quo(new(big.Float).SetInt(atomic),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))).Float64()
	return f
}

func (r *Reader) endpoints(chain registry.Chain) []string {
	if urls, ok := r.cfg.Endpoints[chain.Key]; ok {
		return urls
	}
	return chain.RPCURLs
}

func (r *Reader) holding(chain registry.Chain, symbol, assetID string, amount *big.Int, decimals int, priceKey, source string) Holding {
	return Holding{AssetID: assetID, Symbol: symbol, Chain: chain.Key, Atomic: amount, Decimals: decimals,
		PriceKey: priceKey, Source: source, AsOf: r.cfg.Now()}
}

func (r *Reader) readChain(ctx context.Context, chain registry.Chain, address, tokens string) ([]Holding, error) {
	urls := r.endpoints(chain)
	native := chain.NativeCurrency
	nativeID := chain.Key + ":" + strings.ToLower(native.Symbol)
	source := chain.Name + " RPC"
	includeCatalogue := strings.EqualFold(tokens, "catalogue") || tokens == ""

	switch chain.Family {
	case registry.FamilyEVM:
		client := &jsonrpc.Client{URLs: urls, HTTPClient: r.cfg.HTTPClient, Timeout: r.cfg.Timeout}
		var hexBal string
		if err := client.Call(ctx, "eth_getBalance", []any{address, "latest"}, &hexBal); err != nil {
			return nil, err
		}
		bal, err := parseHexQuantity(hexBal)
		if err != nil {
			return nil, err
		}
		out := []Holding{r.holding(chain, native.Symbol, nativeID, bal, native.Decimals, nativePriceKeys[chain.Key], source)}

		var catTokens []TokenAsset
		if includeCatalogue && r.cfg.Catalogue != nil {
			catTokens, _ = r.cfg.Catalogue.ListTokens(ctx, chain.Key)
		}

		var multicallHoldings []Holding
		if len(catTokens) > 0 {
			multicallHoldings = r.readEVMTokens(ctx, client, chain, address, catTokens)
		}

		if len(multicallHoldings) > 0 {
			out = append(out, multicallHoldings...)
		} else if t, ok := usdcByChain[chain.Key]; ok {
			data := "0x70a08231" + strings.Repeat("0", 24) + strings.ToLower(strings.TrimPrefix(address, "0x"))
			var hexTok string
			if err := client.Call(ctx, "eth_call", []any{map[string]string{"to": t.contract, "data": data}, "latest"}, &hexTok); err == nil {
				if tb, perr := parseHexQuantity(hexTok); perr == nil && tb.Sign() > 0 {
					out = append(out, r.holding(chain, t.symbol, chain.Key+":usdc", tb, t.decimals, t.priceKey, source))
				}
			}
		}
		return out, nil

	case registry.FamilySolana:
		client := &jsonrpc.Client{URLs: urls, HTTPClient: r.cfg.HTTPClient, Timeout: r.cfg.Timeout}
		var res struct {
			Value uint64 `json:"value"`
		}
		if err := client.Call(ctx, "getBalance", []any{address}, &res); err != nil {
			return nil, err
		}
		out := []Holding{r.holding(chain, native.Symbol, nativeID, new(big.Int).SetUint64(res.Value), native.Decimals, nativePriceKeys[chain.Key], source)}
		if includeCatalogue {
			var catTokens []TokenAsset
			if r.cfg.Catalogue != nil {
				catTokens, _ = r.cfg.Catalogue.ListTokens(ctx, chain.Key)
			}
			spls := r.readSolanaSPLTokens(ctx, client, chain, address, catTokens)
			out = append(out, spls...)
		}
		return out, nil

	case registry.FamilyNEAR:
		client := &jsonrpc.Client{URLs: urls, HTTPClient: r.cfg.HTTPClient, Timeout: r.cfg.Timeout}
		var res struct {
			Amount string `json:"amount"`
		}
		err := client.Call(ctx, "query", map[string]string{
			"request_type": "view_account", "finality": "final", "account_id": address,
		}, &res)
		amount := big.NewInt(0)
		if err != nil {
			var rpcErr *jsonrpc.Error
			if !errors.As(err, &rpcErr) || !strings.Contains(string(rpcErr.Data)+rpcErr.Message, "does not exist") {
				return nil, err
			}
		} else if _, ok := amount.SetString(res.Amount, 10); !ok {
			return nil, fmt.Errorf("invalid NEAR amount %q", res.Amount)
		}
		return []Holding{r.holding(chain, native.Symbol, nativeID, amount, native.Decimals, nativePriceKeys[chain.Key], source)}, nil

	case registry.FamilyStellar:
		fetcher := &rpc.HTTPFetcher{URLs: urls, Client: r.cfg.HTTPClient, Timeout: r.cfg.Timeout}
		var acct struct {
			Balances []struct {
				AssetType string `json:"asset_type"`
				Balance   string `json:"balance"`
			} `json:"balances"`
		}
		amount := big.NewInt(0)
		if err := fetcher.Get(ctx, "/accounts/"+url.PathEscape(address), &acct); err != nil {
			var status *rpc.StatusError
			if !errors.As(err, &status) || !status.NotFound() {
				return nil, err
			}
		}
		for _, b := range acct.Balances {
			if b.AssetType == "native" {
				parsed, err := domain.ParseUnits(b.Balance, native.Decimals)
				if err != nil {
					return nil, fmt.Errorf("invalid Stellar balance %q", b.Balance)
				}
				amount = parsed
			}
		}
		return []Holding{r.holding(chain, native.Symbol, nativeID, amount, native.Decimals, nativePriceKeys[chain.Key], source)}, nil

	case registry.FamilyMove:
		fetcher := &rpc.HTTPFetcher{URLs: urls, Client: r.cfg.HTTPClient, Timeout: r.cfg.Timeout}
		var raw json.RawMessage
		if err := fetcher.Get(ctx, "/accounts/"+address+"/balance/0x1::aptos_coin::AptosCoin", &raw); err != nil {
			return nil, err
		}
		amount, ok := new(big.Int).SetString(strings.Trim(string(raw), "\" \n"), 10)
		if !ok {
			return nil, fmt.Errorf("invalid Aptos balance %q", string(raw))
		}
		return []Holding{r.holding(chain, native.Symbol, nativeID, amount, native.Decimals, nativePriceKeys[chain.Key], source)}, nil
	}
	return nil, fmt.Errorf("unsupported chain family %q", chain.Family)
}

func parseHexQuantity(value string) (*big.Int, error) {
	v := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if v == "" {
		return big.NewInt(0), nil
	}
	n, ok := new(big.Int).SetString(v, 16)
	if !ok {
		return nil, fmt.Errorf("invalid hex quantity %q", value)
	}
	return n, nil
}

// LlamaPrices reads https://coins.llama.fi/prices/current with a short cache.
type LlamaPrices struct {
	BaseURL string
	Client  *http.Client
	TTL     time.Duration

	mu    sync.Mutex
	cache map[string]priceEntry
}

type priceEntry struct {
	price float64
	at    time.Time
}

// Current returns prices for keys (missing keys are simply absent).
func (l *LlamaPrices) Current(ctx context.Context, keys []string) (map[string]float64, error) {
	ttl := l.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	now := time.Now()
	out := map[string]float64{}
	var missing []string
	l.mu.Lock()
	if l.cache == nil {
		l.cache = map[string]priceEntry{}
	}
	for _, k := range keys {
		if e, ok := l.cache[k]; ok && now.Sub(e.at) < ttl {
			out[k] = e.price
		} else {
			missing = append(missing, k)
		}
	}
	l.mu.Unlock()
	if len(missing) == 0 {
		return out, nil
	}
	base := strings.TrimRight(l.BaseURL, "/")
	if base == "" {
		base = "https://coins.llama.fi"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/prices/current/"+strings.Join(missing, ","), nil)
	if err != nil {
		return nil, err
	}
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("price source returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Coins map[string]struct {
			Price float64 `json:"price"`
		} `json:"coins"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	l.mu.Lock()
	for k, v := range payload.Coins {
		if v.Price > 0 {
			out[k] = v.Price
			l.cache[k] = priceEntry{price: v.Price, at: now}
		}
	}
	l.mu.Unlock()
	return out, nil
}
