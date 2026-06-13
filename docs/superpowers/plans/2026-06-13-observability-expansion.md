# Observability Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track materially more of the NetWorker REST surface, make OTLP coherent + demoable, document the remaining ADRs, and deepen the Grafana dashboards.

**Architecture:** Extend the existing decoupled snapshot collector. New domains follow the established `ResourceCollector` pattern (one file per domain in `internal/nsr/`, registered in `DefaultCollectors()`), with pointer-typed tolerant parsing (absent-never-zero) and dual-export tests (Prometheus gather + OTLP `ManualReader`). OTLP transport is repackaged and made config-driven.

**Tech Stack:** Go 1.26.4, `go-resty/resty/v2`, `prometheus/client_golang`, `go.opentelemetry.io/otel`, `httptest`, Grafana JSON + Prometheus rules.

**Spec:** `docs/superpowers/specs/2026-06-13-observability-expansion-design.md`

**Reference pattern files** (read before each collector task): `internal/nsr/clients.go` (collector shape), `internal/nsr/catalog.go` (metric registration), `internal/nsr/metrics.go` (sample/label builders), `internal/nsr/export_test.go` (dual-export test shape), `cmd/mocknw/main.go` (fixture shape), `internal/nsrclient/client.go` (`Get` + `QueryOpts`).

**Global rules for every task:** TDD (write the failing dual-export test first); absent-never-zero (pointer fields, no phony 0); every metric in `catalog.go`; `make ci` + semgrep stay green with **no** inline suppressions; one commit per task; conventional-commit messages ending with the `Co-Authored-By: Claude Opus 4.8 (1M context)` trailer.

---

## Phase A — ADRs (documentation; no tests)

Each task: write `docs/adr/NNNN-title.md` (sections Status/Context/Decision/Consequences), add a row to `docs/adr/index.md`, commit. Content source = spec §3 + the named sibling ADR as a structural template (read `~/Projects/ppdd_exporter/docs/adr/` for tone). Decisions are already implemented in-code — these record them.

- [ ] **Task A1 — ADR 0002 Modular resource collectors.** Decision: one file per domain implementing `ResourceCollector`; `DefaultCollectors()` composes them; adding a domain = one file + one catalog entry. Template: 0001. Commit `docs(adr): 0002 modular resource collectors`.
- [ ] **Task A2 — ADR 0005 Config hot reload.** Decision: SIGHUP re-parse (+ file-watch); live client swap deferred; rationale = MVP simplicity. Template: 0001.
- [ ] **Task A3 — ADR 0006 Label-key consistency invariant.** Decision: one label-key set per metric name; union set (canonical order, empty for inapplicable) for two-path metrics; enforced by `TestCatalogCoversAllEmittedMetrics` + label-parity. Template: 0003.
- [ ] **Task A4 — ADR 0008 Absent-never-zero parsing.** Decision: pointer types on optional numerics; unparseable/missing → absent sample, never 0; localized tolerant helpers; rationale = a phony 0 corrupts capacity/error dashboards + alerts. Template: 0007.
- [ ] **Task A5 — ADR 0009 Metric naming & units.** Decision: per-second values are gauges (sum/avg, never rate()); unit-explicit suffixes; `nsr_` prefix; port 9097. Template: 0001.
- [ ] **Task A6 — ADR 0011 Supply-chain / release hardening.** Decision: CGO off; multi-stage non-root image; CA-certs copied (no apk add); GoReleaser + CycloneDX SBOM; SHA-pinned actions; dependabot (actions+gomod+docker); `persist-credentials: false`. Template: 0003; content from v0.1 spec §9. Update `index.md` to mark all 10 ADRs present.

---

## Phase B — OTLP cleanup + demoable

- [ ] **Task B1 — Remove dead package.** Delete the empty `internal/telemetry/` directory. Run `go build ./...` (expect PASS). Commit `refactor(otlp): remove dead internal/telemetry package`.
- [ ] **Task B2 — Reposition transport.** `git mv otlp_grpc.go internal/nsr/otlp_grpc.go`; change its package clause `package main` → `package nsr`; update the caller in `main.go` (it currently calls `otlpGRPCExporter` from package main — move/rename so `main.go` calls `nsr.NewGRPCExporter(ctx, cfg)` or equivalent; keep the signature minimal). Run `go build ./... && go vet ./...` (PASS). Commit `refactor(otlp): move gRPC transport into internal/nsr`.
- [ ] **Task B3 — Config block (TDD).**
  - Write failing test in `internal/config/config_test.go`: load a YAML with an `opentelemetry:` block (`endpoint`, `pushInterval: 15s`, `insecure: true`) and assert the parsed struct; plus a defaults test (omitted block → `pushInterval` defaults `30s`, endpoint empty). Run → FAIL.
  - Add `OpenTelemetry struct { Endpoint string; PushInterval time.Duration; Insecure bool; Headers map[string]string }` to `internal/config/config.go` with the `30s` default and env override note (`OTEL_EXPORTER_OTLP_ENDPOINT` wins if set). Run → PASS.
  - Update `main.go` `setupOTLP()` to use `cfg.OpenTelemetry.PushInterval` (replace hardcoded 30s) and `cfg.OpenTelemetry.Endpoint` (fallback to env). Run `make test` (PASS). Commit `feat(otlp): configurable opentelemetry block`.
- [ ] **Task B4 — Push-error metric (TDD).** Add `nsr_otlp_export_errors_total` (Prometheus `CounterVec` or plain counter, labelled `system` if per-target else none) registered on the Prom registry; increment in the OTLP push-error path (where `log.Warn` currently fires). Write a test that forces an export error and asserts the counter increments (or, if hard to force, assert the counter is registered + zero-valued). Commit `feat(otlp): nsr_otlp_export_errors_total health metric`.
- [ ] **Task B5 — Demo wiring.** In `config.demo.yaml` add `opentelemetry:\n  endpoint: "otel-collector:4317"\n  insecure: true`; in `docs/deployment/docker.md` document the collector’s Prometheus endpoint on `:8889`. `docker compose -f docker-compose.yml config -q` (PASS). Commit `feat(otlp): wire demo compose to the bundled otel-collector`.

---

## Phase C — Collectors (field adds + new collectors)

### Field additions

- [ ] **Task C1 — clients: last-backup + OS (TDD).** Read `internal/nsr/clients.go`. Add `lastBackupTime`, `operatingSystem` to the `fl=` list and the response struct (pointer types). Emit `nsr_client_last_backup_timestamp_seconds{client_name,system}` (gauge; absent if unparseable RFC3339). Add `operating_system` label to `nsr_client_info` (every series gets it). Update `catalog.go`, the `mocknw` `/clients` fixture (add the two fields), and `export_test.go`. Test-first: extend the clients assertion to expect the new gauge + label. `make test` PASS. Commit.
- [ ] **Task C2 — volumes: status (TDD).** In `storage.go` add `status` to `/volumes` `fl=`/struct; emit `nsr_volume_status{volume_name,pool,status,system}=1`. Catalog + fixture + test. Commit.
- [ ] **Task C3 — jobs: timing + grouping (TDD).** In `jobs.go` add `startTime`,`endTime`,`group`,`level`; emit `nsr_job_start_timestamp_seconds` + `nsr_job_end_timestamp_seconds` (gauges, absent if unparseable); add `group`,`level` labels to `nsr_job_status` (uniform across all series). Catalog + fixture + test. Commit.
- [ ] **Task C4 — alerts: acknowledged (TDD).** In `alerts.go` add `acknowledged` to `fl=`/struct; add `acknowledged` label to `nsr_alert_info`. Catalog + fixture + test. Commit.

### New collectors

For **each** task below: (1) add a wrapped fixture to `cmd/mocknw/main.go` (and any `testdata`); (2) write a failing dual-export test mirroring `export_test.go` (assert via Prometheus gather **and** OTLP `ManualReader`); (3) implement `internal/nsr/<file>.go` mirroring `clients.go` — wrapped response struct (pointer numerics), `client.Get(ctx, "<endpoint>", nsrclient.QueryOpts{Fields: [...]}, &resp)`, map via the `metrics.go` builders, register in `catalog.go`, add to `DefaultCollectors()`; (4) `make test` PASS; (5) commit. Use exactly the metrics/labels/`fl=` from spec §6.

- [ ] **Task C5 — devices.go** `/devices` `fl=name,type,status,serialNumber,capacity` → `nsr_device_info{device_name,type,status,serial,system}=1`, `nsr_device_capacity_bytes{device_name,system}`.
- [ ] **Task C6 — storagenodes.go** `/storagenodes` `fl=name,status,deviceCount` → `nsr_storagenode_info{node,status,system}=1`, `nsr_storagenode_device_count{node,system}`.
- [ ] **Task C7 — pools.go** `/pools` `fl=name,type,capacityTotal,capacityUsed,volumeCount` → `nsr_pool_capacity_bytes{pool,type,system}`, `nsr_pool_used_bytes{pool,system}`, `nsr_pool_volume_count{pool,system}`.
- [ ] **Task C8 — vmware.go** `/vmwares` `fl=name,version,connectionStatus` → `nsr_vmware_info{vcenter,version,status,system}=1`.
- [ ] **Task C9 — queues.go** `/queues` `fl=name,depth,waitTime` → `nsr_queue_depth{queue,system}`, `nsr_queue_wait_seconds{queue,system}`.
- [ ] **Task C10 — policies.go** `/protectionpolicies` `fl=name,enabled,clientCount` + `/protectiongroups` `fl=name,policy,clientCount` (two `Get` calls in one collector; correlate in-process) → `nsr_policy_enabled{policy,system}=0/1`, `nsr_policy_client_count{policy,system}`, `nsr_group_client_count{group,policy,system}`.

---

## Phase D — Grafana + alerts (depends on Phase C metric names)

- [ ] **Task D1 — Devices & Media dashboard.** Create `grafana/dashboards/nsr-devices.json` (model JSON on `grafana/dashboards/nsr-capacity.json`): device status table (`nsr_device_info`), storage-node health, pool capacity/used bars (`nsr_pool_*`), volume status (`nsr_volume_status`). `system` template var via `label_values(nsr_up, system)`. Validate `jq empty`. Commit.
- [ ] **Task D2 — Protection & Compliance dashboard.** Create `grafana/dashboards/nsr-protection.json`: policy enabled/disabled (`nsr_policy_enabled`), per-policy client coverage, group membership, client staleness (`time() - nsr_client_last_backup_timestamp_seconds`), VMware status, queue depth/wait. Validate. Commit.
- [ ] **Task D3 — Extend Overview.** Add to `grafana/dashboards/nsr-overview.json` stat panels: stale clients `count(time()-nsr_client_last_backup_timestamp_seconds > 48*3600)`, offline devices, disabled policies. Validate. Commit.
- [ ] **Task D4 — Alert rules.** Extend `deploy/prometheus/nsr.rules.yml`: `NsrDeviceOffline`, `NsrStorageNodeDown`, `NsrClientBackupStale` (>48h), `NsrQueueDepthHigh`, `NsrPolicyDisabled` (`nsr_policy_enabled==0`), `NsrVCenterUnreachable`, `NsrPoolCapacityHigh` (used/total>0.85). Use real metric names. Commit.

---

## Phase E — Docs

- [ ] **Task E1 — Metrics catalog.** Refresh `docs/metrics.md` with every new metric (name, type, labels, source endpoint). Commit.
- [ ] **Task E2 — Dashboards doc.** Update `docs/dashboards.md` to describe the two new boards + new overview panels; ensure mkdocs nav covers them. Commit.

---

## Self-review

- **Spec coverage:** §3 ADRs → Phase A (A1–A6). §4 OTLP → Phase B (B1–B5, all 5 fixes). §5 field adds → C1–C4. §6 new collectors → C5–C10 (all 6). §7 Grafana+alerts → D1–D4. §8 mock+tests → folded into each C task. §10 testing → global rules + per-task TDD. No spec section is unmapped.
- **Type consistency:** `nsrclient.QueryOpts{Fields,Filter}` and `client.Get(ctx,path,opts,&out)` used consistently (matches `client.go`); metric names/labels match spec §5–§6 verbatim.
- **Placeholders:** none — every task names exact files, exact metrics/labels/`fl=`, exact test approach (dual-export), and a commit. Collector bodies intentionally reference the established `clients.go` pattern (codebase-pattern exception) rather than re-pasting near-identical Go.
