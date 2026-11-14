package trading

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresJournal runs against TEST_DATABASE_URL when it is set (the
// migrated database must contain trading_signals); otherwise it is skipped.
func TestPostgresJournal(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('trading_signals') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Skip("trading_signals table not migrated in this database")
	}
	_, _ = pool.Exec(ctx, `DELETE FROM trading_signals WHERE asset_id = 'test:journal'`)
	defer pool.Exec(ctx, `DELETE FROM trading_signals WHERE asset_id = 'test:journal'`)

	j := NewPostgresJournal(pool)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := JournalEntry{GeneratedAt: base, BarTime: base, AssetID: "test:journal", Symbol: "TST", Chain: "test",
		Direction: "bullish", Timeframe: "4h", ModelVersion: ModelVersion, Score: 55, Entry: 10, Stop: 9, Target1: 11.5, Target2: 13}
	if err := j.Record(ctx, []JournalEntry{e, e}); err != nil {
		t.Fatal(err)
	}
	e2 := e
	e2.Direction, e2.GeneratedAt = "bearish", base.Add(time.Hour)
	if err := j.Record(ctx, []JournalEntry{e2}); err != nil {
		t.Fatal(err)
	}
	items, total, err := j.List(ctx, JournalFilter{Asset: "test:journal", PageSize: 1})
	if err != nil || total != 2 || len(items) != 1 || items[0].Direction != "bearish" || items[0].Target2 != 13 {
		t.Fatalf("items=%+v total=%d err=%v", items, total, err)
	}
}

func TestPostgresJournalDateRange(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('trading_signals') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Skip("trading_signals table not migrated in this database")
	}
	_, _ = pool.Exec(ctx, `DELETE FROM trading_signals WHERE asset_id = 'test:range'`)
	defer pool.Exec(ctx, `DELETE FROM trading_signals WHERE asset_id = 'test:range'`)
	j := NewPostgresJournal(pool)
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		at := base.Add(time.Duration(i) * 24 * time.Hour)
		if err := j.Record(ctx, []JournalEntry{{GeneratedAt: at, BarTime: at, AssetID: "test:range", Symbol: "RNG",
			Chain: "test", Direction: "bullish", Timeframe: "4h", ModelVersion: ModelVersion}}); err != nil {
			t.Fatal(err)
		}
	}
	_, total, err := j.List(ctx, JournalFilter{Asset: "test:range", From: base.Add(24 * time.Hour), To: base.Add(48 * time.Hour)})
	if err != nil || total != 1 {
		t.Fatalf("total=%d err=%v", total, err)
	}
}
