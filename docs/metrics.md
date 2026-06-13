# Metrics catalog

Every metric carries `system="<name>"`. This catalog mirrors `internal/nsr/catalog.go`;
a test (`TestCatalogCoversAllEmittedMetrics`) fails if a collector emits a metric absent
here. Diff against a live appliance with:

```
nsr_exporter --config real.yaml --once --debug --trace 2>trace.log | sort > samples.txt
```

## Health

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_up` | Gauge | `system` | 1 if the system was reachable this cycle, else 0 |

## Alerts (`/alerts`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_alert_info` | Gauge (1) | `severity`, `category`, `message`, `timestamp` | An active alert |
| `nsr_alerts_total` | Gauge | `severity` | Count of active alerts by severity |

## Clients (`/clients`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_client_info` | Gauge (1) | `client_name`, `ndmp`, `scheduled_backup`, `backup_command` | Configured client metadata |
| `nsr_client_parallelism` | Gauge | `client_name` | Configured backup stream limit (absent if unset — never 0) |

## Planned (design spec §5, not yet implemented)

`nsr_server_*` + `nsr_job_status` (jobs.go) · `nsr_session_*` (sessions.go) ·
`nsr_volume_*` + `nsr_datadomain_*` (storage.go) · `nsr_backup_*` +
`nsr_job_bytes_per_second` + `nsr_job_duration_seconds` (sizing.go, bounded `/backups`).
