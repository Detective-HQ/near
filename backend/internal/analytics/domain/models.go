// Package domain defines the domain models for analytics metrics, snapshots,
// and time series.
package domain

import (
	"time"
)

// Status represents the availability status of a metric observation or snapshot.
type Status string

const (
	StatusAvailable   Status = "available"
	StatusStale       Status = "stale"
	StatusPartial     Status = "partial"
	StatusUnavailable Status = "unavailable"
)

// MetricObservation is a single data point for a chain metric.
type MetricObservation struct {
	ID                 int64     `json:"id,omitempty"`
	MetricKey          string    `json:"metricKey"`
	ChainKey           string    `json:"chainKey"`
	Value              *float64  `json:"value"`
	Unit               string    `json:"unit"`
	SourceName         string    `json:"sourceName"`
	SourceURL          string    `json:"sourceUrl"`
	Status             Status    `json:"status"`
	MethodologyVersion string    `json:"methodologyVersion"`
	ObservedAt         time.Time `json:"observedAt"`
	RetrievedAt        time.Time `json:"retrievedAt"`
	Label              string    `json:"label,omitempty"`
	Definition         string    `json:"definition,omitempty"`
	Scope              string    `json:"scope,omitempty"`
	Error              string    `json:"error,omitempty"`
}

// SeriesPoint is a date-keyed value in a historical chart series.
type SeriesPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// ChainComparison compares a metric across different networks.
type ChainComparison struct {
	Chain          string  `json:"chain"`
	TVLUSD         float64 `json:"tvlUsd"`
	DailyVolumeUSD float64 `json:"dailyVolumeUsd"`
	Source         string  `json:"source"`
}

// Snapshot is a complete point-in-time collection of metrics for a chain.
type Snapshot struct {
	ID                 string              `json:"id"`
	Mode               string              `json:"mode"`
	Provider           string              `json:"provider"`
	Chain              string              `json:"chain"`
	Status             Status              `json:"status"`
	Notice             string              `json:"notice"`
	MethodologyVersion string              `json:"methodologyVersion"`
	CapturedAt         time.Time           `json:"capturedAt"`
	RetrievedAt        time.Time           `json:"retrievedAt"`
	Metrics            []MetricObservation `json:"metrics"`
	TVLSeries          []SeriesPoint       `json:"tvlSeries"`
	ChainComparisons   []ChainComparison   `json:"chainComparisons"`
	RawChecksum        string              `json:"rawChecksum,omitempty"`
}

// DataSnapshot stores immutable raw research/analytics inputs.
type DataSnapshot struct {
	ID              string    `json:"id"`
	SourceName      string    `json:"sourceName"`
	DatasetKey      string    `json:"datasetKey"`
	CapturedAt      time.Time `json:"capturedAt"`
	Checksum        string    `json:"checksum"`
	PayloadLocation string    `json:"payloadLocation"`
	CreatedAt       time.Time `json:"createdAt"`
}
