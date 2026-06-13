package nsr

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/fjacquet/nsr_exporter/internal/models"
)

// PromCollector is an UNCHECKED prometheus.Collector: Describe sends nothing, so
// the metric-name set may vary between scrapes (different systems, dynamic labels).
// Collect reads the latest snapshot from the store — it never touches the backend.
type PromCollector struct {
	store *SnapshotStore
}

// NewPromCollector builds the Prometheus export path over the snapshot store.
func NewPromCollector(store *SnapshotStore) *PromCollector {
	return &PromCollector{store: store}
}

// Describe intentionally sends no descriptors, making this an unchecked collector.
func (*PromCollector) Describe(chan<- *prometheus.Desc) {}

// Collect renders every Sample in the current snapshot.
func (p *PromCollector) Collect(ch chan<- prometheus.Metric) {
	snap := p.store.Load()
	for _, s := range snap.Samples {
		keys, vals := splitLabels(s.Labels)
		valueType := prometheus.GaugeValue
		if s.Type == models.Counter {
			valueType = prometheus.CounterValue
		}
		desc := prometheus.NewDesc(s.Name, helpOrDefault(s), keys, nil)
		m, err := prometheus.NewConstMetric(desc, valueType, s.Value, vals...)
		if err != nil {
			// A malformed sample (e.g. label/value mismatch) is dropped rather than
			// crashing the scrape; the snapshot for other series still serves.
			continue
		}
		ch <- m
	}
}

func splitLabels(labels []models.Label) (keys, vals []string) {
	keys = make([]string, len(labels))
	vals = make([]string, len(labels))
	for i, l := range labels {
		keys[i] = l.Key
		vals[i] = l.Value
	}
	return keys, vals
}

func helpOrDefault(s models.Sample) string {
	if s.Help != "" {
		return s.Help
	}
	return s.Name
}
