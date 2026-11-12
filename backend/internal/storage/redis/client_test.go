package redis_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"universal-defi-intelligence/internal/config"
	rediscache "universal-defi-intelligence/internal/storage/redis"
	"universal-defi-intelligence/internal/swap/domain"
)

func newTestClient(t *testing.T, appEnv string) (*rediscache.Client, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	return rediscache.New(rdb, appEnv, time.Second), server
}

func TestKeyPrefixing(t *testing.T) {
	client, _ := newTestClient(t, "Production")

	if got := client.Prefix(); got != "udi:production" {
		t.Fatalf("prefix = %q, want %q", got, "udi:production")
	}
	if got := client.Key("catalogue", "near"); got != "udi:production:catalogue:near" {
		t.Fatalf("key = %q, want %q", got, "udi:production:catalogue:near")
	}
	if got := client.Key("", "lock", "  ", "catalogue"); got != "udi:production:lock:catalogue" {
		t.Fatalf("key with empty segments = %q", got)
	}
	if got := client.Key(":trim:"); got != "udi:production:trim" {
		t.Fatalf("key with colons = %q", got)
	}
}

func TestGetOrSetJSONCacheAside(t *testing.T) {
	client, server := newTestClient(t, "local")
	ctx := context.Background()

	type payload struct {
		Value int `json:"value"`
	}

	var loads atomic.Int32
	load := func(context.Context) (payload, error) {
		loads.Add(1)
		return payload{Value: 42}, nil
	}

	key := client.Key("cache", "answer")

	first, hit, err := rediscache.GetOrSetJSON(ctx, client, key, time.Minute, load)
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	if hit {
		t.Fatal("first get reported a cache hit")
	}
	if first.Value != 42 {
		t.Fatalf("first value = %d, want 42", first.Value)
	}
	if !server.Exists(key) {
		t.Fatalf("key %q was not written", key)
	}

	second, hit, err := rediscache.GetOrSetJSON(ctx, client, key, time.Minute, load)
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	if !hit {
		t.Fatal("second get did not report a cache hit")
	}
	if second.Value != 42 {
		t.Fatalf("second value = %d, want 42", second.Value)
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestGetOrSetJSONNilClientLoads(t *testing.T) {
	ctx := context.Background()
	value, hit, err := rediscache.GetOrSetJSON(ctx, nil, "ignored", time.Minute, func(context.Context) (int, error) {
		return 7, nil
	})
	if err != nil {
		t.Fatalf("nil client get: %v", err)
	}
	if hit || value != 7 {
		t.Fatalf("nil client result = (%d, %v), want (7, false)", value, hit)
	}
}

func TestSetNXLockLifecycle(t *testing.T) {
	client, server := newTestClient(t, "local")
	ctx := context.Background()
	key := client.Key("lock", "catalogue", "oneclick")

	lock, acquired, err := client.SetNX(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if !acquired {
		t.Fatal("first acquire did not succeed")
	}

	if _, second, err := client.SetNX(ctx, key, time.Minute); err != nil {
		t.Fatalf("second acquire: %v", err)
	} else if second {
		t.Fatal("second acquire succeeded while the lock was held")
	}

	released, err := lock.Release(ctx)
	if err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if !released {
		t.Fatal("release reported failure for an owned lock")
	}
	if server.Exists(key) {
		t.Fatalf("key %q still exists after release", key)
	}

	if _, reacquired, err := client.SetNX(ctx, key, time.Minute); err != nil {
		t.Fatalf("re-acquire: %v", err)
	} else if !reacquired {
		t.Fatal("re-acquire after release failed")
	}
}

func TestLockReleaseRejectsForeignToken(t *testing.T) {
	client, server := newTestClient(t, "local")
	ctx := context.Background()
	key := client.Key("lock", "catalogue", "oneclick")

	lock, acquired, err := client.SetNX(ctx, key, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire lock: acquired=%v err=%v", acquired, err)
	}

	// Simulate the lock expiring and another worker taking it over.
	server.Set(key, "another-worker-token")

	released, err := lock.Release(ctx)
	if err != nil {
		t.Fatalf("release foreign lock: %v", err)
	}
	if released {
		t.Fatal("release deleted a lock owned by another worker")
	}
	if !server.Exists(key) {
		t.Fatal("foreign lock was deleted")
	}
}

func TestSetNXRejectsZeroTTL(t *testing.T) {
	client, _ := newTestClient(t, "local")
	if _, _, err := client.SetNX(context.Background(), client.Key("lock"), 0); err == nil {
		t.Fatal("SetNX with a zero TTL succeeded")
	} else if !domain.IsCode(err, domain.CodeValidationFailed) {
		t.Fatalf("error code = %s, want VALIDATION_FAILED", domain.CodeOf(err))
	}
}

func TestOpenRejectsInvalidURL(t *testing.T) {
	_, err := rediscache.Open(context.Background(), config.RedisConfig{URL: "http://not-redis:6379"}, "local")
	if err == nil {
		t.Fatal("Open accepted a non-redis URL")
	}
	if !domain.IsCode(err, domain.CodeInternal) {
		t.Fatalf("error code = %s, want INTERNAL", domain.CodeOf(err))
	}
}

func TestOpenEmptyURL(t *testing.T) {
	if _, err := rediscache.Open(context.Background(), config.RedisConfig{}, "local"); err == nil {
		t.Fatal("Open accepted an empty URL")
	}
}

func TestPing(t *testing.T) {
	client, _ := newTestClient(t, "local")
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
