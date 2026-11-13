package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/migrations"
)

func migrationDatabase(t *testing.T) (*postgres.Migrator, *sql.DB, *pgxpool.Pool) {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if url == "" {
		t.Skip("set TEST_DATABASE_URL or run Docker to enable PostgreSQL integration tests")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	schema := "udi_migrate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE"); err != nil {
			t.Logf("drop test schema %s: %v", schema, err)
		}
		admin.Close()
	})

	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("create migration pool: %v", err)
	}
	t.Cleanup(pool.Close)

	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })

	migrator, err := postgres.NewMigrator(db, migrations.FS)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	t.Cleanup(func() { _ = migrator.Close() })

	return migrator, db, pool
}

func TestMigratorRoundTrip(t *testing.T) {
	migrator, _, _ := migrationDatabase(t)
	ctx := context.Background()

	if got, want := migrator.MaxVersion(), int64(6); got != want {
		t.Fatalf("MaxVersion = %d, want %d", got, want)
	}
	if version, err := migrator.Version(ctx); err != nil {
		t.Fatalf("initial Version: %v", err)
	} else if version != 0 {
		t.Fatalf("initial Version = %d, want 0", version)
	}
	if pending, err := migrator.HasPending(ctx); err != nil {
		t.Fatalf("initial HasPending: %v", err)
	} else if !pending {
		t.Fatal("HasPending = false on an empty schema")
	}
	if err := migrator.Check(ctx); err == nil {
		t.Fatal("Check passed on an empty schema")
	}

	applied, err := migrator.Up(ctx)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if want := []int64{1, 2, 3, 4, 5, 6}; !equalVersions(applied, want) {
		t.Fatalf("Up applied %v, want %v", applied, want)
	}
	if version, err := migrator.Version(ctx); err != nil {
		t.Fatalf("Version after Up: %v", err)
	} else if version != migrator.MaxVersion() {
		t.Fatalf("Version after Up = %d, want %d", version, migrator.MaxVersion())
	}
	if pending, err := migrator.HasPending(ctx); err != nil {
		t.Fatalf("HasPending after Up: %v", err)
	} else if pending {
		t.Fatal("HasPending = true after Up")
	}
	if err := migrator.Check(ctx); err != nil {
		t.Fatalf("Check after Up: %v", err)
	}

	statuses, err := migrator.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statuses) != 7 {
		t.Fatalf("Status returned %d rows, want 7", len(statuses))
	}
	for _, status := range statuses {
		if status.State != "applied" {
			t.Fatalf("migration %d state = %q, want applied", status.Version, status.State)
		}
		if status.AppliedAt.IsZero() {
			t.Fatalf("migration %d has no applied timestamp", status.Version)
		}
	}
	if formatted := postgres.FormatStatus(statuses); !strings.Contains(formatted, "6\tapplied\t") {
		t.Fatalf("FormatStatus output missing applied row:\n%s", formatted)
	}

	version, err := migrator.Down(ctx)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if version != 6 {
		t.Fatalf("Down rolled back version %d, want 6", version)
	}

	rolledBack, err := migrator.DownTo(ctx, 0)
	if err != nil {
		t.Fatalf("DownTo(0): %v", err)
	}
	if want := []int64{5, 4, 3, 2, 1}; !equalVersions(rolledBack, want) {
		t.Fatalf("DownTo(0) rolled back %v, want %v", rolledBack, want)
	}
	if version, err := migrator.Version(ctx); err != nil {
		t.Fatalf("Version after rollback: %v", err)
	} else if version != 0 {
		t.Fatalf("Version after rollback = %d, want 0", version)
	}
}

func TestMigratorUpByOne(t *testing.T) {
	migrator, _, _ := migrationDatabase(t)
	ctx := context.Background()

	for want := int64(1); want <= 2; want++ {
		got, err := migrator.UpByOne(ctx)
		if err != nil {
			t.Fatalf("UpByOne: %v", err)
		}
		if got != want {
			t.Fatalf("UpByOne = %d, want %d", got, want)
		}
	}
	if version, _ := migrator.Version(ctx); version != 2 {
		t.Fatalf("Version = %d, want 2", version)
	}
}

func TestNewMigratorRejectsNilInputs(t *testing.T) {
	if _, err := postgres.NewMigrator(nil, migrations.FS); !domain.IsCode(err, domain.CodeInternal) {
		t.Fatalf("nil db error = %v, want INTERNAL", err)
	}

	_, db, _ := migrationDatabase(t)
	if _, err := postgres.NewMigrator(db, nil); !domain.IsCode(err, domain.CodeInternal) {
		t.Fatalf("nil fs error = %v, want INTERNAL", err)
	}
}

func TestFormatStatus(t *testing.T) {
	applied := time.Date(2026, 11, 13, 1, 2, 3, 0, time.UTC)
	output := postgres.FormatStatus([]postgres.MigrationStatus{
		{Version: 1, State: "applied", AppliedAt: applied},
		{Version: 2, State: "pending"},
	})

	want := "VERSION\tSTATE\tAPPLIED_AT\n" +
		"1\tapplied\t2026-11-13T01:02:03Z\n" +
		"2\tpending\t-\n"
	if output != want {
		t.Fatalf("FormatStatus =\n%q\nwant\n%q", output, want)
	}
}

func equalVersions(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
