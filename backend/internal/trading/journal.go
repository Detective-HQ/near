package trading

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// JournalEntry is a signal as it was published, with the plan used to grade it.
type JournalEntry struct {
	ID           string
	GeneratedAt  time.Time
	BarTime      time.Time
	AssetID      string
	Symbol       string
	Chain        string
	Direction    string
	Timeframe    string
	ModelVersion string
	Score        float64
	Entry        float64
	Stop         float64
	Target1      float64
	Target2      float64
}

// JournalFilter narrows a journal listing.
type JournalFilter struct {
	Asset     string
	Direction string
	Timeframe string
	From      time.Time // inclusive, zero = unbounded
	To        time.Time // exclusive, zero = unbounded
	Page      int
	PageSize  int
}

// Journal stores published signals. Entries are immutable; outcomes are
// computed on read from price data, never edited after the fact.
type Journal interface {
	Record(ctx context.Context, entries []JournalEntry) error
	List(ctx context.Context, filter JournalFilter) ([]JournalEntry, int, error)
}

func entryID(e JournalEntry) string {
	return fmt.Sprintf("%s|%s|%s|%d", e.AssetID, e.Timeframe, e.Direction, e.BarTime.Unix())
}

func (f *JournalFilter) normalize() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 25
	}
}

// MemoryJournal is an in-process journal bounded to maxJournal entries.
type MemoryJournal struct {
	mu      sync.RWMutex
	entries []JournalEntry
	ids     map[string]bool
}

const maxJournal = 5000

// NewMemoryJournal creates an empty journal.
func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{ids: map[string]bool{}}
}

// Record appends new entries, ignoring duplicates.
func (m *MemoryJournal) Record(_ context.Context, entries []JournalEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range entries {
		e.ID = entryID(e)
		if m.ids[e.ID] {
			continue
		}
		m.ids[e.ID] = true
		m.entries = append(m.entries, e)
	}
	if over := len(m.entries) - maxJournal; over > 0 {
		for _, e := range m.entries[:over] {
			delete(m.ids, e.ID)
		}
		m.entries = append([]JournalEntry(nil), m.entries[over:]...)
	}
	return nil
}

// List returns one page, newest first.
func (m *MemoryJournal) List(_ context.Context, f JournalFilter) ([]JournalEntry, int, error) {
	f.normalize()
	m.mu.RLock()
	matched := make([]JournalEntry, 0, len(m.entries))
	for _, e := range m.entries {
		if matchesFilter(e, f) {
			matched = append(matched, e)
		}
	}
	m.mu.RUnlock()
	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].GeneratedAt.Equal(matched[j].GeneratedAt) {
			return matched[i].GeneratedAt.After(matched[j].GeneratedAt)
		}
		return matched[i].ID > matched[j].ID
	})
	total := len(matched)
	start := (f.Page - 1) * f.PageSize
	if start >= total {
		return []JournalEntry{}, total, nil
	}
	end := start + f.PageSize
	if end > total {
		end = total
	}
	return matched[start:end], total, nil
}

func matchesFilter(e JournalEntry, f JournalFilter) bool {
	if f.Direction != "" && e.Direction != f.Direction {
		return false
	}
	if f.Timeframe != "" && e.Timeframe != f.Timeframe {
		return false
	}
	if !f.From.IsZero() && e.GeneratedAt.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && !e.GeneratedAt.Before(f.To) {
		return false
	}
	if f.Asset != "" {
		needle := strings.ToLower(f.Asset)
		if !strings.EqualFold(e.AssetID, f.Asset) && !strings.Contains(strings.ToLower(e.Symbol), needle) {
			return false
		}
	}
	return true
}

// PostgresJournal persists the journal in trading_signals.
type PostgresJournal struct {
	pool *pgxpool.Pool
}

// NewPostgresJournal creates a journal on an initialized pool.
func NewPostgresJournal(pool *pgxpool.Pool) *PostgresJournal {
	return &PostgresJournal{pool: pool}
}

// Record inserts entries; duplicates (same asset, timeframe, direction and
// bar) are ignored.
func (p *PostgresJournal) Record(ctx context.Context, entries []JournalEntry) error {
	if len(entries) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range entries {
		batch.Queue(`
			INSERT INTO trading_signals (
				id, generated_at, bar_time, asset_id, symbol, chain_key, direction,
				timeframe, model_version, score, entry_price, stop_price, target1_price, target2_price
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (id) DO NOTHING`,
			entryID(e), e.GeneratedAt.UTC(), e.BarTime.UTC(), e.AssetID, e.Symbol, e.Chain, e.Direction,
			e.Timeframe, e.ModelVersion, e.Score, e.Entry, e.Stop, e.Target1, e.Target2)
	}
	return p.pool.SendBatch(ctx, batch).Close()
}

// List returns one page, newest first, with the total match count.
func (p *PostgresJournal) List(ctx context.Context, f JournalFilter) ([]JournalEntry, int, error) {
	f.normalize()
	where := `WHERE ($1 = '' OR direction = $1) AND ($2 = '' OR timeframe = $2)
		AND ($3 = '' OR lower(asset_id) = lower($3) OR symbol ILIKE '%' || $3 || '%')
		AND ($4::timestamptz IS NULL OR generated_at >= $4) AND ($5::timestamptz IS NULL OR generated_at < $5)`
	var total int
	from, to := nullTime(f.From), nullTime(f.To)
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM trading_signals `+where,
		f.Direction, f.Timeframe, f.Asset, from, to).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT id, generated_at, bar_time, asset_id, symbol, chain_key, direction, timeframe,
			model_version, score, entry_price, stop_price, target1_price, target2_price
		FROM trading_signals `+where+`
		ORDER BY generated_at DESC, id DESC
		LIMIT $6 OFFSET $7`,
		f.Direction, f.Timeframe, f.Asset, from, to, f.PageSize, (f.Page-1)*f.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]JournalEntry, 0, f.PageSize)
	for rows.Next() {
		var e JournalEntry
		if err := rows.Scan(&e.ID, &e.GeneratedAt, &e.BarTime, &e.AssetID, &e.Symbol, &e.Chain, &e.Direction,
			&e.Timeframe, &e.ModelVersion, &e.Score, &e.Entry, &e.Stop, &e.Target1, &e.Target2); err != nil {
			return nil, 0, err
		}
		e.GeneratedAt, e.BarTime = e.GeneratedAt.UTC(), e.BarTime.UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, err
	}
	return out, total, nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}
