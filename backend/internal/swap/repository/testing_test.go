package repository_test

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/repository"
	"universal-defi-intelligence/migrations"
)

var testContainer *tcpostgres.PostgresContainer

func TestMain(m *testing.M) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		container, err := tcpostgres.Run(ctx,
			"postgres:16-alpine",
			tcpostgres.WithDatabase("udi_test"),
			tcpostgres.WithUsername("udi"),
			tcpostgres.WithPassword("udi"),
			tcpostgres.BasicWaitStrategies(),
		)
		cancel()
		if err != nil {
			log.Printf("testcontainers unavailable; PostgreSQL integration tests will be skipped: %v", err)
		} else {
			urlCtx, urlCancel := context.WithTimeout(context.Background(), 30*time.Second)
			url, urlErr := container.ConnectionString(urlCtx, "sslmode=disable")
			urlCancel()
			if urlErr != nil {
				log.Printf("read testcontainers connection string: %v", urlErr)
				terminateContainer(container)
			} else {
				if err := os.Setenv("TEST_DATABASE_URL", url); err != nil {
					log.Printf("set TEST_DATABASE_URL: %v", err)
					terminateContainer(container)
				} else {
					testContainer = container
				}
			}
		}
	}

	code := m.Run()
	if testContainer != nil {
		terminateContainer(testContainer)
	}
	os.Exit(code)
}

func terminateContainer(container *tcpostgres.PostgresContainer) {
	if container == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := container.Terminate(ctx); err != nil {
		log.Printf("terminate testcontainers postgres: %v", err)
	}
}

type testSchema struct {
	url      string
	schema   string
	pool     *pgxpool.Pool
	db       *sql.DB
	migrator *postgres.Migrator
}

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if url == "" {
		t.Skip("set TEST_DATABASE_URL or run Docker to enable PostgreSQL integration tests")
	}
	return url
}

func newTestSchema(t *testing.T, applyMigrations bool) *testSchema {
	t.Helper()
	url := testDatabaseURL(t)
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	schema := "udi_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE"); err != nil {
			t.Logf("drop test schema %s: %v", schema, err)
		}
		admin.Close()
	})

	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema
	poolCfg.MaxConns = 20
	poolCfg.MinConns = 0

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	t.Cleanup(pool.Close)

	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })

	migrator, err := postgres.NewMigrator(db, migrations.FS)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	t.Cleanup(func() { _ = migrator.Close() })

	ts := &testSchema{
		url:      url,
		schema:   schema,
		pool:     pool,
		db:       db,
		migrator: migrator,
	}
	if applyMigrations {
		if _, err := migrator.Up(ctx); err != nil {
			t.Fatalf("apply test migrations: %v", err)
		}
	}
	return ts
}

func newTestStore(t *testing.T) (*repository.Store, *testSchema) {
	t.Helper()
	ts := newTestSchema(t, true)
	return repository.New(ts.pool), ts
}
