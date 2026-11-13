// Package service coordinates metric ingestion, provenance hashing, and persistence.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"universal-defi-intelligence/internal/analytics/connectors"
	"universal-defi-intelligence/internal/analytics/domain"
)

// Store defines persistence operations for analytics data.
type Store interface {
	SaveMetricObservations(ctx context.Context, obs []domain.MetricObservation) error
	GetLatestObservations(ctx context.Context, chain string) ([]domain.MetricObservation, error)
	SaveDataSnapshot(ctx context.Context, snap domain.DataSnapshot) error
	GetDataSnapshot(ctx context.Context, id string) (domain.DataSnapshot, error)
}

// Config configures the analytics service.
type Config struct {
	Connector  connectors.Connector
	Store      Store
	StorageDir string
	Logger     *slog.Logger
	Now        func() time.Time
}

// Service manages the ingestion and retrieval of public chain analytics.
type Service struct {
	connector  connectors.Connector
	store      Store
	storageDir string
	logger     *slog.Logger
	now        func() time.Time
}

// New creates an initialized analytics ingestion service.
func New(cfg Config) *Service {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	storageDir := cfg.StorageDir
	if storageDir == "" {
		storageDir = filepath.Join("data", "snapshots")
	}

	return &Service{
		connector:  cfg.Connector,
		store:      cfg.Store,
		storageDir: storageDir,
		logger:     logger,
		now:        now,
	}
}

// Ingest collects metrics for a chain, persists the provenance snapshot, and saves observations.
func (s *Service) Ingest(ctx context.Context, chain string) (domain.Snapshot, error) {
	if s.connector == nil {
		return domain.Snapshot{}, fmt.Errorf("no analytics connector configured")
	}

	snapshot, err := s.connector.Fetch(ctx, chain)
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("connector fetch: %w", err)
	}

	// Canonical JSON serialization for checksum computation
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return snapshot, fmt.Errorf("marshal snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	snapshot.RawChecksum = checksum

	// Save to local filesystem storage
	if err := os.MkdirAll(s.storageDir, 0755); err == nil {
		filename := fmt.Sprintf("%s.json", snapshot.ID)
		filePath := filepath.Join(s.storageDir, filename)
		if writeErr := os.WriteFile(filePath, raw, 0644); writeErr != nil {
			s.logger.Warn("could not persist raw snapshot to disk", "path", filePath, "error", writeErr)
		} else {
			snapshotRecord := domain.DataSnapshot{
				ID:              snapshot.ID,
				SourceName:      s.connector.Name(),
				DatasetKey:      fmt.Sprintf("%s_overview", chain),
				CapturedAt:      snapshot.CapturedAt,
				Checksum:        checksum,
				PayloadLocation: filePath,
				CreatedAt:       s.now(),
			}
			if s.store != nil {
				_ = s.store.SaveDataSnapshot(ctx, snapshotRecord)
			}
		}
	}

	// Persist metric observations in PostgreSQL
	if s.store != nil && len(snapshot.Metrics) > 0 {
		if saveErr := s.store.SaveMetricObservations(ctx, snapshot.Metrics); saveErr != nil {
			s.logger.Warn("could not save metric observations", "error", saveErr)
		}
	}

	return snapshot, nil
}

