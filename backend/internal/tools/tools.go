//go:build tools

// Package tools pins the production dependency set declared in the roadmap.
//
// The HTTP, PostgreSQL, Redis, migration, UUID, and metrics packages are wired
// into their respective phases. Keeping them imported under the tools build
// tag means `go mod tidy` retains the pins while the application packages are
// being built incrementally.
package tools

import (
	_ "github.com/go-chi/chi/v5"
	_ "github.com/google/uuid"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/pressly/goose/v3"
	_ "github.com/prometheus/client_golang/prometheus"
	_ "github.com/redis/go-redis/v9"
)
