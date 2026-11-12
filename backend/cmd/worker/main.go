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
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/storage/postgres"
	rediscache "universal-defi-intelligence/internal/storage/redis"
	"universal-defi-intelligence/internal/swap/repository"
	"universal-defi-intelligence/internal/worker"
)

const heartbeatInterval = time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
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

	// Phase 4 replaces this placeholder with the authenticated 1Click adapter.
	// Until then the worker runs but every provider call reports
	// PROVIDER_NOT_CONFIGURED, and polling stays paused rather than failing swaps.
	provider := worker.NewUnavailableProvider("oneclick")
	if cfg.OneClick.JWT != "" {
		logger.Warn("ONECLICK_JWT is configured but the provider adapter is not implemented yet; polling stays paused")
	}

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

	err = group.Wait()
	if err == nil || errors.Is(err, context.Canceled) {
		logger.Info("worker stopped")
		return nil
	}
	return err
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
