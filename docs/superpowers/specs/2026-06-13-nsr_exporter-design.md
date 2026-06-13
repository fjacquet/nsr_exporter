# Design Specification: nsr_exporter

**Date**: June 13, 2026
**Status**: APPROVED (rev. 2 — 2026-06-13)
**Author**: Gemini CLI; rev. 2 validated against `dell/ansible-networker` REST collection + `exporter-standards` family
**Target Repository**: `github.com/fjacquet/nsr_exporter`

> **Rev. 2 changelog.** Validated every endpoint/auth claim against the working
> `dell/ansible-networker` collection. Added: native `fl=`/`q=` query projection, the
> `/backups` bounding constraint, wrapped-response decoding, **OTLP dual export**, the
> `/sessions` live-activity collector, the full Makefile/CI contract, the ADR set, and the
> family CLI/runtime surface (`--once --debug --trace`, hot reload, serve-before-collect,
> absent-never-zero parsing). These make it a conformant member of Fred's exporter family.

---

## 1. Executive Summary

`nsr_exporter` is a **Prometheus + OTLP** metric exporter written in **Go** that monitors
Dell EMC NetWorker backup servers. Following the decoupled architecture of `ppdd_exporter`,
it exposes a standard Prometheus `/metrics` endpoint **and** pushes the same snapshot over
OTLP.

A thread-safe, non-blocking background collection loop with a pointer-swapped snapshot cache
guarantees target NetWorker servers are never overwhelmed by scraping activity — backend API
load is decoupled from both the number of Prometheus scrapers and the OTLP push cadence. It
compiles to a standalone static binary (`CGO_ENABLED=0`), portable across Windows, Linux, and
macOS with no runtime installers.

---

## 2. Architecture & System Topology

Decoupled **collect-and-serve** model: one background loop polls every configured system,
builds an **immutable snapshot**, and atomically swaps it into a store that both export paths
read from. Neither `/metrics` scrapes nor OTLP pushes ever touch the NetWorker backend.

```
loop → fetch all systems (errgroup) → build immutable Snapshot → SnapshotStore.Swap()
                                                                    ├── PromCollector  (/metrics)
                                                                    └── OTLPExporter   (periodic push)
```

### Components

1. **Main Daemon (`main.go`)**: cobra CLI; loads config; **starts the HTTP server BEFORE the
   first collection cycle** (first poll can exceed the timeout and must not stall `/metrics`);
   spawns the collection loop; registers the Prometheus collector + OTLP reader; SIGHUP +
   file-watch config reload; graceful shutdown on SIGINT/SIGTERM.
2. **Snapshot Store (`internal/nsr/store.go`)**: thread-safe `RWMutex` pointer-swap of an
   **immutable** snapshot. `/metrics` responds in microseconds from cache.
3. **Collection Loop (`internal/nsr/collector.go`)**: ticker loop on `collection.interval`,
   fanning out across systems with `golang.org/x/sync/errgroup` (`SetLimit` caps concurrency).
   Per-system failure degrades gracefully (emits `nsr_up{system}=0`) rather than failing the cycle.
4. **Resource Collectors (`internal/nsr/*.go`)**: modular per-domain collectors (`alerts.go`,
   `clients.go`, `jobs.go`, `sessions.go`, `storage.go`, `sizing.go`) that call the client,
   **unwrap the named collection field, project only needed fields, tolerantly parse**, and emit
   a unified `Sample{Name, []Label, Value}`.
5. **Dual export (`internal/nsr/prometheus.go`, `otlp.go`)**: Prometheus **unchecked** collector
   (`Describe` sends nothing → dynamic metric set) and OTLP observable gauges driven by a periodic
   reader. Both read the same snapshot.
6. **NetWorker Client (`internal/nsrclient`)**: lean `go-resty/resty/v2` wrapper doing
   Basic-authenticated requests, TLS min 1.2, retry that **excludes 4xx**, and a credential-safe
   trace hook.

---

## 3. Configuration (`config.yaml`)

`config.yaml` is the source of truth. `${ENV_VAR}` refs expand in **host, username, AND
password** (fail-fast on unset); `passwordFile` supported for secrets. A `.env` is loaded at
startup as a quickstart convenience, never a replacement for `config.yaml`.

```yaml
---
server:
  host: "0.0.0.0"
  port: "9097"        # exporter's own /metrics port (NOT the NetWorker REST port)
  uri: "/metrics"
  logName: ""         # "" → stdout
collection:
  interval: "5m"      # background poll frequency
  timeout: "60s"      # per-system API deadline
systems:
  - name: nsr-prod-01
    host: "https://networker-prod-01.local:9090"   # 9090 = NetWorker REST API port
    username: "${NSR1_USERNAME}"
    password: "${NSR1_PASSWORD}"
    insecureSkipVerify: true
```

---

## 4. NetWorker REST Client & Authentication

Validated against `dell/ansible-networker` (`plugins/module_utils/nsrapi.py`,
`plugins/modules/clients.py:382,388`).

* **Base API path**: `https://<host>:9090/nwrestapi/v3/global`.
* **Auth**: HTTP **Basic** on every request — `auth = (username, password)`. No token/refresh
  dance (unlike Data Domain). **Deviation from the family bearer+refresh default → ADR-0007.**
* **TLS**: `insecureSkipVerify` per system for self-signed certs (the collection runs with
  `verify=False` universally). TLS min version 1.2 when verification is on.
* **Field projection (`fl=`)** — *use everywhere*. Endpoints accept
  `?fl=field1,field2` to return only requested fields. Each collector requests exactly the
  fields it maps. Massively reduces payload on `/clients` and `/backups`.
* **Server-side filter (`q=`)** — `?q=key:value and key2:value2`. Used to bound `/backups`
  (see §5.6) and to scope queries.
* **Response shape**: list endpoints return a **wrapped object**
  `{"count": N, "<resource>": [ ... ]}` (e.g. `{"clients":[…]}`), **not** a bare JSON array.
  Decoders MUST unwrap the named collection field.
* **Absent, never zero**: an unparseable/missing vendor value yields an **absent sample**, never
  a fake `0`. A phony 0 on a capacity/error metric silently corrupts dashboards and alerts.
  Tolerant types localized in one file. ADR-0008.

---

## 5. Metric Collectors & Mapping Schema

Every metric carries `system="<name>"`. All collectors send `fl=` projections and unwrap the
named collection field. Endpoints confirmed present in `ansible-networker`.

### 5.1. Alerts Collector (`alerts.go`)
* **Endpoint**: `GET /alerts?fl=severity,category,message,acknowledged,time`
* `nsr_alert_info` (Gauge, value `1.0`) — labels `severity`, `category`, `message`, `timestamp`
* `nsr_alerts_total` (Gauge) — labels `severity`

### 5.2. Clients Collector (`clients.go`)
* **Endpoint**: `GET /clients?fl=hostname,ndmp,scheduledBackup,backupCommand,parallelism`
* `nsr_client_info` (Gauge, `1.0`) — labels `client_name`, `ndmp`, `scheduled_backup`, `backup_command`
* `nsr_client_parallelism` (Gauge) — label `client_name`

### 5.3. Server Stats & Jobs Collector (`jobs.go`)
* **Endpoints**: `GET /serverstatistics`, `GET /jobs?fl=...`
* `nsr_server_up_since_timestamp_seconds` (Gauge)
* `nsr_server_saves_total`, `nsr_server_save_size_bytes`, `nsr_server_recovers_total`,
  `nsr_server_recover_size_bytes`, `nsr_server_bad_saves_total`, `nsr_server_bad_recovers_total` (Counters)
* `nsr_job_status` (Gauge) — labels `job_id`, `job_name`, `job_type`, `state`, `completion_status`, `client`

### 5.4. Live Sessions Collector (`sessions.go`) — **new in rev. 2**
* **Endpoint**: `GET /sessions` (live, in-flight backup/recover/clone activity — real-time
  signal `/jobs` can't give)
* `nsr_session_active` (Gauge, `1.0`) — labels `session_type`, `client`, `state`
* `nsr_session_bytes` (Gauge) — bytes moved so far by the active session — labels `session_type`, `client`
* `nsr_sessions_total` (Gauge) — count of active sessions — label `session_type`

### 5.5. Storage & Capacity Collector (`storage.go`)
* **Endpoints**: `GET /volumes?fl=...`, `GET /datadomainsystems?fl=...`
* `nsr_volume_capacity_bytes`, `nsr_volume_written_bytes` (Gauge) — labels `volume_name`, `pool`, `type`
* `nsr_volume_recycled_total` (Counter)
* `nsr_datadomain_capacity_total_bytes`, `nsr_datadomain_capacity_used_bytes`,
  `nsr_datadomain_capacity_available_bytes`, `nsr_datadomain_logical_capacity_used_bytes` (Gauge)
  — labels `dd_name`, `model`, `os_version`

### 5.6. Sizing & Capacity Forecaster Collector (`sizing.go`)
* **Endpoint**: `GET /backups?q=<bounded window>&fl=client,name,level,size,saveTime,retentionTime,pool`
* **⚠️ `/backups` is the full catalog — NEVER fetch it whole.** It can hold millions of
  savesets; an unbounded `5m` poll would hammer the server and exhaust memory — defeating the
  snapshot model. **Bound it** with `q=` (rolling `saveTime>` window, e.g. last interval +
  margin) and `fl=`. Aggregate FETB/change/retention in-process. ADR-0010.
* `nsr_backup_source_size_bytes` (Gauge) — FETB, largest **Full** per client/saveset — labels `client`, `saveset_name`, `level="Full"`
* `nsr_backup_change_size_bytes` (Gauge) — labels `client`, `saveset_name`, `level="Incr"`
* `nsr_backup_retention_seconds` (Gauge) — labels `client`, `saveset_name`, `pool`
* `nsr_job_bytes_per_second` (Gauge — per-second value, aggregate with `sum`/`avg`, **never `rate()`**) — labels `client`, `job_name`
* `nsr_job_duration_seconds` (Gauge)

### Naming & units (family invariants)
* Per-second values are **gauges**; aggregate with `sum`/`avg` in PromQL, never `rate()`.
* Unit-explicit names (`_bytes`, `_seconds`, `_bytes_per_second`).
* **Label-key consistency**: a metric name carries one label-key set across all its series; if a
  family is emitted by two paths, emit a union label set in fixed canonical order (empty values
  for inapplicable keys), enforced by a test. ADR-0006.

---

## 6. Testing & Offline Mock Server (`cmd/mocknw`)

* **TDD.** Mock NetWorker REST emulator `cmd/mocknw/main.go`: serves wrapped
  `{"<resource>":[…]}` payloads from `internal/nsr/testdata/`, validates Basic auth, honors
  `fl=`/`q=`. Unit tests use Go `httptest` for isolated parser/mapper assertions.
* **Assert via both export paths**: every collector test verifies output through **both** the
  Prometheus registry gather **and** an OTLP `ManualReader`.
* Label-parity test for any metric family emitted by two paths.
* **Semgrep runs on every file write via a hook and blocks on findings**; inline
  `// nosemgrep`/`//nolint` suppressions are not allowed — restructure instead.

---

## 7. CLI, runtime & operability (family surface)

* **Flags**: `--config`, `--debug`, `--once` (single cycle then exit), `--trace`.
* **`--once --debug`**: print every collected sample (sorted, exposition style) to diff against
  `docs/metrics.md` — catches silently-absent metrics that `_up` can't.
* **`--trace`**: log each API response as **method/path/status/body only** via a resty
  `OnAfterResponse` hook. **Never** use resty `SetDebug` — Basic auth puts base64 creds in the
  `Authorization` **request header**, which `SetDebug` would dump. Body-only is safe here
  (NetWorker responses carry no token).
* **`/health`** endpoint served off the snapshot; **serve HTTP before first collect**.
* **Hot reload**: SIGHUP + file-watch → rebuild-and-swap config.
* Validation recipe: `nsr_exporter --config real.yaml --once --debug --trace 2>trace.log | sort > samples.txt`.

---

## 8. Architecture Decision Records (`docs/adr/`)

`NNNN-title.md`, sections Status/Context/Decision/Consequences, with an `index.md`. Seed set
(reuse sibling ADRs as templates):

| ADR | Decision |
|---|---|
| 0001 | Snapshot collection model (immutable, RWMutex swap) — `ppdd` 0001 |
| 0002 | Modular resource collectors — `ppdd` 0002 |
| 0003 | **Client: hand-rolled `resty/v2`** (no NetWorker Go SDK exists → automatic) |
| 0005 | Config hot reload (SIGHUP + file-watch) — `ppdd` 0005 |
| 0006 | Label-key consistency invariant — `ppdd` 0006 |
| 0007 | **Auth: HTTP Basic** + retry excludes 4xx (deviates from family bearer+refresh) |
| 0008 | Defensive payload parsing: absent-never-zero — `obs` 0007 |
| 0009 | Metric naming & units (per-second = gauge) — `pstore` 0006 |
| 0010 | `/backups` bounding strategy (`q=` window + `fl=`) — nsr-specific |
| 0011 | Supply-chain / release hardening — `pflex` 0001 |

---

## 9. Developer Tooling & Build Command Standards

Full family Makefile contract — CI reproduces locally; everything CI runs is a Makefile target:

```
tools fmt-check fmt vet lint test test-race test-coverage vuln \
ci sure cli sbom release release-snapshot docker run-cli clean
```

* `make ci` — the gate: gofmt check, `go vet`, `golangci-lint`, `go test -race`, `govulncheck`.
* `make cli` — build `bin/nsr_exporter` with ldflags injecting `main.version`.
* `make sure` — local convenience: fmt + vet + test + build + lint.
* `make release` / `release-snapshot` — GoReleaser (CGO off, linux/darwin × amd64/arm64,
  cyclonedx-gomod SBOM, checksums, self-skipping Homebrew cask).
* CI trio `ci.yml` / `release.yml` / `docs.yml`; all actions SHA-pinned with `# vX.Y.Z`;
  `dependabot.yml` (actions+gomod+docker); `persist-credentials: false` on checkouts.
* Multi-stage Dockerfile, **non-root `USER`** (CI-enforced); copy CA certs from the builder
  (never `apk add ca-certificates`). Separate `Dockerfile.goreleaser` for the release image.

---

## 10. Bonus endpoints available for future collectors

`ansible-networker` also wraps these (ready to add as the app grows): `/pools`, `/devices`,
`/storagenodes`, `/protectionpolicies`, `/protectiongroups`, `/nasdevices`, `/schedules`,
`/timepolicies`, `/probes`, `/notifications`, `/directives`, `/vmwares`, `/queues`, `/lockboxes`.
