package migrations_test

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"universal-defi-intelligence/migrations"
)

var migrationFilePattern = regexp.MustCompile(`^([0-9]+)_[a-z0-9_]+\.sql$`)

func TestEmbeddedMigrationsAreSequentialAndReversible(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}

	versions := make([]int64, 0, len(entries))
	seen := make(map[int64]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			t.Fatalf("migration %q does not match NNNNN_name.sql", entry.Name())
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || version <= 0 {
			t.Fatalf("migration %q has an invalid version", entry.Name())
		}
		if previous, ok := seen[version]; ok {
			t.Fatalf("migrations %q and %q share version %d", previous, entry.Name(), version)
		}
		seen[version] = entry.Name()
		versions = append(versions, version)

		content, err := fs.ReadFile(migrations.FS, entry.Name())
		if err != nil {
			t.Fatalf("read migration %q: %v", entry.Name(), err)
		}
		text := string(content)
		if !strings.Contains(text, "-- +goose Up") {
			t.Fatalf("migration %q is missing -- +goose Up", entry.Name())
		}
		if !strings.Contains(text, "-- +goose Down") {
			t.Fatalf("migration %q is missing -- +goose Down", entry.Name())
		}
		if strings.Index(text, "-- +goose Up") > strings.Index(text, "-- +goose Down") {
			t.Fatalf("migration %q has Down before Up", entry.Name())
		}
	}

	if len(versions) == 0 {
		t.Fatal("no embedded migrations found")
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	for index, version := range versions {
		want := int64(index + 1)
		if version != want {
			t.Fatalf("migration versions are not contiguous: got %v, want 1..%d", versions, len(versions))
		}
	}

	if versions[len(versions)-1] != 8 {
		t.Fatalf("expected eight migrations, got %d", versions[len(versions)-1])
	}
	t.Logf("embedded migrations: %v", formatVersions(versions))
}

func formatVersions(versions []int64) string {
	parts := make([]string, len(versions))
	for index, version := range versions {
		parts[index] = fmt.Sprintf("%05d", version)
	}
	return strings.Join(parts, ", ")
}
