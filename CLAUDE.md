# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status: greenfield

There is **no Go code yet** — no `go.mod`, no `Makefile`, no `main.go`. The repo currently
holds only an approved design spec and reference material:

- `docs/superpowers/specs/2026-06-13-nsr_exporter-design.md` — **the approved design contract.** Read it first.
- `docs/export.md`, `docs/sizing.md`, `samples/extract.sh` — NetWorker CLI/reporting background (`gstclreport`, `mminfo`) feeding the sizing collector.
- `docs/*.pdf` — NetWorker 19.x REST API + reference guides.

When scaffolding, this repo MUST conform to **Fred's exporter-standards family** (invoke the
`exporter-standards` skill). `nsr_exporter` is a new member modeled on `ppdd_exporter`
(hand-rolled REST client, snapshot model). Follow the new-exporter checklist in that skill;
the notes below capture where NetWorker diverges from the family defaults and where the
design spec is thinner than the family standard.

## What this is

A Prometheus + OTLP exporter for **Dell EMC NetWorker** backup servers. A single process
polls every configured NetWorker system on a background interval, caches an immutable
snapshot, and serves `/metrics` instantly from cache so scrapes never hit the backend.
Metric prefix `nsr_`, default port `9447`, every metric carries a `system="<name>"` label.

## Commands (target contract — build these)

The spec lists only `make test / cli / ci`. The family contract is larger and CI must
reproduce locally (everything CI runs is a Makefile target):

```
tools fmt-check fmt vet lint test test-race test-coverage vuln \
ci sure cli sbom release release-snapshot docker run-cli clean
```

- `make ci` — the gate: gofmt check, `go vet`, `golangci-lint`, `go test -race`, `govulncheck`.
- `make cli` — build `bin/nsr_exporter` with ldflags injecting `main.version`.
- `make sure` — local convenience: fmt + vet + test + build + lint.
- Single test: `go test ./internal/nsr/ -run TestName -v` (use `httptest`; see mock below).
- Live validation: `bin/nsr_exporter --config real.yaml --once --debug --trace 2>trace.log | sort > samples.txt`.

## Architecture (decoupled collect-and-serve)

```
collection loop → fetch all systems → build immutable Snapshot → SnapshotStore.Swap()
                                                                    ├── PromCollector (/metrics)
                                                                    └── OTLPExporter (push)
```

- `main.go` — cobra CLI (`--config --debug --once --trace`). **Start the HTTP server BEFORE the first collection cycle** — first poll can exceed the timeout and would otherwise stall `/metrics`. SIGHUP + file-watch config reload.
- `internal/nsr/store.go` — `SnapshotStore`, RWMutex pointer-swap of an **immutable** snapshot. Both export paths read the latest snapshot; never fetch on scrape.
- `internal/nsr/collector.go` — ticker loop, fans out across systems (`errgroup` with `SetLimit`).
- `internal/nsr/resource.go` + per-collector files (`alerts.go`, `clients.go`, `jobs.go`, `sessions.go`, `storage.go`, `sizing.go`) — call the client, **unwrap the named collection field, project fields with `fl=`, tolerantly parse**, map to samples.
- `internal/nsrclient/` — Resty client wrapper (Basic auth, TLS min 1.2, retry excludes 4xx, body-only trace hook).
- `internal/{models,config,logging,telemetry}` — support packages.
- `cmd/mocknw/main.go` — offline mock NetWorker REST emulator (Basic-auth validated, serves fixtures from `internal/nsr/testdata/`) for local dev without a real appliance.

## Load-bearing constraints (don't regress)

- **Dual export is mandatory.** The design spec only describes the Prometheus path — you MUST also wire OTLP (observable gauges + periodic reader reading the same snapshot). Collector tests assert via **both** the Prometheus registry gather **and** an OTLP `ManualReader`.
- **Auth is the NetWorker exception.** Family default is bearer + token-refresh; NetWorker uses **HTTP Basic auth directly** on every request (`auth=(user,pass)`) against base path `https://<host>:9090/nwrestapi/v3/global`. No token dance. ADR-0007. Retry must still **exclude 4xx**. (Verified against `dell/ansible-networker`: `plugins/modules/clients.py:382,388`.)
- **Use NetWorker's native query params.** Every list endpoint accepts `?fl=field1,field2` (field projection — request only what you map) and `?q=key:value and key2:value2` (server-side filter). Send `fl=` everywhere to trim payloads.
- **List responses are wrapped, not bare arrays.** NetWorker returns `{"count":N,"<resource>":[…]}` (e.g. `{"clients":[…]}`). Decoders must unwrap the named field — the spec's "decode JSON arrays" is imprecise.
- **`/backups` is the full catalog — NEVER fetch it whole.** `sizing.go` MUST bound it with a `q=` rolling `saveTime>` window + `fl=`, else an unbounded `5m` poll hammers the server and exhausts memory (defeating the snapshot model). ADR-0010.
- **Per-second values are gauges** (throughput, ingest rate) — aggregate with `sum`/`avg` in PromQL, never `rate()`. Be unit-explicit in metric names (`_bytes`, `_seconds`, `_bytes_per_second`).
- **Absent, never zero.** An unparseable vendor value yields an absent sample, not a fake `0` — a phony 0 on a capacity/error metric silently corrupts dashboards and alerts. Localize tolerant parsing in one place.
- **Label-key consistency invariant.** A metric name carries one label-key set across all its series; if a family is emitted by two paths, emit a union label set in fixed canonical order (empty values for inapplicable keys) and enforce with a test.
- **Per-target graceful degradation** — one system failing emits an `nsr_up{system}=0`-style gauge rather than failing the whole cycle.
- **`--trace` must not leak credentials.** Hand-roll an `OnAfterResponse` hook logging method/path/status/body only; never use resty `SetDebug` (it dumps the `Authorization` header).

## Config

`config.yaml` is the source of truth. `${ENV_VAR}` expansion in **host, username, AND password**
(fail-fast on unset); also support `passwordFile`. `.env` is a quickstart convenience loaded at
startup, never a replacement for `config.yaml`. Shape (from the spec):

```yaml
server: { host, port: "9447", uri: /metrics, logName }
collection: { interval: 5m, timeout: 60s }
systems:
  - { name, host, username: ${NSR1_USERNAME}, password: ${NSR1_PASSWORD}, insecureSkipVerify }
```

## Stack

Go `1.26.4` (patch-pinned). `go-resty/resty/v2`, `prometheus/client_golang` (unchecked
collector), `go.opentelemetry.io/otel`, `gopkg.in/yaml.v2`, `joho/godotenv`, `spf13/cobra`,
`sirupsen/logrus`, `golang.org/x/sync/errgroup`. CGO off for release. Multi-stage Dockerfile
with a **non-root `USER`** (CI-enforced); copy CA certs from the builder, don't `apk add` them.

## Testing & security

- TDD. Mock the backend with `httptest` driven by fixtures under `internal/nsr/testdata/`.
- **Semgrep runs on every file write via a hook and blocks on findings.** Inline `// nosemgrep` / `//nolint` suppressions are not allowed — fix by restructuring.

## Collectors (per the design spec §5)

`alerts.go` (`/alerts` → `nsr_alert_info`, `nsr_alerts_total`) · `clients.go` (`/clients` →
`nsr_client_info`, `nsr_client_parallelism`) · `jobs.go` (`/serverstatistics` + `/jobs` →
`nsr_server_*`, `nsr_job_status`) · `sessions.go` (`/sessions` — live in-flight activity →
`nsr_session_*`) · `storage.go` (`/volumes` + `/datadomainsystems` → `nsr_volume_*`,
`nsr_datadomain_*`) · `sizing.go` (bounded `/backups` → FETB `nsr_backup_*`,
`nsr_job_bytes_per_second`, `nsr_job_duration_seconds`).
See spec §5 for the full label/value schema before implementing any of them. Future endpoints
available in the NetWorker REST API (wrapped in `ansible-networker`): `/pools`, `/devices`,
`/storagenodes`, `/protectionpolicies`, `/protectiongroups`, `/nasdevices`.
