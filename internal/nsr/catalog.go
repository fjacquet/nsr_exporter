package nsr

import "github.com/fjacquet/nsr_exporter/internal/models"

// MetricMeta describes one metric the exporter can emit. The catalog is the single
// source of truth the OTLP path uses to pre-register observable instruments, and a
// test asserts every Sample a collector produces has a catalog entry (so the two
// export paths never drift).
type MetricMeta struct {
	Name string
	Help string
	Type models.MetricType
}

// Catalog lists every metric name the collectors emit. Add an entry here when a
// collector introduces a new metric.
func Catalog() []MetricMeta {
	return []MetricMeta{
		{"nsr_up", "1 if the system was reachable this cycle, else 0.", models.Gauge},
		{"nsr_alert_info", "An active NetWorker alert (always 1).", models.Gauge},
		{"nsr_alerts_total", "Count of active alerts by severity.", models.Gauge},
		{"nsr_client_info", "Metadata about a configured backup client (always 1).", models.Gauge},
		{"nsr_client_parallelism", "Configured backup stream limit per client.", models.Gauge},
	}
}

// catalogNames returns the set of known metric names for fast membership tests.
func catalogNames() map[string]struct{} {
	set := make(map[string]struct{})
	for _, m := range Catalog() {
		set[m.Name] = struct{}{}
	}
	return set
}
