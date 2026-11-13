// Package redis contains the Redis connection manager, keyspace helpers, a
// JSON cache-aside helper, and a token-checked distributed lock.
//
// Every key is namespaced with a "udi:{env}" prefix so several environments can
// share one Redis instance without collisions. Cache reads and writes are
// best-effort: a Redis outage must degrade to a cache miss, never fail an API
// request. Locks are strict, because two workers refreshing the catalogue at the
// same time is a correctness problem, not a performance one.
package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
)

const (
	// KeyPrefix is the root namespace for every key the backend writes.
	KeyPrefix = "udi"
	// defaultTimeout bounds a single Redis round trip when the config omits one.
	defaultTimeout = 2 * time.Second
)

// releaseScript deletes a lock only when it still holds the caller's token, so a
// worker that outlived its TTL cannot release a lock another worker now owns.
const releaseScript = `
if redis.call('get', KEYS[1]) == ARGV[1] then
	return redis.call('del', KEYS[1])
else
	return 0
end`

// Client wraps a go-redis universal client with the backend keyspace and
// best-effort JSON cache helpers.
type Client struct {
	client  goredis.UniversalClient
	prefix  string
	timeout time.Duration
}

// Open connects to Redis from the validated configuration and verifies the
// connection with a bounded ping. appEnv becomes part of the key prefix.
func Open(ctx context.Context, cfg config.RedisConfig, appEnv string) (*Client, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, domain.New(domain.CodeInternal, "Redis is not configured.")
	}

	options, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "The Redis configuration is invalid.", err)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	options.DialTimeout = timeout
	options.ReadTimeout = timeout
	options.WriteTimeout = timeout
	options.ContextTimeoutEnabled = true

	client := &Client{
		client:  goredis.NewClient(options),
		prefix:  fmt.Sprintf("%s:%s", KeyPrefix, sanitizeSegment(appEnv)),
		timeout: timeout,
	}

	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// New wraps an existing go-redis client. It is used by tests with an in-process
// Redis (miniredis) and by callers that need a custom client.
func New(client goredis.UniversalClient, appEnv string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		client:  client,
		prefix:  fmt.Sprintf("%s:%s", KeyPrefix, sanitizeSegment(appEnv)),
		timeout: timeout,
	}
}

// Prefix returns the environment-scoped key prefix (for example "udi:local").
func (c *Client) Prefix() string {
	if c == nil {
		return ""
	}
	return c.prefix
}

// Key joins path segments under the environment prefix. Empty segments are
// skipped and surrounding colons are trimmed so callers cannot accidentally
// escape the namespace.
func (c *Client) Key(parts ...string) string {
	if c == nil {
		return ""
	}
	segments := make([]string, 0, len(parts)+1)
	segments = append(segments, c.prefix)
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), ":")
		if part != "" {
			segments = append(segments, part)
		}
	}
	return strings.Join(segments, ":")
}

// Ping verifies connectivity with the configured timeout.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.client == nil {
		return domain.New(domain.CodeInternal, "Redis is not configured.")
	}
	pingCtx, cancel := c.withTimeout(ctx)
	defer cancel()
	if err := c.client.Ping(pingCtx).Err(); err != nil {
		return domain.Wrap(domain.CodeInternal, "Redis is unavailable.", err)
	}
	return nil
}

// Close releases the underlying client.
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// GetJSON reads and decodes a key. The bool reports whether the key existed; a
// missing key is not an error.
func (c *Client) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	if c == nil || c.client == nil {
		return false, domain.New(domain.CodeInternal, "Redis is not configured.")
	}
	readCtx, cancel := c.withTimeout(ctx)
	defer cancel()
	raw, err := c.client.Get(readCtx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, domain.Wrap(domain.CodeInternal, "The Redis read failed.", err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		// A corrupt cached value is treated as a miss rather than an outage.
		return false, nil
	}
	return true, nil
}

// SetJSON encodes and stores a value with a TTL. A non-positive TTL stores the
// value without expiry.
func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return domain.New(domain.CodeInternal, "Redis is not configured.")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return domain.Wrap(domain.CodeInternal, "The value could not be serialized.", err)
	}
	writeCtx, cancel := c.withTimeout(ctx)
	defer cancel()
	if err := c.client.Set(writeCtx, key, raw, ttl).Err(); err != nil {
		return domain.Wrap(domain.CodeInternal, "The Redis write failed.", err)
	}
	return nil
}

// Delete removes keys.
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if c == nil || c.client == nil {
		return domain.New(domain.CodeInternal, "Redis is not configured.")
	}
	if len(keys) == 0 {
		return nil
	}
	deleteCtx, cancel := c.withTimeout(ctx)
	defer cancel()
	if err := c.client.Del(deleteCtx, keys...).Err(); err != nil {
		return domain.Wrap(domain.CodeInternal, "The Redis delete failed.", err)
	}
	return nil
}

// GetOrSetJSON implements the cache-aside pattern. On a hit it returns the cached
// value with hit=true. On a miss it calls load, stores the result for ttl, and
// returns hit=false.
//
// Cache reads and writes are best-effort: a Redis failure degrades to a live
// load so an outage never breaks the caller.
func GetOrSetJSON[T any](
	ctx context.Context,
	c *Client,
	key string,
	ttl time.Duration,
	load func(context.Context) (T, error),
) (T, bool, error) {
	var zero T
	if c == nil || c.client == nil {
		value, err := load(ctx)
		return value, false, err
	}

	var cached T
	if hit, err := c.GetJSON(ctx, key, &cached); err == nil && hit {
		return cached, true, nil
	}

	value, err := load(ctx)
	if err != nil {
		return zero, false, err
	}
	_ = c.SetJSON(ctx, key, value, ttl)
	return value, false, nil
}

// Lock is a held distributed lock. Release must be called with the same token
// that acquired it.
type Lock struct {
	client *Client
	key    string
	token  string
}

// Key returns the locked key.
func (l Lock) Key() string { return l.key }

// Token returns the ownership token.
func (l Lock) Token() string { return l.token }

// SetNX attempts to acquire key for ttl and returns the lock when successful.
// The second result reports whether the lock was acquired.
func (c *Client) SetNX(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	if c == nil || c.client == nil {
		return Lock{}, false, domain.New(domain.CodeInternal, "Redis is not configured.")
	}
	if ttl <= 0 {
		return Lock{}, false, domain.New(domain.CodeValidationFailed, "Lock TTL must be greater than zero.")
	}
	token, err := newToken()
	if err != nil {
		return Lock{}, false, domain.Wrap(domain.CodeInternal, "The lock token could not be generated.", err)
	}

	lockCtx, cancel := c.withTimeout(ctx)
	defer cancel()
	acquired, err := c.client.SetNX(lockCtx, key, token, ttl).Result()
	if err != nil {
		return Lock{}, false, domain.Wrap(domain.CodeInternal, "The Redis lock could not be acquired.", err)
	}
	if !acquired {
		return Lock{}, false, nil
	}
	return Lock{client: c, key: key, token: token}, true, nil
}

// Release deletes the lock only if it still holds the caller's token. It returns
// true when the lock was released and false when it had already expired or been
// taken over by another owner.
func (l Lock) Release(ctx context.Context) (bool, error) {
	if l.client == nil || l.client.client == nil {
		return false, nil
	}
	releaseCtx, cancel := l.client.withTimeout(ctx)
	defer cancel()
	result, err := l.client.client.Eval(releaseCtx, releaseScript, []string{l.key}, l.token).Int64()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return false, domain.Wrap(domain.CodeInternal, "The Redis lock could not be released.", err)
	}
	return result == 1, nil
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c == nil || c.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

func sanitizeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "local"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

func newToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
