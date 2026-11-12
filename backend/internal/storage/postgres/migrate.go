package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"github.com/pressly/goose/v3"

	"universal-defi-intelligence/internal/swap/domain"
)

// MigrationStatus is a driver-neutral view of one embedded migration.
type MigrationStatus struct {
	Version   int64
	State     string
	AppliedAt time.Time
}

// Migrator applies the embedded, reversible SQL migrations with goose. goose
// wraps each SQL migration in its own transaction, so a failed migration never
// leaves the schema partially applied.
type Migrator struct {
	provider *goose.Provider
}

// NewMigrator creates a migration runner for the supplied database and
// filesystem. Callers normally pass migrations.FS.
func NewMigrator(db *sql.DB, fsys fs.FS) (*Migrator, error) {
	if db == nil {
		return nil, domain.New(domain.CodeInternal, "The migration database handle is nil.")
	}
	if fsys == nil {
		return nil, domain.New(domain.CodeInternal, "The migration filesystem is nil.")
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		fsys,
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "The migrations could not be loaded.", err)
	}
	return &Migrator{provider: provider}, nil
}

// Up applies every pending migration and returns the applied versions.
func (m *Migrator) Up(ctx context.Context) ([]int64, error) {
	results, err := m.provider.Up(ctx)
	if err != nil {
		return nil, MapError(err)
	}
	return versions(results), nil
}

// UpByOne applies the next pending migration and returns its version. It
// returns 0 when there is nothing to apply.
func (m *Migrator) UpByOne(ctx context.Context) (int64, error) {
	result, err := m.provider.UpByOne(ctx)
	if err != nil {
		return 0, MapError(err)
	}
	if result == nil || result.Source == nil {
		return 0, nil
	}
	return result.Source.Version, nil
}

// Down rolls back exactly one migration and returns its version. It returns 0
// when the database has no applied migrations.
func (m *Migrator) Down(ctx context.Context) (int64, error) {
	result, err := m.provider.Down(ctx)
	if err != nil {
		return 0, MapError(err)
	}
	if result == nil || result.Source == nil {
		return 0, nil
	}
	return result.Source.Version, nil
}

// DownTo rolls back all migrations after the target version.
func (m *Migrator) DownTo(ctx context.Context, version int64) ([]int64, error) {
	results, err := m.provider.DownTo(ctx, version)
	if err != nil {
		return nil, MapError(err)
	}
	return versions(results), nil
}

// Version returns the current schema version, or 0 when nothing is applied.
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	version, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, MapError(err)
	}
	return version, nil
}

// Status lists every embedded migration with its applied/pending state.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, MapError(err)
	}

	out := make([]MigrationStatus, 0, len(statuses))
	for _, status := range statuses {
		if status == nil || status.Source == nil {
			continue
		}
		out = append(out, MigrationStatus{
			Version:   status.Source.Version,
			State:     string(status.State),
			AppliedAt: status.AppliedAt,
		})
	}
	return out, nil
}

// HasPending reports whether the database is missing one or more migrations.
func (m *Migrator) HasPending(ctx context.Context) (bool, error) {
	pending, err := m.provider.HasPending(ctx)
	if err != nil {
		return false, MapError(err)
	}
	return pending, nil
}

// MaxVersion returns the highest embedded migration version.
func (m *Migrator) MaxVersion() int64 {
	var maxVersion int64
	for _, source := range m.provider.ListSources() {
		if source != nil && source.Version > maxVersion {
			maxVersion = source.Version
		}
	}
	return maxVersion
}

// Check verifies the database is exactly at the embedded target version. It is
// intended for CI drift checks: a pending migration, a failed migration, or a
// database version newer than the binary is an error.
func (m *Migrator) Check(ctx context.Context) error {
	current, target, err := m.provider.GetVersions(ctx)
	if err != nil {
		return MapError(err)
	}
	if current != target {
		return domain.Newf(
			domain.CodeInternal,
			"Database migration version %d does not match target %d.",
			current,
			target,
		)
	}
	if target != m.MaxVersion() {
		return domain.Newf(
			domain.CodeInternal,
			"Database migration target %d does not match embedded maximum %d.",
			target,
			m.MaxVersion(),
		)
	}
	return nil
}

// Close releases the goose provider. It does not close the database handle.
func (m *Migrator) Close() error {
	if m == nil || m.provider == nil {
		return nil
	}
	return m.provider.Close()
}

func versions(results []*goose.MigrationResult) []int64 {
	out := make([]int64, 0, len(results))
	for _, result := range results {
		if result == nil || result.Source == nil {
			continue
		}
		out = append(out, result.Source.Version)
	}
	return out
}

// FormatStatus renders a migration status list for the migrate CLI.
func FormatStatus(statuses []MigrationStatus) string {
	output := "VERSION\tSTATE\tAPPLIED_AT\n"
	for _, status := range statuses {
		appliedAt := "-"
		if !status.AppliedAt.IsZero() {
			appliedAt = status.AppliedAt.UTC().Format(time.RFC3339)
		}
		output += fmt.Sprintf("%d\t%s\t%s\n", status.Version, status.State, appliedAt)
	}
	return output
}
