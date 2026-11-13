package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"universal-defi-intelligence/internal/analytics/domain"
)

type mockAnalyticsConnector struct {
	snapshot domain.Snapshot
	err      error
}

func (m *mockAnalyticsConnector) Name() string { return "mock_connector" }
func (m *mockAnalyticsConnector) Fetch(ctx context.Context, chain string) (domain.Snapshot, error) {
	return m.snapshot, m.err
}

type mockAnalyticsStore struct {
	savedObs  []domain.MetricObservation
	savedSnap domain.DataSnapshot
}

func (m *mockAnalyticsStore) SaveMetricObservations(ctx context.Context, obs []domain.MetricObservation) error {
	m.savedObs = append(m.savedObs, obs...)
	return nil
}
func (m *mockAnalyticsStore) GetLatestObservations(ctx context.Context, chain string) ([]domain.MetricObservation, error) {
	return m.savedObs, nil
}
func (m *mockAnalyticsStore) SaveDataSnapshot(ctx context.Context, snap domain.DataSnapshot) error {
	m.savedSnap = snap
	return nil
}
func (m *mockAnalyticsStore) GetDataSnapshot(ctx context.Context, id string) (domain.DataSnapshot, error) {
	return m.savedSnap, nil
}

func TestAnalyticsIngest(t *testing.T) {
	tmpDir := t.TempDir()

	val := 125000000.0
	mockSnap := domain.Snapshot{
		ID:         "snap_test_123",
		Provider:   "mock_connector",
		Chain:      "near",
		Status:     domain.StatusAvailable,
		CapturedAt: time.Now().UTC(),
		Metrics: []domain.MetricObservation{
			{
				MetricKey:  "tvl",
				ChainKey:   "near",
				Value:      &val,
				Unit:       "USD",
				SourceName: "mock_connector",
				Status:     domain.StatusAvailable,
			},
		},
	}

	mockStore := &mockAnalyticsStore{}
	svc := New(Config{
		Connector:  &mockAnalyticsConnector{snapshot: mockSnap},
		Store:      mockStore,
		StorageDir: tmpDir,
	})

	ingested, err := svc.Ingest(context.Background(), "near")
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	if ingested.RawChecksum == "" {
		t.Fatal("expected non-empty checksum")
	}

	// Verify file was written to disk
	expectedFile := filepath.Join(tmpDir, "snap_test_123.json")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Fatalf("snapshot file %s was not written: %v", expectedFile, err)
	}

	// Verify store was called
	if mockStore.savedSnap.ID != "snap_test_123" {
		t.Fatalf("saved snapshot ID %s != snap_test_123", mockStore.savedSnap.ID)
	}
	if len(mockStore.savedObs) != 1 {
		t.Fatalf("saved %d observations, want 1", len(mockStore.savedObs))
	}
}

