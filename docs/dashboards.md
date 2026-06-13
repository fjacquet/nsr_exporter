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
| NetWorker — Live Activity | `nsr-activity` | In-flight sessions, live throughput, client inventory, alert detail |
| NetWorker — Devices & Media | `nsr-devices` | Device inventory, storage-node health, pool inventory, volume status |
| NetWorker — Protection & Compliance | `nsr-protection` | Policy status, pool/group inventory, VMware integration |

Each dashboard has a `system` template variable populated from
`label_values(nsr_up, system)` so you can filter to a single NetWorker system or view
all systems at once.

## nsr-overview — System Health

**Panels:**

- Systems Up — `nsr_up` per system (background color: green=UP, red=DOWN)
- Critical Alerts — `nsr_alerts_active{priority="Critical"}` per system
- Warning Alerts — `nsr_alerts_active{priority="Warning"}` per system
- Active Sessions — `sum(nsr_active_sessions)` per system
- Jobs by Completion Status — `nsr_job_status` grouped by `completion_status` (bargauge)
- Jobs by Type — `nsr_job_status` grouped by `job_type` (bargauge)
- Server statistics — `nsr_server_saves_total`, `nsr_server_bad_saves_total`, `nsr_server_save_size_bytes`, `nsr_server_recovers_total`
- **Current Saves / Recovers** — `nsr_server_current_saves`, `nsr_server_current_recovers` (live concurrency gauges)
- **Server Max Saves / Recovers** — `nsr_server_max_saves`, `nsr_server_max_recovers` (configured limits)
- **Offline Devices** — `count(nsr_device_info{status="offline"})` (red ≥1)
- **Disabled Policies** — `count(nsr_policy_enabled == 0)` (yellow ≥1)

## nsr-capacity — Storage Capacity

**Panels:**

- Data Domain Used % — gauge with 70%/85% thresholds per DD
- Data Domain Total Size — `nsr_datadomain_capacity_total_bytes`
- Data Domain Free Space — `nsr_datadomain_capacity_available_bytes` (red when near zero)
- Logical vs Physical Used — deduplication ratio visualisation (`nsr_datadomain_logical_capacity_used_bytes` vs `nsr_datadomain_capacity_used_bytes`)
- Data Domain Capacity Over Time — trend timeseries
- Volume Capacity by Pool — `nsr_volume_capacity_bytes` (bargauge; `type` label = media type)
- Volume Written Bytes — `nsr_volume_written_bytes` (bargauge; `type` label = media type)
- Volume Detail Table — capacity, written, recycles per volume; `type` column renamed to "Media Type"

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

- Total Active Sessions — `sum(nsr_active_sessions)` per system (renamed from `nsr_sessions_total`)
- Sessions by Type — `nsr_active_sessions` by `session_type` bargauge (values: Saving/Recovering/Browsing)
- Total Bytes In-Flight — `sum(nsr_session_bytes)` across active sessions
- Active Session Count Over Time — timeseries by session_type and system
- Bytes In-Flight Over Time — timeseries by session_type and system
- **Live Transfer Throughput** — `sum(nsr_session_transfer_bytes_per_second)` by session_type and system (Bps timeseries; aggregate with sum/avg, never rate())
- Client Parallelism — `nsr_client_parallelism` per client (bargauge)
- Client Inventory Table — `nsr_client_info` with NDMP, scheduled, backup-command columns
- Alerts by Priority — `nsr_alerts_active` bargauge grouped by `priority` (replaces former `severity`)
- Alert Trend Over Time — timeseries by priority and system

## nsr-devices — Devices & Media

Added in the observability-expansion milestone. Covers device, storage-node, pool,
and volume-status metrics.

**Panels:**

- Devices by Status — `sum by (status) (nsr_device_info)` bargauge (status label)
- Devices by Media Type — `sum by (media_type) (nsr_device_info)` bargauge (`media_type` label; replaces former `type`)
- Offline Device Count — `count(nsr_device_info{status="offline"})` stat (red ≥1)
- Device Inventory Table — `nsr_device_info`; columns: device_name, media_type, media_family, status, serial (capacity column removed; `nsr_device_capacity_bytes` metric removed)
- Storage Nodes by Enabled State — `sum by (enabled) (nsr_storagenode_info)` bargauge (`enabled` label replaces former `status`)
- Devices per Storage Node — `nsr_storagenode_device_count` bargauge (node label)
- Pools by Type — `sum by (pool_type) (nsr_pool_info)` bargauge (info gauge =1; replaces removed `nsr_pool_capacity_bytes`/`nsr_pool_used_bytes`/`nsr_pool_volume_count`)
- Pool Inventory Table — `nsr_pool_info`; columns: pool, pool_type, enabled
- Volumes by Status — `sum by (status) (nsr_volume_status)` bargauge (status holds values like Recyclable/WORM)
- Volumes by Pool and Status — `sum by (pool, status) (nsr_volume_status)` bargauge

## nsr-protection — Protection & Compliance

Added in the observability-expansion milestone. Covers policy, group, and VMware metrics.

**Panels:**

- Enabled Policies — `count(nsr_policy_enabled == 1)` stat (policy label)
- Disabled Policies — `count(nsr_policy_enabled == 0)` stat (yellow ≥1)
- Pools by Type — `nsr_pool_info` table (pool, pool_type, enabled columns; replaces removed pool capacity panels)
- Policy Enabled/Disabled Table — `nsr_policy_enabled`; rendered as ENABLED/DISABLED with colour mapping (client count column removed; `nsr_policy_client_count` metric removed)
- Protection Groups by Work Item Type — `nsr_group_info` table (group, work_item_type columns; replaces removed `nsr_group_client_count` bargauge)
- VMware vCenter Inventory — `nsr_vmware_info` table; columns: vcenter, cloud_deployment (`version` and `status` labels removed; `cloud_deployment` label added)

!!! note "Removed metrics"
    The following metrics were removed and all references have been cleaned from these dashboards:
    `nsr_client_last_backup_timestamp_seconds`, `nsr_device_capacity_bytes`,
    `nsr_pool_capacity_bytes`, `nsr_pool_used_bytes`, `nsr_pool_volume_count`,
    `nsr_queue_depth`, `nsr_queue_wait_seconds`,
    `nsr_policy_client_count`, `nsr_group_client_count`.

!!! note "Renamed metrics"
    `nsr_alerts_total` → `nsr_alerts_active` (label `severity` → `priority`).
    `nsr_sessions_total` → `nsr_active_sessions` (session_type values: Saving/Recovering/Browsing).

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
