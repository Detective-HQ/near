// Package migrations embeds the versioned PostgreSQL schema migrations in the
// backend binary.
//
// The files use goose annotations (-- +goose Up / -- +goose Down) so each
// migration is reversible and runs inside a single transaction. Keeping the
// migrations embedded means the API, worker, and migrate command always ship
// the exact schema they expect; there is no runtime dependency on a migrations
// directory or a separately installed migration tool.
package migrations

import "embed"

// FS contains every *.sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
