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
		{"nsr_server_up_since_timestamp_seconds", "NetWorker server start time (Unix seconds).", models.Gauge},
		{"nsr_server_saves_total", "Cumulative backup attempts.", models.Counter},
		{"nsr_server_save_size_bytes", "Cumulative bytes written by backups.", models.Counter},
		{"nsr_server_recovers_total", "Cumulative recovery attempts.", models.Counter},
		{"nsr_server_recover_size_bytes", "Cumulative bytes restored by recoveries.", models.Counter},
		{"nsr_server_bad_saves_total", "Cumulative failed backup attempts.", models.Counter},
		{"nsr_server_bad_recovers_total", "Cumulative failed recovery attempts.", models.Counter},
		{"nsr_job_status", "An individual NetWorker job (always 1).", models.Gauge},
		{"nsr_session_active", "An active NetWorker session (always 1).", models.Gauge},
		{"nsr_session_bytes", "Bytes moved so far by an active session.", models.Gauge},
		{"nsr_sessions_total", "Count of active sessions by type.", models.Gauge},
		{"nsr_volume_capacity_bytes", "Formatted volume capacity.", models.Gauge},
		{"nsr_volume_written_bytes", "Bytes written to the volume.", models.Gauge},
		{"nsr_volume_recycled_total", "Times the volume has been recycled.", models.Counter},
		{"nsr_datadomain_capacity_total_bytes", "Target Data Domain total size.", models.Gauge},
		{"nsr_datadomain_capacity_used_bytes", "Data Domain physical capacity used.", models.Gauge},
		{"nsr_datadomain_capacity_available_bytes", "Data Domain free space.", models.Gauge},
		{"nsr_datadomain_logical_capacity_used_bytes", "Pre-deduplication size stored on target.", models.Gauge},
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
