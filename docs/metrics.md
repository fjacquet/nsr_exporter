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
| `nsr_alert_info` | Gauge (1) | `priority`, `category`, `message`, `timestamp` | An active alert; `priority` replaces former `severity`; `acknowledged` label removed; `timestamp` now populated |
| `nsr_alerts_active` | Gauge | `priority` | Count of active alerts by priority (renamed from `nsr_alerts_total`; `severity` label → `priority`) |

## Clients (`/clients`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_client_info` | Gauge (1) | `client_name`, `ndmp`, `scheduled_backup`, `backup_command`, `operating_system` | Configured client metadata |
| `nsr_client_parallelism` | Gauge | `client_name` | Configured backup stream limit (absent if unset — never 0) |

## Server & jobs (`/serverstatistics`, `/jobs`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_server_up_since_timestamp_seconds` | Gauge | — | Server start time (Unix seconds) |
| `nsr_server_saves_total` | Counter | — | Cumulative backup attempts |
| `nsr_server_save_size_bytes` | Counter | — | Cumulative bytes written by backups |
| `nsr_server_recovers_total` | Counter | — | Cumulative recovery attempts |
| `nsr_server_recover_size_bytes` | Counter | — | Cumulative bytes restored |
| `nsr_server_bad_saves_total` | Counter | — | Cumulative failed backups |
| `nsr_server_bad_recovers_total` | Counter | — | Cumulative failed recoveries |
| `nsr_server_current_saves` | Gauge | — | Current concurrent save operations |
| `nsr_server_current_recovers` | Gauge | — | Current concurrent recover operations |
| `nsr_server_max_saves` | Gauge | — | Configured maximum concurrent saves |
| `nsr_server_max_recovers` | Gauge | — | Configured maximum concurrent recovers |
| `nsr_job_status` | Gauge (1) | `job_id`, `job_name`, `job_type`, `state`, `completion_status`, `client`, `level` | An individual job; `group` label removed |
| `nsr_job_start_timestamp_seconds` | Gauge | `job_id`, `job_name` | Unix timestamp when the job started (absent if unparseable) |
| `nsr_job_end_timestamp_seconds` | Gauge | `job_id`, `job_name` | Unix timestamp when the job ended (absent if unparseable) |

## Live sessions (`/sessions`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_session_active` | Gauge (1) | `session_type`, `client` | An active session; `state` label removed; `session_type` values: Saving/Recovering/Browsing |
| `nsr_session_bytes` | Gauge | `session_type`, `client` | Bytes moved so far (absent if unknown); `state` label removed |
| `nsr_active_sessions` | Gauge | `session_type` | Count of active sessions by type (renamed from `nsr_sessions_total`; `session_type` values: Saving/Recovering/Browsing) |
| `nsr_session_transfer_bytes_per_second` | Gauge | `session_type`, `client` | Live transfer throughput — aggregate with `sum`/`avg`, **never `rate()`** |

## Storage & capacity (`/volumes`, `/datadomainsystems`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_volume_capacity_bytes` | Gauge | `volume_name`, `pool`, `type` | Volume capacity; `type` = media type (was `mediaType`) |
| `nsr_volume_written_bytes` | Gauge | `volume_name`, `pool`, `type` | Bytes written; `type` = media type (was `mediaType`) |
| `nsr_volume_recycled_total` | Counter | `volume_name` | Times recycled |
| `nsr_volume_status` | Gauge (1) | `volume_name`, `pool`, `status` | Volume status; `status` now holds values like Recyclable/WORM (was appendable/full) |
| `nsr_datadomain_capacity_total_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD total size |
| `nsr_datadomain_capacity_used_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD physical used |
| `nsr_datadomain_capacity_available_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD free |
| `nsr_datadomain_logical_capacity_used_bytes` | Gauge | `dd_name`, `model`, `os_version` | Pre-dedup logical used |

## Devices (`/devices`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_device_info` | Gauge (1) | `device_name`, `media_type`, `media_family`, `status`, `serial` | A backup device; `type` label renamed to `media_type`; NEW `media_family` label |

## Storage nodes (`/storagenodes`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_storagenode_info` | Gauge (1) | `node`, `enabled`, `type`, `version` | A NetWorker storage node; `status` label renamed to `enabled`; NEW `type` and `version` labels |
| `nsr_storagenode_device_count` | Gauge | `node` | Number of devices attached (absent if unknown) |

## Pools (`/pools`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_pool_info` | Gauge (1) | `pool`, `pool_type`, `enabled` | A media pool (replaces removed capacity metrics); `pool_type` and `enabled` labels |

## VMware vCenters (`/vmwares`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_vmware_info` | Gauge (1) | `vcenter`, `cloud_deployment` | A registered VMware vCenter; `version` and `status` labels removed; NEW `cloud_deployment` label |

## Protection policies and groups (`/protectionpolicies`, `/protectiongroups`)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_policy_enabled` | Gauge | `policy` | 1 if the protection policy is enabled, else 0 |
| `nsr_group_info` | Gauge (1) | `group`, `work_item_type` | A protection group (replaces removed `nsr_group_client_count`) |

## Sizing & capacity forecasting (bounded `/backups`)

The `/backups` query is **bounded** by `collection.backupWindow` (default 24h) — the
full catalog is never fetched (ADR-0010). The `q=` savetime syntax and `level`
vocabulary are INFERRED (see `sizing.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_backup_source_size_bytes` | Gauge | `client`, `saveset_name`, `level="Full"` | FETB — largest Full per saveset |
| `nsr_backup_change_size_bytes` | Gauge | `client`, `saveset_name`, `level="Incr"` | Largest incremental change |
| `nsr_backup_retention_seconds` | Gauge | `client`, `saveset_name`, `pool` | Retention period (retentionTime−saveTime) |
| `nsr_job_duration_seconds` | Gauge | `client`, `job_name` | Elapsed backup time (when present) |
| `nsr_job_bytes_per_second` | Gauge | `client`, `job_name` | Ingest throughput — aggregate with `sum`/`avg`, **never `rate()`** |
