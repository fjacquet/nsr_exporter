# nsr_exporter production readiness — design

Status: draft for approval. Date: 2026-10-27.

## Intent

Get nsr_exporter ready to be called v1.0 for a large NetWorker estate. The trigger:
issues #36 and #72 showed that a single collector emitting two identical series makes
the whole `/metrics` endpoint return HTTP 500, and the exporter has never been sized
or measured against a big server.

**Reference scale (stated by Fred):** NetWorker servers with about 5,000 clients each.
The exporter is a monitoring tool. It reports; it never searches, restarts or manages
jobs. Observability detail (per-job series) is a feature, not waste.

## Scope — three pieces, built in this order, one PR each

Item 5 from the earlier assessment (freezing the metric/label contract and a
deprecation policy) is **out of scope here**. It follows from what "production" turns
out to mean after this work, and is decided when v1.0 is tagged.

### A. Direct tests for `internal/nsrclient` (currently 0% coverage)

The client carries security invariants that are only tested indirectly today. Add
`httptest`-based tests, written before any change to the client:

- A 401 response causes exactly 1 request (4xx is never retried; ADR-0007).
- A 503 response causes 3 requests (first try plus 2 retries), and a transport error is retried.
- With `Trace` on, captured logrus output contains neither the base64 `Authorization` value nor the password.
- `fl` and `q` are sent as query parameters and correctly encoded.
- A non-200 response returns an error that names the system and path.

No production change is expected. If a test exposes a bug, fix it in the same PR.

### B. Series guard: one bad collector must not 500 `/metrics`

A new step in `CollectOnce` runs **after** the `system` label is applied and
**before** `store.Swap`, so both the Prometheus and OTLP paths are protected and
`system` is part of the key.

- Detect duplicate series: same metric name and same label set. The key must not depend on label order (sort label keys).
- Also guard the other conditions that abort `Registry.Gather()`. Checked in `client_golang` `registry.go` (`processMetric`, `checkMetricConsistency`) on 2026-10-27, for an **unchecked** collector like `PromCollector`:
  - duplicate series (same name and label set): **aborts** — drop the later sample;
  - type mismatch within a metric family (e.g. gauge then counter): **aborts** — drop the later sample;
  - help text differing within a family: **aborts** — do not drop; rewrite the later sample's help to the first-seen text;
  - duplicate or invalid label names, non-UTF-8 label values: **abort** — drop the sample;
  - label-key sets differing within a family: does **not** abort an unchecked collector (`checkDescConsistency` only runs for registered descriptors). The guard must **not** drop these; ADR-0006's invariant stays enforced by tests, not by the guard.
  - the builder must also check `checkSuffixCollisions` (metric names that collide on `_total`, `_count`, etc.) and decide whether it needs a class of its own.
- On a hit: apply the action above, log it once per metric name per cycle, and expose it in a self-metric. The cycle still publishes; `nsr_up` is unchanged.
- The guard turns a loud HTTP 500 into a quiet undercount, so it ships with its own visibility: a new alert in `deploy/prometheus/nsr.rules.yml` on the self-metric, an entry in `catalog.go` (`TestCatalogCoversAllEmittedMetrics` requires it), and a row in `docs/metrics.md`.
- Test: feed a synthetic snapshot containing each error class and assert `Gather()` succeeds, good samples remain, and a label-key mismatch is left untouched. Also assert via an OTLP `ManualReader`, per the dual-export rule.

Self-metric: a **per-cycle gauge** `{system, reason}` holding the number of samples fixed or dropped in the last cycle. It lives in the snapshot, so it needs no state outside it and resets cleanly on reload (a counter would need state outside the immutable snapshot). Final name and unit follow the exporter-standards family convention (builder's call; check the skill).

### C. Scale and performance for 5,000 clients, with a bounded `/jobs`

**Measure first, then change.** Add a Go benchmark with large synthetic `httptest`
fixtures. Starting sizes (my guess, adjust after the first run): 5,000 clients, 10,000
concurrent sessions, 50,000 jobs in the window. Record: collection-cycle wall time,
allocations and peak memory, `Gather()` time, and `/metrics` body size.

**Decided changes:**

- `/jobs` is currently read without any filter, and each job emits about three series carrying a `job_id` label (`jobs.go`), so cardinality grows with all history. Add a config option `collection.jobWindow` (duration, **default 24h**, same pattern as `backupWindow` in `internal/config/config.go`, which replaces an unset or zero value with the default) and bound `/jobs` with a `q=` filter on `startTime`, modeled on `backupWindowFilter` in `sizing.go` (relative range form; isolate it in one function). The swagger lists a time-range filter parameter for `/jobs`; the builder must confirm the exact `q=` syntax for `startTime` in `docs/swagger/swagger-v3.yaml`.
- Also cut client-side with the existing `parseTime`: drop any returned job whose `startTime` is older than the window. If the server ignores `q=`, the cardinality bound still holds, for one comparison per job. Failure mode to note in the code and docs: if the filter syntax is wrong and the server answers 400, `jobs.go` returns an error and the `/serverstatistics` samples fetched by the same collector are lost too.
- Inside the window, per-job series keep their full detail. Operators shorten the window to trade detail for size. No other knob (no job cap, no aggregate-only mode, no "unbounded" value) unless measurement shows a need.
- Document the option in `config.yaml`, `config.demo.yaml`, the Helm chart values if it exposes `backupWindow`, and `docs/`. Add a CHANGELOG "Changed" entry: the 24h cut is a visible change from today's unbounded fetch. Dashboards and the `NsrJobFailed`-style rule (`nsr_job_status{completion_status="Failed"}`) were checked and use only recent jobs, so they are not affected; the builder re-greps `grafana/` and `deploy/prometheus/` for `nsr_job_*` before merging.

**Candidates, only if the benchmark shows they matter:**

- Collectors run one after another inside one shared 60s `CollectOnce` timeout, so one slow endpoint starves later ones and flips `nsr_up` to 0. Option: run a system's collectors concurrently with a per-collector timeout, with a **small cap on in-flight requests per system** (start at 3–4). On a 5,000-client server the NetWorker REST API is the likely bottleneck, and 11 parallel requests could make it worse.
- `PromCollector` builds a `prometheus.NewDesc` per sample on every scrape. Cache descriptors only if scrape time or allocations justify it.
- NetWorker supports optional `PageSize` and `PageNumber` on list endpoints. Per the swagger, by default **all results come back in one page**, so missing pagination does not lose data; the risk is payload size, memory and timeouts. Chunking with `PageSize` helps when a single request times out; it does not reduce memory once the pages are accumulated. This is per swagger and is not confirmed on a live server.

**Thresholds, at the reference scale (5,000 clients), and the lever for each:**

| Threshold | Lever |
| --- | --- |
| Exporter's own processing of a cycle (decode, build, guard, swap) takes less than 50% of `collection.timeout` | concurrent collectors; `PageSize` if one request times out |
| `/metrics` answers in under 1s | descriptor cache |
| `/metrics` body under 20 MB | `jobWindow` only |

- The first two are **Fred's** (accepted). The 20 MB figure is **the author's call**: Fred gave no size. A rough estimate of 400–500 bytes per job series puts 50,000 jobs at 20–25 MB before clients, so the cap may fail at the 24h default. The first benchmark run checks this estimate. **If the cap fails at 24h, the choice between per-job detail and the size cap is Fred's**, and the builder stops and asks rather than shrinking the default silently.
- An `httptest` mock answers instantly, so the benchmark measures the exporter's own work, not NetWorker's response time. The first threshold is therefore the exporter's processing budget. To judge real cycle time in production, add a collection-duration self-metric (per system, in seconds); none exists today (builder: check the exporter-standards convention for its name).
- The PR shows benchmark numbers before and after, and a recommended `jobWindow` for a 5,000-client server.

If a threshold is missed, the candidate changes above become required, in the order concurrent collectors, `PageSize` chunking, descriptor cache, and only as far as needed to pass. Only `jobWindow` moves the body size.

## Calls made by the author (not yet confirmed by Fred — check these)

- **Long-running jobs.** `jobWindow` filters on `startTime`, so a job that started before the cutoff and is still running drops out of the per-job series. Accepted: live work stays visible through `/sessions` (`nsr_session_*`) and `nsr_server_current_saves` / `nsr_server_current_recovers`. The builder must not add an "or still running" clause unless the swagger shows the API can express it.
- **No "unbounded" value for `jobWindow`.** Fred did not ask for one, and the `backupWindow` pattern turns 0 into the default (yaml.v2 cannot tell an omitted field from 0). The default is 24h (confirmed by Fred).
- **Server-side retention.** The swagger says NetWorker prunes job records after the `jobsdb retention` setting (default 72h). A 24h window is therefore a visible cut of history on a default server, and no cut at all if retention is shorter. Document this next to the option.
- **Guard ordering.** "Keep the first" means first in collector order and API response order, which is fixed for a given backend response. No sorting is added. For a duplicate or type mismatch, the first-seen sample or family type wins and later samples that disagree are dropped; for a help mismatch, the later sample is kept with the first-seen help.
- **Guard self-metric.** A per-cycle gauge with labels `{system, reason}` (reason is one of the error classes), obeying the label-key invariant. The log line names the metric and is emitted once per metric name per cycle. The collector name is not available at that point, so it is not a label.

## Goals, non-goals, anti-goals

Goals: one failing collector can never take down `/metrics`; the client's security behavior is pinned by tests; the exporter's cost at 5,000 clients is measured and bounded.

Non-goals: freezing the v1 metric contract (item 5); live-appliance validation of every collector; reviewing the Helm chart beyond adding the new option; adding management features.

Anti-goals: dropping per-job detail to make numbers look good; fake zeros to hide a bad value (ADR-0008); any `nosemgrep` or `nolint` suppression; changing existing metric names or label keys.

## Constraints

- Follow the `exporter-standards` family conventions and CLAUDE.md: TDD; tests assert via both Prometheus gather and an OTLP `ManualReader`; `make ci` must pass; no inline suppressions.
- Semgrep must pass on every changed file. Edit code with the Edit/Write tools so the write hook runs.
- PR #77 (session aggregation) must be merged first. Branch each piece from `main`, not from another feature branch.
- Each piece is its own PR so a performance regression can be reverted without losing the guard or the tests.

## Left to the builder

Exact self-metric name; where the guard lives (new file versus `collector.go`); the benchmark harness layout; whether the candidate performance changes are needed, judged from the measurements; the exact `startTime` filter syntax once checked against the swagger.
