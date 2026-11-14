// Command worker is the standalone background daemon.
//
// It runs three long-lived jobs under one errgroup and shuts them down together
// on SIGINT/SIGTERM:
//
//   - the swap status poller, which advances active swaps from provider status;
//   - the catalogue refresher, which keeps the provider token catalogue current;
//   - a heartbeat, so an idle worker is still visible in the logs.
//
// Configuration is loaded with the same rules as the API and the migrate
// command. Redis is optional: when it is unreachable the catalogue refresh runs
// without a distributed lock and the poller continues normally.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/httpclient"
	"universal-defi-intelligence/internal/metrics"
	"universal-defi-intelligence/internal/storage/postgres"
	rediscache "universal-defi-intelligence/internal/storage/redis"
	"universal-defi-intelligence/internal/swap/providers"
	"universal-defi-intelligence/internal/swap/providers/alternative"
	"universal-defi-intelligence/internal/swap/providers/nearoneclick"
	"universal-defi-intelligence/internal/swap/repository"
	"universal-defi-intelligence/internal/trading"
	"universal-defi-intelligence/internal/worker"
)

const heartbeatInterval = time.Minute

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "worker healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
}

func runHealthcheck() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	return pool.Ping(ctx)
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Log)
	logger.Info("worker starting", "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Database.URL == "" {
		return errors.New("DATABASE_URL is required for the worker")
	}

	pool, err := postgres.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := repository.New(pool.Pool)

	cache, err := rediscache.Open(ctx, cfg.Redis, string(cfg.AppEnv))
	if err != nil {
		logger.Warn("redis unavailable; catalogue locking and caching disabled", "error", err)
		cache = nil
	} else {
		defer func() { _ = cache.Close() }()
	}

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
	if cfg.OneClick.JWT == "" {
		logger.Warn("ONECLICK_JWT is not configured; provider status polling stays paused")
	}
	if cfg.OneClick.QuotePublicKey == "" {
		logger.Warn("ONECLICK_QUOTE_PUBLIC_KEY is not configured; quote signatures cannot be verified")
	}

	// The 1Click adapter is primary. The alternative stub is a real fallback
	// entry that reports NO_ROUTE until another provider is integrated.
	chain := providers.NewChain(oneclick, alternative.New())
	logger.Info("provider fallback chain configured", "providers", chain.JoinNames())
	provider := worker.NewAdapterProvider(chain)

	health := worker.NewHealth()

	poller := worker.NewPoller(provider, store, health, logger, worker.SystemClock{}, worker.PollerConfig{
		Interval:      cfg.Worker.PollInterval,
		Concurrency:   cfg.Worker.PollConcurrency,
		StatusMaxAge:  cfg.Worker.StatusMaxAge,
		MinInterval:   cfg.Worker.PollInterval,
		Backoff:       worker.DefaultRetryPolicy(),
		ProviderPause: 5 * time.Minute,
	})

	catalogue := worker.NewCatalogueRefresher(worker.CatalogueConfig{
		Provider: provider,
		Store:    store,
		Cache:    cache,
		Health:   health,
		Logger:   logger,
		Clock:    worker.SystemClock{},
		TTL:      cfg.Catalogue.TTL,
		Interval: cfg.Catalogue.TTL,
	})

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return poller.Run(groupCtx) })
	group.Go(func() error { return catalogue.Run(groupCtx) })
	group.Go(func() error { return heartbeat(groupCtx, logger) })
	if addr := cfg.Security.WorkerMetricsAddr; addr != "" {
		group.Go(func() error { return serveMetrics(groupCtx, addr, cfg.Security.MetricsToken, logger) })
	}
	if cfg.MarketData.Mode == config.MarketDataLive {
		engine := trading.NewEngine(trading.Config{
			Source:  &trading.DefiLlamaPrices{Client: httpclient.New(cfg.MarketData.Timeout)},
			Journal: trading.NewPostgresJournal(pool.Pool),
			Logger:  logger,
		})
		group.Go(func() error { return tradingJournal(groupCtx, engine, logger) })
	}

	err = group.Wait()
	if err == nil || errors.Is(err, context.Canceled) {
		logger.Info("worker stopped")
		return nil
	}
	return err
}

// tradingJournal recomputes trading signals on the shortest timeframe TTL so
// the signal journal is recorded continuously, not only when users visit.
func tradingJournal(ctx context.Context, engine *trading.Engine, logger *slog.Logger) error {
	interval := trading.Timeframes[0].TTL
	for {
		if err := engine.Refresh(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("trading journal refresh failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// serveMetrics exposes the worker's Prometheus metrics until ctx ends.
func serveMetrics(ctx context.Context, addr, token string, logger *slog.Logger) error {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler(token))
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	logger.Info("worker metrics listening", "addr", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return ctx.Err()
}

func heartbeat(ctx context.Context, logger *slog.Logger) error {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			logger.Debug("worker heartbeat")
		}
	}
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	options := &slog.HandlerOptions{Level: cfg.Level}
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, options)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, options)
	}
	return slog.New(handler)
}
