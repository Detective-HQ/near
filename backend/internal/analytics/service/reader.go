package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"universal-defi-intelligence/internal/analytics/domain"
)

// minForcedRefresh bounds how often ?refresh=true can bypass the cache, so the
// public endpoint cannot be used to hammer the upstream source.
const minForcedRefresh = 30 * time.Second

type cachedSnapshot struct {
	snapshot  domain.Snapshot
	fetchedAt time.Time
}

// Reader serves chain snapshots to the API with a TTL cache.
//
// Concurrent requests for the same chain share one upstream fetch
// (singleflight). When a refresh fails, the previous snapshot keeps being
// served and is marked stale rather than turning the page into an error.
type Reader struct {
	ingest *Service
	ttl    time.Duration
	now    func() time.Time

	mu     sync.RWMutex
	cache  map[string]cachedSnapshot
	flight singleflight.Group
}

// NewReader wraps an ingestion service with a read cache.
func NewReader(ingest *Service, ttl time.Duration) *Reader {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Reader{ingest: ingest, ttl: ttl, now: ingest.now, cache: map[string]cachedSnapshot{}}
}

// Snapshot returns the cached snapshot for chain, refreshing it when the TTL
// elapsed or when refresh is requested (at most once per 30 seconds).
func (r *Reader) Snapshot(ctx context.Context, chain string, refresh bool) (domain.Snapshot, error) {
	key := strings.ToLower(strings.TrimSpace(chain))
	now := r.now()

	r.mu.RLock()
	cached, ok := r.cache[key]
	r.mu.RUnlock()
	if ok {
		age := now.Sub(cached.fetchedAt)
		if age < r.ttl && (!refresh || age < minForcedRefresh) {
			return cached.snapshot, nil
		}
	}

	result, err, _ := r.flight.Do(key, func() (any, error) {
		// Detach from the caller so one cancelled request does not fail the
		// shared fetch for everybody waiting on it.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		snapshot, fetchErr := r.ingest.Ingest(fetchCtx, key)
		if fetchErr != nil {
			return nil, fetchErr
		}
		r.mu.Lock()
		r.cache[key] = cachedSnapshot{snapshot: snapshot, fetchedAt: r.now()}
		r.mu.Unlock()
		return snapshot, nil
	})
	if err != nil {
		if ok {
			stale := cached.snapshot
			stale.Status = domain.StatusStale
			stale.Notice = "Showing the last successful snapshot; the live source is currently unreachable."
			return stale, nil
		}
		return domain.Snapshot{}, err
	}
	return result.(domain.Snapshot), nil
}
