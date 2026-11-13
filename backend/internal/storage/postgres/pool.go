package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
)

const (
	defaultPingTimeout = 2 * time.Second
	poolMaxLifetime    = 30 * time.Minute
	poolMaxIdleTime    = 5 * time.Minute
	poolHealthPeriod   = 30 * time.Second
)

// Pool wraps pgxpool with the SQL handle required by the embedded migration
// runner. Closing the Pool closes both handles.
type Pool struct {
	*pgxpool.Pool

	sqlDB       *sql.DB
	pingTimeout time.Duration
}

// Open creates a pgx connection pool from the validated database
// configuration, then verifies connectivity with a bounded ping.
func Open(ctx context.Context, cfg config.DatabaseConfig) (*Pool, error) {
	if cfg.URL == "" {
		return nil, domain.New(domain.CodeInternal, "The database is not configured.")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "The database configuration is invalid.", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns >= 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	if cfg.ConnectTimeout > 0 {
		poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	}
	if cfg.PgBouncer {
		// Unnamed statements are prepared per execution, so they survive
		// PgBouncer transaction pooling where a server connection can change
		// between statements.
		poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	}
	poolCfg.MaxConnLifetime = poolMaxLifetime
	poolCfg.MaxConnLifetimeJitter = 5 * time.Minute
	poolCfg.MaxConnIdleTime = poolMaxIdleTime
	poolCfg.HealthCheckPeriod = poolHealthPeriod

	pingTimeout := cfg.PingTimeout
	if pingTimeout <= 0 {
		pingTimeout = defaultPingTimeout
	}
	poolCfg.PingTimeout = pingTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "The database pool could not be created.", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, domain.Wrap(domain.CodeInternal, "The database is unavailable.", err)
	}

	return &Pool{
		Pool:        pool,
		sqlDB:       stdlib.OpenDBFromPool(pool),
		pingTimeout: pingTimeout,
	}, nil
}

// DB exposes the database/sql handle used by goose. Closing it does not close
// the underlying pgx pool.
func (p *Pool) DB() *sql.DB {
	if p == nil {
		return nil
	}
	return p.sqlDB
}

// Ping verifies connectivity using the configured ping deadline.
func (p *Pool) Ping(ctx context.Context) error {
	if p == nil || p.Pool == nil {
		return domain.New(domain.CodeInternal, "The database is not configured.")
	}
	timeout := p.pingTimeout
	if timeout <= 0 {
		timeout = defaultPingTimeout
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := p.Pool.Ping(pingCtx); err != nil {
		return MapError(err)
	}
	return nil
}

// Close releases the SQL handle and the connection pool.
func (p *Pool) Close() {
	if p == nil {
		return
	}
	if p.sqlDB != nil {
		_ = p.sqlDB.Close()
	}
	if p.Pool != nil {
		p.Pool.Close()
	}
}

// WithTransaction executes fn inside a transaction. It rolls back on error or
// panic and re-panics after the rollback, so a panic cannot leak a checked-out
// connection or leave a half-written swap/event pair.
func WithTransaction(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) (err error) {
	if pool == nil {
		return domain.New(domain.CodeInternal, "The database is not configured.")
	}

	tx, beginErr := pool.Begin(ctx)
	if beginErr != nil {
		return MapError(beginErr)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			rollback(tx, ctx)
			panic(recovered)
		}
		if err != nil {
			rollback(tx, ctx)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return MapError(err)
	}
	return nil
}

// WithTransaction executes fn inside a transaction on this pool.
func (p *Pool) WithTransaction(ctx context.Context, fn func(pgx.Tx) error) error {
	if p == nil || p.Pool == nil {
		return domain.New(domain.CodeInternal, "The database is not configured.")
	}
	return WithTransaction(ctx, p.Pool, fn)
}

func rollback(tx pgx.Tx, ctx context.Context) {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackCtx)
}
