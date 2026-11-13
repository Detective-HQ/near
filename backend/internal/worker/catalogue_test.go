package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	rediscache "universal-defi-intelligence/internal/storage/redis"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/worker"
)

type fakeAssetStore struct {
	mu      sync.Mutex
	calls   int
	assets  []domain.Asset
	source  string
	saveErr error
}

func (s *fakeAssetStore) UpsertAssets(_ context.Context, assets []domain.Asset, source string, _ time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.assets = assets
	s.source = source
	if s.saveErr != nil {
		return 0, s.saveErr
	}
	return len(assets), nil
}

func (s *fakeAssetStore) snapshot() (int, []domain.Asset, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.assets, s.source
}

func newRedisClient(t *testing.T) (*rediscache.Client, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rediscache.New(rdb, "test", time.Second), server
}

func sampleTokens() []domain.Asset {
	return []domain.Asset{
		{ProviderAssetID: "near:wrap.near", Symbol: "wNEAR", Decimals: 24, Chain: "near", Enabled: true},
		{ProviderAssetID: "eth:0xabc", Symbol: "USDC", Decimals: 6, Chain: "ethereum", Enabled: true},
	}
}

func TestCatalogueRefreshUpsertsAndCaches(t *testing.T) {
	client, server := newRedisClient(t)
	store := &fakeAssetStore{}
	provider := &fakeProvider{name: "fake", available: true, tokens: sampleTokens()}
	health := worker.NewHealth()

	refresher := worker.NewCatalogueRefresher(worker.CatalogueConfig{
		Provider: provider,
		Store:    store,
		Cache:    client,
		Health:   health,
		Logger:   discardLogger(),
		Clock:    newFakeClock(time.Now().UTC()),
	})

	count, err := refresher.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	calls, assets, source := store.snapshot()
	if calls != 1 || len(assets) != 2 || source != "fake" {
		t.Fatalf("store calls=%d assets=%d source=%q", calls, len(assets), source)
	}
	if !server.Exists(client.Key("catalogue", "fake")) {
		t.Fatal("catalogue was not cached")
	}
	snapshot, ok := health.Snapshot("fake")
	if !ok || snapshot.CatalogueSize != 2 {
		t.Fatalf("catalogue health = %+v (ok=%v), want size 2", snapshot, ok)
	}
}

func TestCatalogueRefreshStaleOnError(t *testing.T) {
	store := &fakeAssetStore{}
	provider := &fakeProvider{
		name:      "fake",
		available: true,
		tokenErr:  domain.New(domain.CodeProviderRateLimited, "slow down"),
	}
	health := worker.NewHealth()

	refresher := worker.NewCatalogueRefresher(worker.CatalogueConfig{
		Provider: provider,
		Store:    store,
		Health:   health,
		Logger:   discardLogger(),
		Clock:    newFakeClock(time.Now().UTC()),
		Backoff:  worker.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond},
	})

	if _, err := refresher.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh succeeded despite a provider failure")
	}
	if calls, _, _ := store.snapshot(); calls != 0 {
		t.Fatalf("store calls = %d, want 0 on a fetch failure (stale-on-error)", calls)
	}
	snapshot, _ := health.Snapshot("fake")
	if snapshot.Failures == 0 {
		t.Fatal("provider failure was not recorded in health")
	}
}

func TestCatalogueRefreshSkipsWhenProviderUnavailable(t *testing.T) {
	store := &fakeAssetStore{}
	provider := &fakeProvider{name: "fake", available: false}

	refresher := worker.NewCatalogueRefresher(worker.CatalogueConfig{
		Provider: provider,
		Store:    store,
		Logger:   discardLogger(),
		Clock:    newFakeClock(time.Now().UTC()),
	})

	count, err := refresher.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if calls, _, _ := store.snapshot(); calls != 0 {
		t.Fatalf("store calls = %d, want 0", calls)
	}
	if got := provider.tokenCalls.Load(); got != 0 {
		t.Fatalf("provider token calls = %d, want 0", got)
	}
}

func TestCatalogueRefreshSkipsWhenLockHeld(t *testing.T) {
	client, _ := newRedisClient(t)
	ctx := context.Background()
	store := &fakeAssetStore{}
	provider := &fakeProvider{name: "fake", available: true, tokens: sampleTokens()}

	// Another worker holds the catalogue lock.
	if _, acquired, err := client.SetNX(ctx, client.Key("lock", "catalogue", "fake"), time.Minute); err != nil || !acquired {
		t.Fatalf("pre-acquire lock: acquired=%v err=%v", acquired, err)
	}

	refresher := worker.NewCatalogueRefresher(worker.CatalogueConfig{
		Provider: provider,
		Store:    store,
		Cache:    client,
		Logger:   discardLogger(),
		Clock:    newFakeClock(time.Now().UTC()),
	})

	count, err := refresher.Refresh(ctx)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 when the lock is held", count)
	}
	if got := provider.tokenCalls.Load(); got != 0 {
		t.Fatalf("provider token calls = %d, want 0", got)
	}
}

func TestCatalogueRefreshWithoutCacheStillRuns(t *testing.T) {
	store := &fakeAssetStore{}
	provider := &fakeProvider{name: "fake", available: true, tokens: sampleTokens()}

	refresher := worker.NewCatalogueRefresher(worker.CatalogueConfig{
		Provider: provider,
		Store:    store,
		Logger:   discardLogger(),
		Clock:    newFakeClock(time.Now().UTC()),
	})

	count, err := refresher.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 without a cache", count)
	}
}
