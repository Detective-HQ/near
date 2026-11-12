package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	rediscache "universal-defi-intelligence/internal/storage/redis"
	"universal-defi-intelligence/internal/swap/domain"
)

// AssetStore persists a provider catalogue. *repository.Store implements it.
type AssetStore interface {
	UpsertAssets(ctx context.Context, assets []domain.Asset, source string, observedAt time.Time) (int, error)
}

// CatalogueConfig tunes the catalogue refresh job.
type CatalogueConfig struct {
	Provider Provider
	Store    AssetStore
	// Cache is optional. When nil the refresh still runs, just without a
	// distributed lock or a cached copy.
	Cache    *rediscache.Client
	Health   *Health
	Logger   *slog.Logger
	Clock    Clock
	TTL      time.Duration
	LockTTL  time.Duration
	Interval time.Duration
	Backoff  RetryPolicy
}

func (c CatalogueConfig) withDefaults() CatalogueConfig {
	if c.TTL <= 0 {
		c.TTL = 5 * time.Minute
	}
	if c.LockTTL <= 0 {
		c.LockTTL = 30 * time.Second
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Minute
	}
	if c.Backoff.MaxAttempts <= 0 {
		c.Backoff = DefaultRetryPolicy()
	}
	return c
}

// CatalogueRefresher periodically refreshes the provider token catalogue into
// PostgreSQL, using a Redis lock so only one worker does the work.
type CatalogueRefresher struct {
	cfg CatalogueConfig
}

// NewCatalogueRefresher creates a refresher. Nil health and logger are replaced
// with safe defaults.
func NewCatalogueRefresher(cfg CatalogueConfig) *CatalogueRefresher {
	if cfg.Health == nil {
		cfg.Health = NewHealth()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = SystemClock{}
	}
	return &CatalogueRefresher{cfg: cfg.withDefaults()}
}

// Run refreshes the catalogue until ctx is cancelled.
func (c *CatalogueRefresher) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	for {
		if _, err := c.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
			c.cfg.Logger.Error("catalogue refresh failed", "error", err, "provider", c.cfg.Provider.Name())
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Refresh fetches the provider catalogue and persists it.
//
// A fetch failure is stale-on-error: the previous catalogue rows are left
// untouched (never disabled), and the failure is recorded in health. Assets the
// provider no longer returns are soft-disabled by the store, never deleted.
func (c *CatalogueRefresher) Refresh(ctx context.Context) (int, error) {
	if !c.cfg.Provider.Available() {
		return 0, nil
	}

	if c.cfg.Cache != nil {
		lock, acquired, err := c.acquireLock(ctx)
		if err != nil {
			// A Redis failure must not stop the refresh; it only removes the
			// cross-worker deduplication.
			c.cfg.Logger.Warn("catalogue lock unavailable; refreshing without a lock", "error", err)
		} else if !acquired {
			return 0, nil
		} else {
			defer func() {
				if released, releaseErr := lock.Release(context.WithoutCancel(ctx)); releaseErr != nil {
					c.cfg.Logger.Warn("release catalogue lock", "error", releaseErr)
				} else if !released {
					c.cfg.Logger.Debug("catalogue lock expired before release")
				}
			}()
		}
	}

	now := c.cfg.Clock.Now()

	var assets []domain.Asset
	err := c.cfg.Backoff.Do(ctx, func(callCtx context.Context) error {
		var fetchErr error
		assets, fetchErr = c.cfg.Provider.GetTokens(callCtx)
		return fetchErr
	})
	if err != nil {
		c.cfg.Health.RecordError(c.cfg.Provider.Name(), domain.CodeOf(err), now)
		return 0, err
	}

	count, err := c.cfg.Store.UpsertAssets(ctx, assets, c.cfg.Provider.Name(), now)
	if err != nil {
		c.cfg.Health.RecordError(c.cfg.Provider.Name(), domain.CodeOf(err), now)
		return 0, err
	}

	c.cfg.Health.RecordCatalogue(c.cfg.Provider.Name(), count, now)
	if c.cfg.Cache != nil {
		if err := c.cfg.Cache.SetJSON(ctx, c.cfg.Cache.Key("catalogue", c.cfg.Provider.Name()), assets, c.cfg.TTL); err != nil {
			c.cfg.Logger.Warn("cache catalogue", "error", err)
		}
	}

	c.cfg.Logger.Info("catalogue refreshed", "provider", c.cfg.Provider.Name(), "assets", count)
	return count, nil
}

func (c *CatalogueRefresher) acquireLock(ctx context.Context) (rediscache.Lock, bool, error) {
	key := c.cfg.Cache.Key("lock", "catalogue", c.cfg.Provider.Name())
	lock, acquired, err := c.cfg.Cache.SetNX(ctx, key, c.cfg.LockTTL)
	if err != nil {
		return rediscache.Lock{}, false, err
	}
	return lock, acquired, nil
}
