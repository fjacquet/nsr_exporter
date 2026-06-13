# Design Specification: nsr_exporter v0.2.0 — Observability Expansion

**Date**: June 13, 2026
**Status**: PROPOSED
**Builds on**: `2026-06-13-nsr_exporter-design.md` (the approved v0.1 contract) and the
`feat/observability-stack` work (compose + Prometheus + Grafana quickstart, PR #8).

> Goal of this milestone: **track materially more of the NetWorker REST surface**, make the
> **OTLP path coherent and demoable**, **document the remaining ADRs**, and **deepen the Grafana
> dashboards** so the new signals are actionable.

---

## 1. Goals / Non-goals

**Goals**
- Add 6 new resource collectors and extend 4 existing ones, surfacing device/media health,
  pool capacity, protection-policy coverage, VMware reachability, queue depth, and client
  backup-staleness.
- Refactor and complete the OTLP export path: remove dead code, make it configurable, wire it
  into the demo stack, and expose push-failure health.
- Write the 6 ADRs the v0.1 spec (§8) lists as planned.
- Add two Grafana dashboards + new alert rules covering the new signals.

**Non-goals**
- Live validation against a real NetWorker appliance (tracked separately — see §8 Risks). New
  collectors ship against **inferred** field names with tolerant parsing + mock fixtures.
- Live config hot-swap of clients (SIGHUP re-parse only — see ADR-0005).
- New auth modes, pagination frameworks, or non-`nsr_` metric families.

## 2. Conventions (apply to every new collector / metric)

- One file per domain implementing the existing `ResourceCollector` pattern; registered in
  `DefaultCollectors()`.
- Request only mapped fields with `fl=`; unwrap the named collection field; bound any
  large/unbounded endpoint.
- **Absent-never-zero** (ADR-0008): optional numeric fields are pointer-typed; an
  unparseable/missing value yields an absent sample, never `0`.
- Every metric carries `system="<name>"`; per-second values are gauges; unit-explicit names.
- **Label-key consistency** (ADR-0006): one label-key set per metric name; enforced by
  `TestCatalogCoversAllEmittedMetrics` + the new collectors' dual-export tests.
- Each metric registered in `catalog.go`; each collector covered by a `httptest` dual-export
  test (Prometheus gather **and** OTLP `ManualReader`); each new endpoint gets a `cmd/mocknw`
  fixture so the demo + tests populate.

---

## 3. ADRs (documentation only — content already in-code)

Write under `docs/adr/`, updating `index.md`. Sections: Status / Context / Decision / Consequences.

| ADR | Decision | Template |
|---|---|---|
| 0002 | Modular resource collectors (one file per domain; `DefaultCollectors()` composes) | 0001 |
| 0005 | Config hot reload: SIGHUP re-parse (client live-swap deferred) | 0001 |
| 0006 | Label-key consistency invariant (union label set for two-path metrics; test-enforced) | 0003 |
| 0008 | Absent-never-zero parsing (pointer types; no phony 0 on capacity/error metrics) | 0007 |
| 0009 | Metric naming & units (per-second = gauge; unit-explicit suffixes; `nsr_`/9097) | 0001 |
| 0011 | Supply-chain / release hardening (CGO off, non-root image, SBOM, SHA-pins, dependabot) | 0003 |

---

## 4. OTLP — full cleanup + demoable

1. **Remove dead code:** delete the empty `internal/telemetry/` package directory.
2. **Reposition transport:** move `otlp_grpc.go` from `package main` (repo root) into
   `internal/nsr/otlp_grpc.go` (`package nsr`), colocated with `otlp.go` so it is unit-testable.
3. **Configurable OTLP:** add an `opentelemetry:` block to the config:
   ```yaml
   opentelemetry:
     endpoint: ""          # e.g. otel-collector:4317; empty disables OTLP push
     pushInterval: "30s"
     insecure: true        # plaintext gRPC for in-cluster collectors
     headers: {}           # optional gRPC metadata
   ```
   - `config.go` parses it; `main.go` uses it instead of the hardcoded 30s + env-only endpoint.
   - `OTEL_EXPORTER_OTLP_ENDPOINT` remains an override (OTEL SDK convention); document precedence.
4. **Demo wiring:** set `opentelemetry.endpoint: "otel-collector:4317"` in `config.demo.yaml` so
   the bundled collector actually receives pushes; document the collector-exposed metrics on :8889.
5. **Health metric:** add `nsr_otlp_export_errors_total` (counter, on the Prometheus registry,
   incremented on push failure) so OTLP outages are alertable.

Dual-export test stays green; add a small test that the config block parses and defaults apply.

---

## 5. Metrics — field additions to existing collectors

| Collector | New `fl=` field(s) | New / changed metric |
|---|---|---|
| `clients.go` | `lastBackupTime`, `operatingSystem` | **`nsr_client_last_backup_timestamp_seconds{client_name}`** (gauge); add `operating_system` label to `nsr_client_info` |
| `storage.go` (volumes) | `status` | `nsr_volume_status{volume_name,pool,status}` (info gauge =1) |
| `jobs.go` | `startTime`, `endTime`, `group`, `level` | `nsr_job_start_timestamp_seconds`, `nsr_job_end_timestamp_seconds` (gauges); add `group`,`level` labels to `nsr_job_status` |
| `alerts.go` | `acknowledged` | add `acknowledged` label to `nsr_alert_info` |

Label-set changes to existing metrics are applied uniformly (every series gets the new key);
update fixtures + tests accordingly.

## 6. Metrics — new collectors

Each: response struct (wrapped field), `Get(ctx, path, QueryOpts{Fields}, &resp)`, map → samples,
catalog entries, `mocknw` fixture, dual-export test. Field names **inferred** — tolerant parsing.

| File | Endpoint | Metrics (gauge unless noted; all `+system`) |
|---|---|---|
| `devices.go` | `/devices` | `nsr_device_info{device_name,type,status,serial}=1`; `nsr_device_capacity_bytes{device_name}` |
| `storagenodes.go` | `/storagenodes` | `nsr_storagenode_info{node,status}=1`; `nsr_storagenode_device_count{node}` |
| `pools.go` | `/pools` | `nsr_pool_capacity_bytes{pool,type}`; `nsr_pool_used_bytes{pool}`; `nsr_pool_volume_count{pool}` |
| `vmware.go` | `/vmwares` | `nsr_vmware_info{vcenter,version,status}=1` |
| `queues.go` | `/queues` | `nsr_queue_depth{queue}`; `nsr_queue_wait_seconds{queue}` |
| `policies.go` | `/protectionpolicies`, `/protectiongroups` | `nsr_policy_enabled{policy}=0/1`; `nsr_policy_client_count{policy}`; `nsr_group_client_count{group,policy}` |

Note: `/protectionpolicies` + `/protectiongroups` are two endpoints in one collector file; if the
group→policy association requires correlating both payloads, do it in-process (no N+1 per cycle).

## 7. Grafana — go deeper

- **New dashboard `nsr-devices.json` (Devices & Media):** device status table, storage-node
  health, pool capacity gauges + used/total bars, volume status (full/appendable) heatmap.
- **New dashboard `nsr-protection.json` (Protection & Compliance):** policy enabled/disabled,
  per-policy client coverage, group membership, client backup-staleness (time since
  `nsr_client_last_backup_timestamp_seconds`), VMware vCenter status, queue depth/wait.
- **Extend `nsr-overview.json`:** add stat panels for stale clients (>48h), offline devices,
  disabled policies.
- **Alert rules** (`deploy/prometheus/nsr.rules.yml`): `NsrDeviceOffline`, `NsrStorageNodeDown`,
  `NsrClientBackupStale` (now − last_backup > 48h), `NsrQueueDepthHigh`, `NsrPolicyDisabled`,
  `NsrVCenterUnreachable`, `NsrPoolCapacityHigh`.
- Every panel uses the provisioned Prometheus datasource + the `system` template variable;
  per-second gauges aggregate with `sum`/`avg`.

## 8. Mock + tests

- Extend `cmd/mocknw` with wrapped fixtures for `/devices`, `/storagenodes`, `/pools`,
  `/vmwares`, `/queues`, `/protectionpolicies`, `/protectiongroups`, and the new fields on
  existing endpoints — so `docker compose up` populates the new dashboards.
- Each new collector + each field add gets a `httptest` dual-export assertion.
- `make ci` (fmt/vet/lint/test-race/vuln) + semgrep stay green; no inline suppressions.

---

## 9. Implementation phasing

Phased so each chunk is independently reviewable; phases A/B/C are largely parallelizable,
D depends on the metrics existing.

- **Phase A — ADRs** (6 docs + index). Independent.
- **Phase B — OTLP refactor** (delete dead dir, move transport, config block, demo wiring, error metric).
- **Phase C — Collectors** (4 field adds + 6 new collectors + mocknw fixtures + dual-export tests).
- **Phase D — Grafana + alerts** (2 dashboards + overview panels + rules). Depends on C's metric names.
- **Phase E — Docs** (`docs/metrics.md` catalog refresh; `docs/dashboards.md` for the new boards).

## 10. Testing strategy

- TDD per collector: fixture → dual-export test (Prometheus gather + OTLP `ManualReader`) → implement.
- `TestCatalogCoversAllEmittedMetrics` extended to cover every new metric (parity guard).
- Config test for the new `opentelemetry:` block (parse + defaults + env override precedence).
- Dashboard JSON validity (`jq empty`) + `docker compose config -q` in the review step.

## 11. Risks

- **Inferred field names (primary risk).** New endpoints/fields are modeled from API docs, not a
  live appliance. Mitigation: tolerant absent-never-zero parsing (a wrong field name yields an
  absent sample, not a crash or a phony 0); fixtures encode the assumed shapes; a single
  `--once --debug --trace` run against a real NetWorker validates them later. Each collector
  isolates its parsing so corrections are localized.
- **Label-set changes to existing metrics** (`nsr_client_info`, `nsr_job_status`,
  `nsr_alert_info`) are breaking for any existing dashboard query — apply uniformly and update
  the v0.1 dashboards in the same change.
- **Scope size.** Phased delivery (A–E) keeps each PR/review tractable.

---

## 12. Out of scope / future

`/schedules`, `/timepolicies`, `/probes`, `/notifications`, `/directives`, `/lockboxes` —
deferred (low metric value or high correlation complexity). Live-appliance field validation.
