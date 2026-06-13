// Package models holds the backend-agnostic data types shared across the exporter:
// the unified metric Sample emitted by collectors and the immutable Snapshot that
// both export paths read from.
package models

import "time"

// MetricType classifies a Sample so the export layer can map it to the right
// Prometheus value type and OTLP instrument.
type MetricType int

const (
	// Gauge is a value that can go up or down (capacities, counts, per-second rates).
	Gauge MetricType = iota
	// Counter is a monotonically increasing cumulative total.
	Counter
)

// Label is a single key/value dimension on a Sample. Collectors build these via
// the shared helpers in internal/nsr/metrics.go to keep label-key sets consistent.
type Label struct {
	Key   string
	Value string
}

// Sample is the unified metric representation every collector emits. The export
// layer (Prometheus unchecked collector, OTLP observable gauges) renders these;
// collectors never touch a Prometheus or OTLP type directly.
type Sample struct {
	Name   string
	Help   string
	Type   MetricType
	Labels []Label
	Value  float64
}

// Snapshot is an immutable point-in-time dump of every Sample collected across
// all configured systems in a single collection cycle. Once published to the
// SnapshotStore it is never mutated; a new cycle builds a fresh Snapshot.
type Snapshot struct {
	Samples   []Sample
	Collected time.Time
}
