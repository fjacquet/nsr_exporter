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
- Also guard the other conditions that abort `Registry.Gather()` in `client_golang`. The builder must read `registry.go` (`processMetric`, `checkMetricConsistency`) and enumerate them. Expected: duplicate series, inconsistent label-key sets within one metric family, and help or type mismatch within a family. This matches the label-key invariant in ADR-0006.
- On a hit: drop the offending sample (keep the first), log it once per cycle with metric name and collector, and count it in a self-metric. The cycle still publishes; `nsr_up` is unchanged.
- Test: feed a synthetic snapshot containing each error class and assert `Gather()` succeeds and the good samples remain. Also assert via an OTLP `ManualReader`, per the dual-export rule.

Self-metric name and unit follow the exporter-standards family convention (builder's call; check the skill).

### C. Scale and performance for 5,000 clients, with a bounded `/jobs`

**Measure first, then change.** Add a Go benchmark with large synthetic `httptest`
fixtures. Starting sizes (my guess, adjust after the first run): 5,000 clients, 10,000
concurrent sessions, 50,000 jobs in the window. Record: collection-cycle wall time,
allocations and peak memory, `Gather()` time, and `/metrics` body size.

**Decided changes:**

- `/jobs` is currently read without any filter, and each job emits series carrying a `job_id` label, so cardinality grows with all history. Add a config option `collection.jobWindow` (duration, **default 24h**, same pattern as `backupWindow` in `internal/config/config.go`) and bound `/jobs` with a `q=` filter on `startTime`, modeled on `backupWindowFilter` in `sizing.go` (relative range form; isolate it in one function). The swagger lists a time-range filter parameter for `/jobs`; the builder must confirm the exact `q=` syntax for `startTime` in `docs/swagger/swagger-v3.yaml`.
- Inside the window, per-job series keep their full detail. Operators shorten the window to trade detail for size. No other knob (no job cap, no aggregate-only mode) unless measurement shows a need.
- Document the option in `config.yaml`, `config.demo.yaml`, the Helm chart values if it exposes `backupWindow`, and `docs/`.

**Candidates, only if the benchmark shows they matter:**

- Collectors run one after another inside one shared 60s `CollectOnce` timeout, so one slow endpoint starves later ones and flips `nsr_up` to 0. Options: run a system's collectors concurrently with a per-collector timeout.
- `PromCollector` builds a `prometheus.NewDesc` per sample on every scrape. Cache descriptors only if scrape time or allocations justify it.
- NetWorker supports optional `PageSize` and `PageNumber` on list endpoints. Per the swagger, by default **all results come back in one page**, so missing pagination does not lose data; the risk is payload size, memory and timeouts. Chunking with `PageSize` is a possible response to measured memory or timeout problems, not a correctness fix. This is per swagger and is not confirmed on a live server.

Success for C, at the reference scale (5,000 clients):

- a full collection cycle takes less than 50% of `collection.timeout` (30s of the default 60s);
- `/metrics` answers in under 1s;
- the `/metrics` body stays under 20 MB (**the author's call**: Fred accepted the other two thresholds and gave no figure for size; revisit once the first benchmark shows real sizes);
- the PR shows benchmark numbers before and after, and a recommended `jobWindow` for a 5,000-client server.

If a threshold is missed, the candidate changes above become required, in the order concurrent collectors, `PageSize` chunking, descriptor cache, and only as far as needed to pass.

## Calls made by the author (not yet confirmed by Fred — check these)

- **Long-running jobs.** `jobWindow` filters on `startTime`, so a job that started before the cutoff and is still running drops out of the per-job series. Accepted: live work stays visible through `/sessions` (`nsr_session_*`) and `nsr_server_current_saves` / `nsr_server_current_recovers`. The builder must not add an "or still running" clause unless the swagger shows the API can express it.
- **`jobWindow: 0` means unbounded**, which is today's behavior, for operators who want the old behavior back. The default is 24h (confirmed by Fred).
- **Server-side retention.** The swagger says NetWorker prunes job records after the `jobsdb retention` setting (default 72h). A 24h window is therefore a visible cut of history on a default server, and no cut at all if retention is shorter. Document this next to the option.
- **Guard ordering.** "Keep the first" means first in collector order and API response order, which is fixed for a given backend response. No sorting is added. For a label-key-set or help/type mismatch, the first-seen definition of the family wins and later samples that disagree are dropped.
- **Guard self-metric.** A counter with labels `{system, reason}` (reason is one of the error classes), obeying the label-key invariant. The log line names the metric and is emitted once per metric name per cycle. The collector name is not available at that point, so it is not a label.

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
