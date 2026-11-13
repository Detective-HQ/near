// Command api serves the /api/v1 HTTP surface.
//
// It wires the swap orchestration service to PostgreSQL (or, only in
// APP_ENV=local without DATABASE_URL, to an in-memory store), the 1Click
// provider with its fallback chain, the chain registry, wallet sign-in, the
// risk engine and — when MARKET_DATA_MODE=live — DeFiLlama analytics.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"universal-defi-intelligence/internal/analytics/connectors/defillama"
	analyticsrepo "universal-defi-intelligence/internal/analytics/repository"
	analyticsservice "universal-defi-intelligence/internal/analytics/service"
	"universal-defi-intelligence/internal/auth"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/chains/rpc"
	"universal-defi-intelligence/internal/chains/rpc/jsonrpc"
	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpapi"
	"universal-defi-intelligence/internal/httpclient"
	"universal-defi-intelligence/internal/research/scenarios"
	"universal-defi-intelligence/internal/risk"
	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/memstore"
	"universal-defi-intelligence/internal/swap/providers"
	"universal-defi-intelligence/internal/swap/providers/alternative"
	"universal-defi-intelligence/internal/swap/providers/nearoneclick"
	"universal-defi-intelligence/internal/swap/repository"
	"universal-defi-intelligence/internal/swap/service"
	"universal-defi-intelligence/internal/trading"
	"universal-defi-intelligence/migrations"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "api: %v\n", err)
		os.Exit(1)
	}
}

// runHealthcheck probes /healthz on the local listener (distroless images have
// no shell or curl).
func runHealthcheck() error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid HTTP_ADDR %q: %w", addr, err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func run() error {
	// Invalid configuration is always fatal. Silently falling back to demo
	// defaults would let a misconfigured production deploy serve fake data.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.Log)
	logger.Info("api starting", "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("load chain registry: %w", err)
	}
	riskEngine := risk.NewEngine(risk.Config{
		MaxSlippageBps:     500,
		WarningSlippageBps: 200,
		CatalogueMaxAge:    2 * cfg.Catalogue.TTL,
	})

	// ---- persistence ------------------------------------------------------
	var (
		pool       *postgres.Pool
		store      service.Store
		authStore  *auth.PostgresStore
		analytics  analyticsservice.Store
		probes     []httpapi.Probe
		closeStore = func() {}
	)
	if cfg.Database.URL != "" {
		pool, err = postgres.Open(ctx, cfg.Database)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		closeStore = pool.Close
		if err := requireCurrentSchema(ctx, pool); err != nil {
			pool.Close()
			return err
		}
		store = repository.New(pool.Pool)
		authStore = auth.NewPostgresStore(pool.Pool)
		analytics = analyticsrepo.New(pool.Pool)
		probes = append(probes, httpapi.Probe{Name: "database", Check: pool.Ping})
		logger.Info("using PostgreSQL store")
	} else {
		// config.Load only allows an empty DATABASE_URL in APP_ENV=local.
		store = memstore.New()
		logger.Warn("DATABASE_URL is not set; using the in-memory store (local development only, not durable)")
	}
	defer closeStore()

	// ---- provider -----------------------------------------------------------
	oneclick, err := nearoneclick.New(nearoneclick.Config{
		BaseURL:          cfg.OneClick.BaseURL,
		JWT:              cfg.OneClick.JWT,
		Timeout:          cfg.OneClick.Timeout,
		MaxResponseBytes: cfg.OneClick.MaxResponseBytes,
		QuotePublicKey:   cfg.OneClick.QuotePublicKey,
		Logger:           logger,
		HTTPClient:       httpclient.New(cfg.OneClick.Timeout),
	})
	if err != nil {
		return fmt.Errorf("configure 1click provider: %w", err)
	}
	if cfg.Execution.QuoteMode == config.QuoteModeLive && cfg.OneClick.JWT == "" {
		logger.Warn("SWAP_QUOTE_MODE=live without ONECLICK_JWT: quotes will return PROVIDER_NOT_CONFIGURED")
	}
	provider := providers.NewChain(oneclick, alternative.New())

	swaps, err := service.New(service.Config{
		Store:               store,
		Provider:            provider,
		Registry:            reg,
		QuoteMode:           cfg.Execution.QuoteMode,
		EnableLiveExecution: cfg.Execution.EnableLiveExecution,
		QuoteDeadline:       cfg.OneClick.QuoteDeadline,
		StatusMaxAge:        cfg.Worker.StatusMaxAge,
		CatalogueTTL:        cfg.Catalogue.TTL,
		Observer:            riskEngine,
		Logger:              logger,
	})
	if err != nil {
		return fmt.Errorf("configure swap service: %w", err)
	}

	// ---- auth ---------------------------------------------------------------
	authCfg := auth.Config{
		AuthRequired: cfg.Security.AuthRequired,
		SessionTTL:   cfg.Security.SessionTTL,
		Registry:     reg,
	}
	if authStore != nil {
		authCfg.Nonces, authCfg.Sessions = authStore, authStore
	}
	if nearChain, ok := reg.Get("near"); ok {
		authCfg.NEARKeys = &rpc.NEARAccessKeyVerifier{RPC: &jsonrpc.Client{
			URLs: nearChain.RPCURLs, Timeout: 5 * time.Second, HTTPClient: httpclient.New(5 * time.Second),
		}}
	}
	authManager := auth.NewManager(authCfg)

	// ---- analytics ----------------------------------------------------------
	var market *analyticsservice.Reader
	var tradingEngine *trading.Engine
	if cfg.MarketData.Mode == config.MarketDataLive {
		var journal trading.Journal
		if pool != nil {
			journal = trading.NewPostgresJournal(pool.Pool)
		}
		tradingEngine = trading.NewEngine(trading.Config{
			Source:  &trading.DefiLlamaPrices{Client: httpclient.New(cfg.MarketData.Timeout)},
			Journal: journal,
			Logger:  logger,
		})
		// Compute the first scan in the background so the first user request
		// does not pay for ~40 upstream calls.
		tradingEngine.Warm()
		connector := defillama.New(defillama.Config{
			Timeout:    cfg.MarketData.Timeout,
			Logger:     logger,
			HTTPClient: httpclient.New(cfg.MarketData.Timeout),
		})
		ingest := analyticsservice.New(analyticsservice.Config{
			Connector:  connector,
			Store:      analytics,
			StorageDir: os.Getenv("ANALYTICS_SNAPSHOT_DIR"),
			Logger:     logger,
		})
		market = analyticsservice.NewReader(ingest, cfg.MarketData.TTL)
	}

	handler, err := httpapi.NewHandler(httpapi.HandlerOptions{
		Swaps:         swaps,
		Auth:          authManager,
		Risk:          riskEngine,
		Market:        market,
		Trading:       tradingEngine,
		Research:      scenarios.New(),
		Config:        cfg,
		Probes:        probes,
		Logger:        logger,
		SecureCookies: cfg.AppEnv != config.EnvLocal,
	})
	if err != nil {
		return err
	}
	server := httpapi.NewServer(cfg, httpapi.NewRouter(handler, logger, httpapi.RouterOptionsFromConfig(cfg)))

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", cfg.HTTP.Addr, "quoteMode", cfg.Execution.QuoteMode,
			"marketData", cfg.MarketData.Mode, "authRequired", cfg.Security.AuthRequired)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down api server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	// Let in-flight provider deposit notifications finish.
	if err := swaps.Wait(shutdownCtx); err != nil {
		logger.Warn("background tasks did not finish before shutdown", "error", err)
	}
	logger.Info("api stopped cleanly")
	return nil
}

// requireCurrentSchema refuses to serve against a database whose migrations
// lag behind this binary; run `migrate up` first.
func requireCurrentSchema(ctx context.Context, pool *postgres.Pool) error {
	migrator, err := postgres.NewMigrator(pool.DB(), migrations.FS)
	if err != nil {
		return fmt.Errorf("prepare migration check: %w", err)
	}
	defer func() { _ = migrator.Close() }()
	pending, err := migrator.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("check schema version: %w", err)
	}
	if pending {
		return errors.New("database schema is behind this binary; run `migrate up` before starting the API")
	}
	return nil
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	options := &slog.HandlerOptions{Level: cfg.Level}
	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		handler = slog.NewTextHandler(os.Stdout, options)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, options)
	}
	return slog.New(handler)
}
