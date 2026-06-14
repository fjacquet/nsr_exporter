# Grafana Dashboard Refinement — Design

**Date:** 2026-06-14
**Status:** Approved (pending spec review)
**Scope:** Refine the 6 existing provisioned Grafana dashboards in place. No exporter
code changes, no new dashboards, no metric changes. JSON-only.

## Goal

Make the dashboards **crispy, pro, focused, and logical**: correct units and
thresholds everywhere, consistent semantic color palettes, an operator/NOC-first
Overview, and the high-value panels that are currently missing despite the metrics
already being emitted.

## Context

`grafana/dashboards/` ships 6 provisioned dashboards (72 panels):

| File | Title | Panels |
|------|-------|--------|
| `nsr-overview.json` | NetWorker — Overview | 18 |
| `nsr-activity.json` | NetWorker — Live Activity | 13 |
| `nsr-capacity.json` | NetWorker — Capacity | 10 |
| `nsr-backups.json` | NetWorker — Backups & Sizing | 9 |
| `nsr-devices.json` | NetWorker — Devices & Media | 14 |
| `nsr-protection.json` | NetWorker — Protection & Compliance | 9 |

All already use a templated `${datasource}` and `$system` variable. Datasource is
provisioned with uid `prometheus`. The dashboards are well-formed; this is genuine
refinement, not a rebuild.

### Verified metric/label ground truth (from `internal/nsr/` collectors)

New panels use only metrics confirmed emitted by the source:

- `nsr_job_end_timestamp_seconds{job_id, job_name}` — Unix seconds (freshness source)
- `nsr_server_up_since_timestamp_seconds` — Unix seconds (uptime source)
- `nsr_server_saves_total`, `nsr_server_bad_saves_total`, `nsr_server_save_size_bytes`,
  `nsr_server_recovers_total` — counters (use `increase()`)
- `nsr_datadomain_capacity_used_bytes{dd_name, model, os_version}`,
  `nsr_datadomain_capacity_total_bytes{...}` — gauges (forecast source)
- `nsr_job_status{job_id, job_name, job_type, state, completion_status, client, level}`
- `nsr_volume_status{volume_name, pool, status}` — **`status` is a comma-joined array**
  (e.g. `"Recyclable,WORM"`)
- `nsr_active_sessions{session_type}`, `nsr_session_bytes{session_type, client}`,
  `nsr_session_transfer_bytes_per_second{session_type, client}`

> Note: `nsr_job_end_timestamp_seconds` carries `job_id, job_name` but **not** `client`.
> Per-client freshness therefore joins job timing to client via `nsr_job_status`
> (which carries both `job_name`/`job_id` and `client`). The freshness query uses a
> `* on(job_id) group_left(client)` join — see Panel Specs below. If the join proves
> unreliable against live data, fall back to per-`job_name` freshness (no client roll-up).

## Global standards (apply to all 6 dashboards)

1. **Units.** Byte metrics → `bytes` (IEC). Per-second → `Bps`. Durations, uptime,
   age, retention → `s` (Grafana auto-scales to "3 weeks"/"1 year"). Percentages →
   `percent`. Ratios → `percentunit`. 1 decimal on byte values.
2. **Semantic threshold palettes** (consistent across dashboards):
   - up / health: `0`→red, `1`→green
   - alerts critical: `≥1`→red; alerts warning: `≥1`→amber, `0`→green
   - failure-rate %: `<1` green / `1–5` amber / `>5` red
   - DD used %: `<70` green / `70–85` amber / `>85` red
   - offline devices / failed jobs counts: `0` green, `≥1` amber/red
   - freshness ratio (see variable below): `<0.75` green / `0.75–1.0` amber / `>1.0` red
3. **Legends.** Time-series: table legend with `last` + `max`. Single-series stats:
   legend hidden. Status tables: sorted by severity descending.
4. **Polish.** "No data" renders empty (not 0 — matches the exporter's absent-not-zero
   invariant). Per-panel description tooltips, including a note on per-second panels
   that they are gauges (aggregate with `sum`/`avg`, never `rate()`). `1m` refresh.
   Consistent tag set per dashboard (unchanged from today).

## Staleness SLA variable

Add a **custom template variable `staleness_hours`** (default `24`) to dashboards that
use freshness (Overview; optionally Backups). Grafana cannot reference template
variables inside threshold steps, so the SLA is driven through the **query**, not the
threshold:

- **Freshness ratio** (for colored stats): `age / (3600 * ${staleness_hours})`, where
  `1.0` = exactly at SLA. Fixed thresholds: green `<0.75`, amber `0.75–1.0`, red `>1.0`.
- **Stale-client count** (breach count): `count(<age> > 3600 * ${staleness_hours})`.
- Human-readable tables still display **real age** (unit `s`) for legibility.

## Per-dashboard changes

### Overview → operator / NOC (flagship; re-rowed by "what needs action now")

- **Row 1 — Action needed:** Systems Up · Critical Alerts · Warning Alerts ·
  **🆕 Stalest backup age** (`time() - max by(system)(nsr_job_end_timestamp_seconds)`,
  colored by freshness ratio) · **🆕 Failure rate 24h**
  (`100 * increase(nsr_server_bad_saves_total[24h]) / clamp_min(increase(nsr_server_saves_total[24h]), 1)`) ·
  Active Sessions
- **Row 2 — Activity (24h):** replace the raw cumulative-counter stats with
  `increase(...[24h])`: Backups attempted · Backups failed · Bytes written
  (`increase(nsr_server_save_size_bytes[24h])`, unit bytes) · Recovers ·
  **🆕 Server uptime** (`time() - nsr_server_up_since_timestamp_seconds`, unit s)
- **Row 3 — Job status:** keep bargauges; add success/failure color mapping on
  `completion_status`
- **Row 4 — Infra snapshot:** Offline devices · Disabled policies (keep) ·
  **🆕 Stale-client count** (uses `staleness_hours`)
- **Row 5 — 🆕 Backup freshness table:** per-client real age since last job end,
  sorted descending, color-thresholded by the ratio

### Capacity

- **🆕 DD "days-to-full"** stat: `(nsr_datadomain_capacity_total_bytes - nsr_datadomain_capacity_used_bytes) / clamp_min(predict_linear(nsr_datadomain_capacity_used_bytes[6h], 86400) - nsr_datadomain_capacity_used_bytes, 1) ... ` rendered as days until full, amber/red thresholds (red when < 14 days). Exact PromQL finalized in implementation; uses `predict_linear` over a 6h window.
- Units/thresholds pass on the remaining capacity panels.

### Devices

- **Fix `nsr_volume_status` comma-array** with `label_replace` to normalize/collapse
  multi-state values so "Volumes by Status" buckets are meaningful.
- Add units; offline-device count gets a red `≥1` threshold.

### Backups / Activity / Protection

- Units + threshold + legend pass only. Retention rendered as duration. No structural
  change. (Optionally wire `staleness_hours` into Backups if a freshness panel is added
  there; default: leave freshness on Overview only.)

## Out of scope (YAGNI)

- No new dashboards, no merging/splitting existing ones.
- No exporter / metric / collector code changes.
- No alerting rules (dashboards only).
- No datasource/provisioning changes beyond what the dashboards reference.

## Testing & validation

- JSON validity: each file parses and provisions cleanly (no duplicate panel IDs,
  valid `gridPos`, every panel references `${datasource}`).
- Every new panel's PromQL references only confirmed-emitted metrics/labels.
- Manual smoke against the docker-compose + mocknw stack: dashboards load, panels
  render without "datasource not found" / "parse error", units display correctly.
- Threshold/color spot-check on the semantic palettes.
