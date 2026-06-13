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

## Server & jobs (`/serverstatistics`, `/jobs`)

Field names are INFERRED (see `jobs.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_server_up_since_timestamp_seconds` | Gauge | — | Server start time (Unix seconds) |
| `nsr_server_saves_total` | Counter | — | Cumulative backup attempts |
| `nsr_server_save_size_bytes` | Counter | — | Cumulative bytes written by backups |
| `nsr_server_recovers_total` | Counter | — | Cumulative recovery attempts |
| `nsr_server_recover_size_bytes` | Counter | — | Cumulative bytes restored |
| `nsr_server_bad_saves_total` | Counter | — | Cumulative failed backups |
| `nsr_server_bad_recovers_total` | Counter | — | Cumulative failed recoveries |
| `nsr_job_status` | Gauge (1) | `job_id`, `job_name`, `job_type`, `state`, `completion_status`, `client` | An individual job |

## Live sessions (`/sessions`)

Field names are INFERRED (see `sessions.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_session_active` | Gauge (1) | `session_type`, `client`, `state` | An active session |
| `nsr_session_bytes` | Gauge | `session_type`, `client` | Bytes moved so far (absent if unknown) |
| `nsr_sessions_total` | Gauge | `session_type` | Count of active sessions by type |

## Storage & capacity (`/volumes`, `/datadomainsystems`)

Field names are INFERRED (see `storage.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_volume_capacity_bytes` | Gauge | `volume_name`, `pool`, `type` | Volume capacity |
| `nsr_volume_written_bytes` | Gauge | `volume_name`, `pool`, `type` | Bytes written |
| `nsr_volume_recycled_total` | Counter | `volume_name` | Times recycled |
| `nsr_datadomain_capacity_total_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD total size |
| `nsr_datadomain_capacity_used_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD physical used |
| `nsr_datadomain_capacity_available_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD free |
| `nsr_datadomain_logical_capacity_used_bytes` | Gauge | `dd_name`, `model`, `os_version` | Pre-dedup logical used |

## Planned (design spec §5, not yet implemented)

`nsr_backup_*` + `nsr_job_bytes_per_second` + `nsr_job_duration_seconds`
(sizing.go, bounded `/backups`).
