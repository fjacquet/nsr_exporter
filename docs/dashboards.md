# Grafana Dashboards

`nsr_exporter` ships six domain-split Grafana dashboards. They are auto-provisioned
by the compose demo stack and can be imported manually into any Grafana instance that
has a Prometheus datasource pointed at the exporter.

## Dashboard overview

| Dashboard | UID | Focus |
|---|---|---|
| NetWorker — Overview | `nsr-overview` | System health, job status summary, server statistics, infrastructure snapshot |
| NetWorker — Capacity | `nsr-capacity` | Data Domain capacity/dedup, volume capacity and writes |
| NetWorker — Backups & Sizing | `nsr-backups` | FETB sizing, ingest throughput, retention, duration |
| NetWorker — Live Activity | `nsr-activity` | In-flight sessions, client inventory, alert detail |
| NetWorker — Devices & Media | `nsr-devices` | Device inventory, storage-node health, pool capacity, volume status |
| NetWorker — Protection & Compliance | `nsr-protection` | Policy status, client staleness, VMware integration, queue depth |

Each dashboard has a `system` template variable populated from
`label_values(nsr_up, system)` so you can filter to a single NetWorker system or view
all systems at once.

## nsr-overview — System Health

**Panels:**

- Systems Up — `nsr_up` per system (background color: green=UP, red=DOWN)
- Critical Alerts — `nsr_alerts_total{severity="Critical"}` per system
- Warning Alerts — `nsr_alerts_total{severity="Warning"}` per system
- Active Sessions — `sum(nsr_sessions_total)` per system
- Jobs by Completion Status — `nsr_job_status` grouped by `completion_status` (bargauge)
- Jobs by Type — `nsr_job_status` grouped by `job_type` (bargauge)
- Server statistics — `nsr_server_saves_total`, `nsr_server_bad_saves_total`, `nsr_server_save_size_bytes`, `nsr_server_recovers_total`
- **Stale Clients (>48h)** — `count(time() - nsr_client_last_backup_timestamp_seconds > 172800)` (yellow ≥1, red ≥5)
- **Offline Devices** — `count(nsr_device_info{status="offline"})` (red ≥1)
- **Disabled Policies** — `count(nsr_policy_enabled == 0)` (yellow ≥1)

## nsr-capacity — Storage Capacity

**Panels:**

- Data Domain Used % — gauge with 70%/85% thresholds per DD
- Data Domain Total Size — `nsr_datadomain_capacity_total_bytes`
- Data Domain Free Space — `nsr_datadomain_capacity_available_bytes` (red when near zero)
- Logical vs Physical Used — deduplication ratio visualisation (`nsr_datadomain_logical_capacity_used_bytes` vs `nsr_datadomain_capacity_used_bytes`)
- Data Domain Capacity Over Time — trend timeseries
- Volume Capacity by Pool — `nsr_volume_capacity_bytes` (bargauge)
- Volume Written Bytes — `nsr_volume_written_bytes` (bargauge)
- Volume Detail Table — capacity, written, recycles per volume with column overrides

## nsr-backups — Backups & Sizing

**Panels:**

- Largest Full Backup per Client/Saveset (FETB) — `nsr_backup_source_size_bytes` (bargauge, bytes)
- Largest Incremental Change — `nsr_backup_change_size_bytes` (bargauge, bytes)
- FETB Trend Over Time — full vs incremental size trend timeseries
- Ingest Throughput by Client — `avg(nsr_job_bytes_per_second)` in Bps (bargauge)
- Backup Duration by Client — `avg(nsr_job_duration_seconds)` in seconds with 1h/4h thresholds
- Retention Period by Pool — `avg(nsr_backup_retention_seconds)` in seconds
- Throughput Trend Over Time — timeseries in Bps

!!! note "Do not use `rate()` on per-second gauges"
    `nsr_job_bytes_per_second` is already an instantaneous per-second gauge — it is
    not a counter. Use `sum()` or `avg()` in PromQL, never `rate()`.

## nsr-activity — Live Activity

**Panels:**

- Total Active Sessions — `sum(nsr_sessions_total)` per system
- Sessions by Type — `nsr_sessions_total` by `session_type` (bargauge)
- Total Bytes In-Flight — `sum(nsr_session_bytes)` across active sessions
- Active Session Count Over Time — timeseries by session_type and system
- Bytes In-Flight Over Time — timeseries by session_type and system
- Client Parallelism — `nsr_client_parallelism` per client (bargauge)
- Client Inventory Table — `nsr_client_info` with NDMP, scheduled, backup-command columns
- Alerts by Severity — `nsr_alerts_total` bargauge with colour thresholds
- Alert Trend Over Time — timeseries by severity and system

## nsr-devices — Devices & Media

Added in the observability-expansion milestone. Covers the new device, storage-node,
pool, and volume-status metrics introduced in collectors C5–C7.

**Panels:**

- Devices by Status — `sum by (status) (nsr_device_info)` bargauge (status label: enabled/disabled/offline)
- Devices by Type — `sum by (type) (nsr_device_info)` bargauge (type label: tape/disk/adv_file)
- Total Device Capacity — `sum(nsr_device_capacity_bytes)` stat (bytes)
- Offline Device Count — `count(nsr_device_info{status="offline"})` stat (red ≥1)
- Device Inventory Table — `nsr_device_info` joined with `nsr_device_capacity_bytes`; columns: device_name, type, status, serial, capacity
- Storage Nodes by Status — `sum by (status) (nsr_storagenode_info)` bargauge (status label)
- Devices per Storage Node — `nsr_storagenode_device_count` bargauge (node label)
- Pool Used % — `nsr_pool_used_bytes / nsr_pool_capacity_bytes` gauge, thresholds 70%/85% (pool label)
- Pool Total Capacity — `nsr_pool_capacity_bytes` bargauge (pool label)
- Volumes per Pool — `nsr_pool_volume_count` bargauge (pool label)
- Volumes by Status — `sum by (status) (nsr_volume_status)` bargauge (status label)
- Volumes by Pool and Status — `sum by (pool, status) (nsr_volume_status)` bargauge

## nsr-protection — Protection & Compliance

Added in the observability-expansion milestone. Covers the new policy, group,
client-staleness, VMware, and queue metrics introduced in collectors C8–C10.

**Panels:**

- Enabled Policies — `count(nsr_policy_enabled == 1)` stat (policy label)
- Disabled Policies — `count(nsr_policy_enabled == 0)` stat (yellow ≥1)
- Total Clients Covered — `sum(nsr_policy_client_count)` stat (policy label)
- Stale Clients (>48h) — `count(time() - nsr_client_last_backup_timestamp_seconds > 172800)` stat (client_name label; yellow ≥1, red ≥5)
- Policy Enabled/Disabled Table — `nsr_policy_enabled` merged with `nsr_policy_client_count`; enabled rendered as ENABLED/DISABLED with colour mapping
- Clients per Protection Group — `nsr_group_client_count` bargauge (group, policy labels)
- Clients per Policy — `nsr_policy_client_count` bargauge (policy label)
- Hours Since Last Backup (Top 20) — `(time() - nsr_client_last_backup_timestamp_seconds) / 3600` topk(20) bargauge, unit=h, thresholds 24h/48h (client_name label)
- Client Backup Staleness Over Time — `avg by (system)` of hours-since trend timeseries
- VMware vCenter Status Table — `nsr_vmware_info`; columns: vcenter, version, connection_status (vcenter, version, status labels)
- VMware vCenters by Connection Status — `sum by (status) (nsr_vmware_info)` bargauge (status label)
- Queue Depth by Queue — `nsr_queue_depth` bargauge, thresholds 10/50 (queue label)
- Queue Wait Time by Queue — `nsr_queue_wait_seconds` bargauge, unit=s, thresholds 5m/15m (queue label)
- Queue Depth Over Time — timeseries trend
- Queue Wait Time Over Time — timeseries trend

!!! note "Do not use `rate()` on queue or staleness gauges"
    `nsr_queue_depth`, `nsr_queue_wait_seconds`, and `nsr_client_last_backup_timestamp_seconds`
    are all gauges. Aggregate with `sum()`, `avg()`, or `count()` in PromQL, never `rate()`.

## Dashboard files

Dashboards live under `grafana/dashboards/`:

```text
grafana/dashboards/
  nsr-overview.json
  nsr-capacity.json
  nsr-backups.json
  nsr-activity.json
  nsr-devices.json
  nsr-protection.json
```

The provisioning provider (`grafana/provisioning/dashboards/dashboards.yml`) loads
`/var/lib/grafana/dashboards` with `foldersFromFilesStructure: true` — subdirectories
become Grafana folders automatically.

## Manual import

To import into an existing Grafana instance:

1. Open Grafana → **Dashboards → Import**.
2. Upload the JSON file from `grafana/dashboards/`.
3. Select the Prometheus datasource that scrapes `nsr_exporter`.
4. Click **Import**.

Alternatively load all dashboards with `grafana-cli`:

```bash
for f in grafana/dashboards/*.json; do
  grafana-cli dashboards import "$f"
done
```
