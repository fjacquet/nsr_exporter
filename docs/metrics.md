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

Field names are INFERRED (see `alerts.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_alert_info` | Gauge (1) | `severity`, `category`, `message`, `timestamp`, `acknowledged` | An active alert (C4: acknowledged label added) |
| `nsr_alerts_total` | Gauge | `severity` | Count of active alerts by severity |

## Clients (`/clients`)

Field names are INFERRED (see `clients.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_client_info` | Gauge (1) | `client_name`, `ndmp`, `scheduled_backup`, `backup_command`, `operating_system` | Configured client metadata (C1: operating_system label added) |
| `nsr_client_parallelism` | Gauge | `client_name` | Configured backup stream limit (absent if unset — never 0) |
| `nsr_client_last_backup_timestamp_seconds` | Gauge | `client_name` | Unix timestamp of most recent completed backup (absent if unparseable — C1) |

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
| `nsr_job_status` | Gauge (1) | `job_id`, `job_name`, `job_type`, `state`, `completion_status`, `client`, `group`, `level` | An individual job (C3: group and level labels added) |
| `nsr_job_start_timestamp_seconds` | Gauge | `job_id`, `job_name` | Unix timestamp when the job started (absent if unparseable — C3) |
| `nsr_job_end_timestamp_seconds` | Gauge | `job_id`, `job_name` | Unix timestamp when the job ended (absent if unparseable — C3) |

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
| `nsr_volume_status` | Gauge (1) | `volume_name`, `pool`, `status` | Volume status (appendable/full/recyclable — C2) |
| `nsr_datadomain_capacity_total_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD total size |
| `nsr_datadomain_capacity_used_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD physical used |
| `nsr_datadomain_capacity_available_bytes` | Gauge | `dd_name`, `model`, `os_version` | DD free |
| `nsr_datadomain_logical_capacity_used_bytes` | Gauge | `dd_name`, `model`, `os_version` | Pre-dedup logical used |

## Devices (`/devices`)

Field names are INFERRED (see `devices.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_device_info` | Gauge (1) | `device_name`, `type`, `status`, `serial` | A backup device (tape/disk/adv_file) |
| `nsr_device_capacity_bytes` | Gauge | `device_name` | Device storage capacity (absent if unknown) |

## Storage nodes (`/storagenodes`)

Field names are INFERRED (see `storagenodes.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_storagenode_info` | Gauge (1) | `node`, `status` | A NetWorker storage node |
| `nsr_storagenode_device_count` | Gauge | `node` | Number of devices attached (absent if unknown) |

## Pools (`/pools`)

Field names are INFERRED (see `pools.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_pool_capacity_bytes` | Gauge | `pool`, `type` | Total pool capacity in bytes (absent if unknown) |
| `nsr_pool_used_bytes` | Gauge | `pool` | Used pool capacity in bytes (absent if unknown) |
| `nsr_pool_volume_count` | Gauge | `pool` | Number of volumes in the pool (absent if unknown) |

## VMware vCenters (`/vmwares`)

Field names are INFERRED (see `vmware.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_vmware_info` | Gauge (1) | `vcenter`, `version`, `status` | A registered VMware vCenter (connected/disconnected) |

## Queues (`/queues`)

Field names are INFERRED (see `queues.go`) pending live validation.

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_queue_depth` | Gauge | `queue` | Number of pending items in the queue (absent if unknown) |
| `nsr_queue_wait_seconds` | Gauge | `queue` | Current wait time in seconds (absent if unknown) — aggregate with `avg`, never `rate()` |

## Protection policies and groups (`/protectionpolicies`, `/protectiongroups`)

Field names are INFERRED (see `policies.go`) pending live validation. Two `Get` calls in
one collector; group-to-policy correlation is done in-process (no N+1 per cycle).

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `nsr_policy_enabled` | Gauge | `policy` | 1 if the protection policy is enabled, else 0 |
| `nsr_policy_client_count` | Gauge | `policy` | Number of clients covered by this policy (absent if unknown) |
| `nsr_group_client_count` | Gauge | `group`, `policy` | Number of clients in this protection group (absent if unknown) |

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
