# Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make nsr_exporter safe and measured for a 5,000-client NetWorker server: pin the client's security behavior with tests, stop one bad collector from 500-ing `/metrics`, and bound `/jobs` so `/metrics` stays under 20 MB.

**Architecture:** Three PRs, each branched from `main`, in this order: A (client tests), B (series guard run in `CollectOnce` after `withSystem` and before `store.Swap`), C (collection-duration metric, benchmark and scale test, `collection.jobWindow`, and only the performance levers the measurements require).

**Tech Stack:** Go 1.27.2, `httptest`, `go-resty/resty/v2`, `prometheus/client_golang` (unchecked collector), OTel SDK `ManualReader`, logrus, golangci-lint v2.

**Spec:** `docs/superpowers/specs/2026-10-27-production-readiness-design.md` (approved by Fred). Read it first, especially "Calls made by the author".

## Global Constraints

- TDD: write the failing test, watch it fail, then implement.
- Tests that touch metrics assert through **both** a Prometheus `Registry.Gather()` and an OTLP `ManualReader` (dual export, ADR-0004).
- Absent, never zero: an unparseable vendor value yields no sample (ADR-0008). The guard's own counts are real counts, so 0 is legitimate there.
- No `// nosemgrep`, no `//nolint`. Use the Edit/Write tools for code so the Semgrep hook runs.
- Existing metric names, label keys and values do not change; new metrics are additive. The only behavior change is `/jobs` returning fewer jobs.
- `make ci` (gofmt, `go vet`, `golangci-lint`, `go test -race`, `govulncheck`) passes before every PR is opened.
- Branch each PR from `origin/main`, never from another feature branch. PR #77 (session aggregation) must be merged before B and C; A does not touch it.
- Commit messages end with the two attribution lines the session provides; write them to a file and use `git commit -F`.
- Thresholds (spec): exporter's own cycle processing < 50% of `collection.timeout`; `/metrics` answers < 1 s; `/metrics` body < **20 MB**. The 20 MB cap wins over per-job detail: shorten the default `jobWindow` until it passes, never raise the cap.
- `jobWindow` default is 24h, and there is no "unbounded" value (an unset or zero value becomes the default, like `backupWindow`).

## Review Focus

- A `/jobs` entry with an unparseable or missing `startTime`: kept (absent timestamp, not dropped), never treated as old. Pinned in Task 5.
- A job still running that started before the window: dropped from per-job series by design; `/sessions` and `nsr_server_current_*` still show live work. Pinned in Task 5.
- A server that ignores `q=` and returns every job: the client-side cut still bounds series. Pinned in Task 5.
- `/jobs` answering 400 (wrong filter syntax): the collector returns an error and `/serverstatistics` samples from the same collector are lost; the system's `nsr_up` goes to 0 and other systems still publish. Pinned in Task 5.
- Two systems producing the same metric and label values: not a duplicate, because `system` is part of the key. Pinned in Task 2.
- A label-key mismatch inside one family must pass through untouched by the guard. Pinned in Task 2.

---

## PR A

### Task 1: Direct tests for `nsrclient`

**Files:**
- Create: `internal/nsrclient/client_test.go` (package `nsrclient`)

**Interfaces:**
- Consumes: `New(Options) *Client`, `(*Client).Get(ctx, path string, QueryOpts, out any) error`, `QueryOpts{Fields []string, Filter string}` from `internal/nsrclient/client.go`.
- Produces: nothing later tasks use.

- [ ] **Step 1: Write the tests** in `client_test.go`, each using an `httptest.NewServer` that counts requests (atomic counter) and serves under `/nwrestapi/v3/global/`. Build the client with `Options{Name: "t", Host: srv.URL, Username: "user", Password: "s3cret-pw", Timeout: 5 * time.Second, Log: <logrus logger writing to a bytes.Buffer>}`:
  - `TestGet_401NotRetried`: server always 401 → `Get` returns an error mentioning the system name and path, and the request count is exactly 1.
  - `TestGet_503RetriedTwice`: server always 503 → error, request count exactly 3.
  - `TestGet_RetriesThenSucceeds`: first response 503, second 200 `{"clients":[]}` → no error, request count 2. (Resty retry waits 500 ms; the test takes ~0.5 s, which is acceptable.)
  - `TestGet_TransportErrorRetried`: close the server, call `Get` → error; assert the request was attempted more than once by using a listener that counts accepted connections, or skip this test if a counting listener is more than ~15 lines (note the skip reason in a comment).
  - `TestGet_QueryParamsEncoded`: `QueryOpts{Fields: []string{"a","b"}, Filter: `saveTime:["24 hours"]`}` → the server sees `fl=a,b` and the `q` value decoded back to `saveTime:["24 hours"]`.
  - `TestGet_Non200ReturnsError`: server 404 → error text contains `status 404`.
  - `TestGet_SendsBasicAuth`: server checks `r.BasicAuth()` equals `user` / `s3cret-pw` on every request.
  - `TestTrace_DoesNotLeakCredentials`: client with `Trace: true`, a successful call; the captured log output must contain the method, the path and the status, and must contain neither `s3cret-pw` nor the base64 of `user:s3cret-pw`.

- [ ] **Step 2: Run** `go test ./internal/nsrclient/ -race -v`. Expected: all pass (no production change is expected). If one fails, it exposes a real bug: fix `client.go` minimally in this same task and note it in the commit message.

- [ ] **Step 3: Verify the trace test can fail.** Temporarily make `installTrace` also log `r.Request.Header.Get("Authorization")`, run the test, confirm it fails, then revert.

- [ ] **Step 4: Commit** `test(nsrclient): pin retry, auth, query and trace behavior`. Run `make ci`, push the branch `test/nsrclient-direct`, open PR A.

---

## PR B

### Task 2: Series guard

**Files:**
- Create: `internal/nsr/guard.go`, `internal/nsr/guard_test.go`
- Modify: `internal/nsr/collector.go` (call the guard in `CollectOnce`), `internal/nsr/catalog.go`, `deploy/prometheus/nsr.rules.yml`, `docs/metrics.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: `models.Sample`, `lbl(key, value string) models.Label`, `systemLabel` constant from `internal/nsr/metrics.go`.
- Produces (runs inside `CollectOnce`, so Task 6's scale test exercises it without calling it directly):
  - `type guardReason string` with constants `reasonDuplicate = "duplicate"`, `reasonTypeMismatch = "type_mismatch"`, `reasonHelpMismatch = "help_mismatch"`, `reasonInvalidLabel = "invalid_label"` (add `reasonSuffixCollision` only if Step 1 shows it aborts `Gather`).
  - `func guardSamples(samples []models.Sample, systems []string, log *logrus.Logger) []models.Sample` returns the repaired samples **plus** one gauge per `(system, reason)`: name `nsr_guard_repaired_samples` (provisional; Step 1 confirms against the exporter-standards family convention), labels `reason` then `system` in that fixed order, value = count repaired in this cycle, emitted for every system and every reason including 0.

- [ ] **Step 1: Confirm the aborting classes and the name.** Read `registry.go` in the module `github.com/prometheus/client_golang` (`go list -m -f '{{.Dir}}' github.com/prometheus/client_golang`): `processMetric`, `checkMetricConsistency`, `checkSuffixCollisions`. Decide whether a suffix collision (names like `x` and `x_total`/`x_count`) needs its own reason. Invoke the `exporter-standards` skill and confirm or change the self-metric name. Record both decisions in a comment at the top of `guard.go` (one line each).

- [ ] **Step 2: Write failing tests** in `guard_test.go` (package `nsr`):
  - `TestGuard_DropsLaterDuplicate`: two samples, same name, same label set in different label order → one kept (the first), repaired count for `duplicate` = 1.
  - `TestGuard_SameSeriesDifferentSystemIsNotDuplicate`: same name and labels but `system` = `a` vs `b` → both kept, count 0.
  - `TestGuard_TypeMismatchDropsLater`: gauge then counter with the same name → counter dropped, `type_mismatch` = 1.
  - `TestGuard_HelpMismatchRewritesHelp`: same name, different help → both samples kept, the later one carries the first-seen help, `help_mismatch` = 1.
  - `TestGuard_InvalidLabelName`: a label key `bad-key` and a sample with the same key twice → both dropped, `invalid_label` = 2.
  - `TestGuard_LabelKeySetMismatchUntouched`: same name, different label keys → both kept, all counts 0.
  - `TestGuard_EmitsZeroForEveryReasonAndSystem`: no problems, systems `["a","b"]` → the self-metric exists for each system and each reason with value 0.
  - `TestGuard_GatherSucceedsViaPromAndOTLP`: build a snapshot with one of each aborting class through `SnapshotStore`, register `NewPromCollector(store)` and assert `Gather()` returns nil error and the good samples are present; read the same store through `NewOTLPExporter(store, sdkmetric.NewManualReader())` and assert the self-metric values.

- [ ] **Step 3: Run** `go test ./internal/nsr/ -run TestGuard -v`. Expected: FAIL (undefined `guardSamples`).

- [ ] **Step 4: Implement `guardSamples`** in `guard.go`. Dedup key: metric name plus label pairs sorted by key. Per-name state: first-seen type and help. Log one `Warn` per `(metric name, reason)` per call, never per sample. Does not mutate its input (build a new slice; `Sample.Labels` slices are shared, do not modify them in place).

- [ ] **Step 5: Run** the Task 2 tests. Expected: PASS.

- [ ] **Step 6: Wire it.** In `CollectOnce`, after `g.Wait()` and before `c.store.Swap`, replace `all` with `guardSamples(all, <names of c.systems>, c.log)`. Add a test `TestCollectOnce_GuardKeepsMetricsServing` in `export_test.go`: a mock whose `/sessions` collector is replaced by a stub `ResourceCollector` emitting two identical samples; after `CollectOnce`, `Gather()` returns nil error. Run it; expect PASS.

- [ ] **Step 7: Visibility.** Add the metric to `catalog.go` (`TestCatalogCoversAllEmittedMetrics` must pass), a row in `docs/metrics.md`, an alert in `deploy/prometheus/nsr.rules.yml` named `NsrSeriesRepaired` firing on `max_over_time(nsr_guard_repaired_samples[15m]) > 0` with severity `warning`, and a CHANGELOG "Added" entry. Run `promtool check rules` if installed, else note that it was not run.

- [ ] **Step 8: Verify and commit.** `make ci`. Commit `feat(collector): guard the snapshot against series that abort /metrics`, push `feat/series-guard`, open PR B.

---

## PR C

### Task 3: Collection-duration metric, benchmark, scale fixture

**Files:**
- Modify: `internal/nsr/collector.go`, `internal/nsr/catalog.go`, `docs/metrics.md`
- Create: `internal/nsr/scale_test.go`

**Interfaces:**
- Produces (used by Tasks 4–6):
  - Metric `nsr_collection_duration_seconds{system}` (provisional name; check the exporter-standards convention) set in `collectSystem`'s caller from a wall-clock timer around one system's collection.
  - `func scaleMock(t *testing.T, clients, sessions, jobs int) *httptest.Server` in `scale_test.go`: serves `/clients`, `/sessions`, `/jobs` with that many synthetic records (jobs with `startTime` spread evenly over the last 72 hours relative to `time.Now()`), plus minimal valid bodies for the other endpoints the default collectors call so `nsr_up` stays 1.
  - `BenchmarkCollectAndGather(b *testing.B)` and `BenchmarkGatherOnly`, sized 5,000 clients / 10,000 sessions / 50,000 jobs (the spec's starting guess).

- [ ] **Step 1: Write a failing test** `TestCollectionDurationMetric`: after `CollectOnce` against `mockNetWorker`, `Gather()` contains `nsr_collection_duration_seconds` with `system="nsr-test"` and a value `>= 0`; assert the same through an OTLP `ManualReader`. Expect FAIL.
- [ ] **Step 2: Implement** the timing in `CollectOnce` (use the injectable `c.now` only for the snapshot timestamp; use `time.Since` for the duration so tests stay deterministic in shape). Add to `catalog.go` and `docs/metrics.md`. Expect PASS.
- [ ] **Step 3: Write `scaleMock` and the two benchmarks.** Each benchmark reports bytes per op (`b.ReportAllocs()`) and, for the combined one, the `/metrics` body size via `b.ReportMetric(float64(len(body)), "body_bytes")` rendered with `promhttp.HandlerFor(reg, ...)`.
- [ ] **Step 4: Commit** `test(nsr): add scale fixture, benchmarks and collection-duration metric` on branch `perf/scale-5000-clients`.

### Task 4: Baseline measurement (no production change)

**Files:**
- Create: `docs/superpowers/specs/2026-10-27-scale-baseline.md`

- [ ] **Step 1: Run** `go test ./internal/nsr/ -run '^$' -bench . -benchmem -count 3 -timeout 20m` on the unmodified `/jobs` code. Also record peak memory with `/usr/bin/time -l` on the benchmark binary.
- [ ] **Step 2: Write the numbers** into the baseline file: cycle wall time, allocs/op, bytes/op, `Gather()` time, `/metrics` body bytes, and the machine. State which of the three thresholds fail.
- [ ] **Step 3: Check the author's estimate.** Compare the measured bytes per job series with the spec's 400–500 bytes guess and say so in the file.
- [ ] **Step 4: Commit** `docs(specs): record the 5,000-client baseline`.

### Task 5: `jobWindow` bound on `/jobs`

**Files:**
- Modify: `internal/config/config.go` (add `JobWindow time.Duration \`yaml:"jobWindow"\`` to `CollectionConfig`, default `24 * time.Hour` in `applyDefaults`), `internal/nsr/jobs.go`, `internal/nsr/collector.go`, `internal/nsr/export_test.go` (`testCollector`), `config.yaml`, `config.demo.yaml`, `charts/nsr-exporter/values.yaml`, `docs/deployment/docker.md`, `docs/metrics.md`, `CHANGELOG.md`
- Test: `internal/config/config_test.go`, `internal/nsr/jobs_window_test.go`

**Interfaces:**
- Consumes: `backupWindowFilter(window time.Duration) string` pattern from `sizing.go`; `parseTime(s string) (float64, bool)` from `metrics.go`.
- Produces:
  - `type JobsCollector struct { Window time.Duration; Now func() time.Time }` (replaces the empty struct; `Now == nil` means `time.Now`). `JobsCollector` leaves `DefaultCollectors()` and is appended in `NewCollector` next to `SizingCollector`, with `Window: cfg.Collection.JobWindow`. Update `testCollector` to append `JobsCollector{Window: 24 * time.Hour, Now: nowZero}` so existing 2026 fixtures are not cut.
  - `func jobWindowFilter(window time.Duration) string` in `jobs.go`. Confirm the exact `q=` syntax for `startTime` in `docs/swagger/swagger-v3.yaml` (the `/jobs` operation references `queryFilterTimeRangeParam`); default to the `backupWindowFilter` form `startTime:["N hours"]` if the swagger agrees. Isolate the guess in this one function and say so in its comment.

- [ ] **Step 1: Write failing tests.**
  - `TestConfig_JobWindowDefault`: unset → `24 * time.Hour`; `jobWindow: 6h` → 6h.
  - `TestJobWindowFilter`: 24h → the confirmed syntax string; sub-hour windows round up to 1 hour like `backupWindowFilter`.
  - `TestJobs_SendsWindowFilter`: the mock records the `q` query parameter on `/jobs`; assert it equals `jobWindowFilter(24h)`.
  - `TestJobs_ClientSideCut`: server ignores `q` and returns three jobs (1h old, 23h old, 48h old, relative to a fixed `Now`); with a 24h window only the first two produce `nsr_job_status`.
  - `TestJobs_UnparseableStartTimeKept`: a job with an empty `startTime` is kept (it gets `nsr_job_status` but no start-timestamp sample).
  - `TestJobs_FilterRejected400`: `/jobs` returns 400 → `Collect` returns an error; through `CollectOnce`, `nsr_up` is 0 for that system while another system backed by a healthy mock still reports `nsr_up` 1.
  - `TestJobs_RunningJobBeforeWindowDropped`: a job with `state` running that started 30h ago is dropped (documents the accepted behavior).
- [ ] **Step 2: Run** them. Expected: FAIL.
- [ ] **Step 3: Implement** the config field and default, `jobWindowFilter`, passing it as `QueryOpts.Filter` for `/jobs`, and the client-side cut (skip a job only when `parseTime(j.StartTime)` succeeds and is older than `Now() - Window`). `/serverstatistics` handling is unchanged.
- [ ] **Step 4: Run** the new tests and the existing `TestJobsCollector*` tests. Expected: PASS.
- [ ] **Step 5: Document.** Add `jobWindow: "24h"` with a one-line comment to `config.yaml`, `config.demo.yaml` and the chart `values.yaml` (next to `backupWindow`), a paragraph to `docs/deployment/docker.md`, and mention NetWorker's own job retention (default 72 h per the swagger, so 24 h cuts visible history on a default server). CHANGELOG "Changed": the 24h cut is a visible change. Re-grep `grafana/dashboards/` and `deploy/prometheus/` for `nsr_job_*` and note the result in the PR.
- [ ] **Step 6: Commit** `feat(jobs): bound /jobs with collection.jobWindow`.

### Task 6: Scale threshold test and conditional levers

**Files:**
- Modify: `internal/nsr/scale_test.go`; `internal/nsr/collector.go` and `internal/nsr/prometheus.go` only if a threshold fails
- Modify: `docs/superpowers/specs/2026-10-27-scale-baseline.md`

- [ ] **Step 1: Write `TestScaleBudget_5000Clients`** (skipped under `-short`): build `scaleMock(5000, 10000, 50000)`, run `CollectOnce` with `JobsCollector{Window: <default>}`, render `/metrics` with `promhttp`, and assert `len(body) < 20*1024*1024`. Time assertions stay in the benchmarks (too flaky for a unit test); the test logs the cycle and gather durations.
- [ ] **Step 2: Run** it and the benchmarks with the 24h window. Record the after-numbers next to the baseline.
- [ ] **Step 3: Decide by threshold, in this order, and stop as soon as all three pass:**
  - Body ≥ 20 MB at 24h: lower the default `jobWindow` (config default, `config.yaml`, `config.demo.yaml`, chart values, docs, CHANGELOG) until it passes, and write the new default and the measured size in the PR. Do not raise the cap.
  - Cycle processing ≥ 50% of the timeout: run a system's collectors concurrently with a per-collector timeout and at most 3 in flight per system (`errgroup.SetLimit(3)` inside `collectSystem`); keep the per-collector failure behavior (that collector errors, others continue, `nsr_up` 0).
  - `/metrics` ≥ 1 s: cache `prometheus.Desc` per `(name, help, label keys)` in `PromCollector`, guarded for concurrent scrapes. If a single request times out on a real server, `PageSize` chunking is the next option; do not build it without a measurement that needs it.
- [ ] **Step 4: Verify** `make ci`, rerun the benchmarks, and put before/after numbers and the recommended `jobWindow` for a 5,000-client server in the PR description.
- [ ] **Step 5: Commit** and open PR C. Leave a note that live-server validation of the `startTime` filter is still pending.
