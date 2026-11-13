package connectors

import (
	"context"

	"universal-defi-intelligence/internal/analytics/domain"
)

// Connector defines the contract for fetching public chain metrics from an external source.
type Connector interface {
	// Name returns the identifier of the analytics source (e.g. "defillama").
	Name() string
	// Fetch gathers the latest metrics and history for the specified chain.
	Fetch(ctx context.Context, chain string) (domain.Snapshot, error)
}

