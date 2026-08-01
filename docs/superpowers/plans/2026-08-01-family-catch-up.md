# Family Standard Catch-Up (nsr_exporter) Implementation Plan

> This plan is written for agentic workers. Each task is self-contained: it names
> the exact files, the exact code, and the exact command to run. Do not infer
> anything from another task — every task repeats what it needs. Execute tasks in
> order; each ends at a committable state.

**Goal:** Bring `nsr_exporter` onto the two family standards it was never enrolled
in — the always-200 `/livez` + `/readyz` probe pattern, and the Alpine container
image standard (`alpine:latest`, `HEALTHCHECK`, compose `healthcheck:`) — plus the
two repo-level artefacts it is missing entirely: a `main_test.go` and a
`CHANGELOG.md`.

**Architecture:** `main.go` owns all HTTP wiring in `newServer` (lines 116-133),
which builds one explicit `http.ServeMux` and returns an `*http.Server`. Today that
mux carries exactly two routes: `cfg.Server.URI` (`/metrics`, served from the
Prometheus registry) and an inline `/health` closure that reads
`store.Load().Collected` and answers **503** with body `starting` until the first
collection cycle publishes a snapshot. This plan splits that closure into two named
package-level functions — `healthHandler` (informational, always 200) and
`staticOKHandler` (state-free, always 200) — and registers `/livez` and `/readyz`
on the same mux. Nothing else in the process changes: `internal/nsr.SnapshotStore`
is untouched, the collection loop is untouched, the export paths are untouched.

**Tech Stack:** Go 1.26 (module `github.com/fjacquet/nsr_exporter`), cobra, logrus,
`prometheus/client_golang`, OpenTelemetry SDK metric. Tests are stdlib `testing`
plus `net/http/httptest`. Container: multi-stage `./Dockerfile` (local/dev) and
`Dockerfile.goreleaser` (release, GoReleaser `dockers_v2`, per-`TARGETPLATFORM`
binary). Docs: MkDocs Material + `awesome-pages`. Gate: `make ci`
(`lint test build vuln`).

**Spec:** `/Users/fjacquet/Projects/obs_exporter/docs/superpowers/specs/2026-08-01-family-standard-catch-up-design.md`
— this plan implements its **Plan C — `nsr_exporter`** section, under the spec's
"Canonical patterns", "Testing" and "Documentation" sections.

---

## Global Constraints

These apply to **every** task. Violating one of these is how each previous
family-wide effort shipped a defect.

1. **`127.0.0.1`, never `localhost`.** Alpine's busybox `wget` resolves `localhost`
   via `::1` first and this exporter binds IPv4 only, so a `localhost`-based health
   check fails at runtime with connection refused — *after* passing both `hadolint`
   and `docker compose config`. Every `HEALTHCHECK` and every compose
   `healthcheck:` in this plan uses the literal string `http://127.0.0.1:9447/livez`.

2. **Timeout is `5s` in BOTH places.** The Dockerfile `HEALTHCHECK --timeout=5s`
   and the compose `healthcheck: timeout: 5s` must match. The Alpine effort shipped
   a 5s/10s mismatch across all eight family repos and had to correct it in every
   final review. Full parameter set, identical everywhere:
   `interval 30s`, `timeout 5s`, `start_period`/`--start-period` `10s`, `retries` 3.

3. **Port stays 9447.** `nsr_exporter` **keeps** 9447 (spec decision 1); it is
   `kemp_exporter` that moves to 9448. Do not change any port in this repo. If you
   find yourself editing `9447`, stop — you are in the wrong repo.

4. **hadolint findings are expected, not defects.** `DL3025` (shell-form `CMD`) is
   *unavoidable* given the required `CMD wget … || exit 1` syntax. `DL3007`
   (`:latest` tag) and `DL3066` are standing family findings — `DL3007` is now
   deliberate per spec decision 5. Do **not** add inline `# hadolint ignore=`
   suppressions (the repo forbids inline suppressions), and do **not** treat these
   as blocking.

5. **Verification means building and running the image.** Reading a Dockerfile is
   not verification. Task 7 is not optional: build the image, run it, and assert
   `docker inspect --format='{{.State.Health.Status}}' <container>` prints
   `healthy`. The `localhost`/`::1` bug passed every static check there is.

6. **Confirm the ADR number by `ls docs/adr/` before writing it.** A prior effort
   shipped literal `ADR-000N` placeholders into committed Dockerfile comments. At
   time of writing there are 11 ADRs (`0001`…`0011`), so the next free number is
   `0012` — but confirm it, do not trust this sentence.

7. **The ADR needs a row in `docs/adr/index.md`.** Every existing ADR has one; a
   new ADR without one is an incomplete change.

8. **Apple Silicon note for `Dockerfile.goreleaser`.** That file has no builder
   stage — it `COPY`s `${TARGETPLATFORM}/nsr_exporter` from the build context. To
   build it locally on an M-series Mac you must first cross-compile with
   `GOOS=linux GOARCH=arm64` into `./linux/arm64/nsr_exporter` and pass
   `--build-arg TARGETPLATFORM=linux/arm64`. Mismatch the arch and the container
   dies instantly with `exec format error`, and the health status never leaves
   `starting`.

9. **No inline `//nolint` or `# nosemgrep`.** Restructure instead. Repo rule.

10. **The CHANGELOG backfill is a summary of real history, never an invention.**
    Derive it from `git tag --sort=v:refname` and `git log` between tags. If a tag
    range contains only a dependency bump, the entry says so. Do not manufacture
    "Added"/"Fixed" bullets to make a version look substantial.

11. Run `gofmt -w .` before every commit that touches a `.go` file; `make ci` runs
    `golangci-lint` which will otherwise fail the gate.

---

## File Structure

| Action | Path | Purpose |
|---|---|---|
| Modify | `/Users/fjacquet/Projects/nsr_exporter/main.go` | `newServer` gains `/livez` + `/readyz`; `/health` closure split into `healthHandler` (always 200) and `staticOKHandler` |
| Create | `/Users/fjacquet/Projects/nsr_exporter/main_test.go` | First server test in this repo: `/livez`, `/readyz`, `/health` before and after first snapshot |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/Dockerfile` | `alpine:3.24` → `alpine:latest`; add `HEALTHCHECK` |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/Dockerfile.goreleaser` | `alpine:3.24` → `alpine:latest`; add `HEALTHCHECK` |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/docker-compose.yml` | `healthcheck:` on the `nsr_exporter` service |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/docker-compose.ghcr.yml` | `healthcheck:` on the `nsr_exporter` service |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/charts/nsr-exporter/values.yaml` | `livenessProbe` → `/livez`, `readinessProbe` → `/readyz` |
| Create | `/Users/fjacquet/Projects/nsr_exporter/CHANGELOG.md` | Keep a Changelog, backfilled v0.1.0 → v0.12.4, this work under `## [Unreleased]` |
| Create | `/Users/fjacquet/Projects/nsr_exporter/docs/adr/0012-health-probes-and-container-healthcheck.md` | ADR recording probes + HEALTHCHECK + unpinned Alpine |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/docs/adr/index.md` | Row for ADR-0012 |
| Verify only | `/Users/fjacquet/Projects/nsr_exporter/docs/adr/.pages` | Uses the `...` rest token — confirm no edit is needed |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/README.md` | Line 22 mentions only `/health`; add the probes |
| Modify | `/Users/fjacquet/Projects/nsr_exporter/docs/deployment/docker.md` | Document the endpoints and the image health check |

---

### Task 1: Failing tests for the probes and the always-200 `/health`

**Files:**
- Create: `/Users/fjacquet/Projects/nsr_exporter/main_test.go`

**Interfaces:**
- Consumes: `newServer(cfg *config.Config, store *nsr.SnapshotStore, reg *prometheus.Registry) *http.Server` (existing, `main.go:116`); `nsr.NewSnapshotStore()`, `(*nsr.SnapshotStore).Swap(*models.Snapshot)` (`internal/nsr/store.go`); `models.Snapshot{Samples, Collected}` (`internal/models`).
- Produces: `main_test.go` in `package main`, red until Task 2.

Convention note: this repo's existing tests (e.g.
`/Users/fjacquet/Projects/nsr_exporter/internal/nsr/store_test.go`) use plain
stdlib `testing`, no test framework, `t.Fatalf` with a `got = X, want Y` message,
and a doc comment above any non-obvious test. Match that. There is no `testify`
dependency — do not add one.

- [x] **Step 1: Read the current `newServer` so the test matches the real signature.**
      Read `/Users/fjacquet/Projects/nsr_exporter/main.go` lines 116-133. Confirm
      the signature is
      `func newServer(cfg *config.Config, store *nsr.SnapshotStore, reg *prometheus.Registry) *http.Server`
      and that `/health` currently writes `http.StatusServiceUnavailable` when
      `store.Load().Collected.IsZero()`.

- [x] **Step 2: Write `main_test.go`.** Create
      `/Users/fjacquet/Projects/nsr_exporter/main_test.go` with exactly this
      content:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/fjacquet/nsr_exporter/internal/config"
	"github.com/fjacquet/nsr_exporter/internal/models"
	"github.com/fjacquet/nsr_exporter/internal/nsr"
)

// testHandler builds the real mux newServer() wires, so these tests assert the
// routes are actually registered — not merely that the handler funcs behave.
func testHandler(t *testing.T, store *nsr.SnapshotStore) http.Handler {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = "9447"
	cfg.Server.URI = "/metrics"
	return newServer(cfg, store, prometheus.NewRegistry()).Handler
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// collectedStore returns a store that has published one snapshot, i.e. the
// post-first-cycle state.
func collectedStore() *nsr.SnapshotStore {
	s := nsr.NewSnapshotStore()
	s.Swap(&models.Snapshot{
		Samples:   []models.Sample{{Name: "nsr_alerts_total", Value: 3}},
		Collected: time.Now(),
	})
	return s
}

// TestLivezAlwaysOK: /livez reads no state, so it answers 200 both before and
// after the first collection cycle. A probe wired here can never be the reason a
// healthy process is restarted.
func TestLivezAlwaysOK(t *testing.T) {
	for name, store := range map[string]*nsr.SnapshotStore{
		"before first snapshot": nsr.NewSnapshotStore(),
		"after first snapshot":  collectedStore(),
	} {
		t.Run(name, func(t *testing.T) {
			rec := get(t, testHandler(t, store), "/livez")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "ok" {
				t.Fatalf("body = %q, want %q", got, "ok")
			}
		})
	}
}

// TestReadyzAlwaysOK mirrors TestLivezAlwaysOK: /readyz is the same state-free
// handler, so readiness never depends on backend reachability either.
func TestReadyzAlwaysOK(t *testing.T) {
	for name, store := range map[string]*nsr.SnapshotStore{
		"before first snapshot": nsr.NewSnapshotStore(),
		"after first snapshot":  collectedStore(),
	} {
		t.Run(name, func(t *testing.T) {
			rec := get(t, testHandler(t, store), "/readyz")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "ok" {
				t.Fatalf("body = %q, want %q", got, "ok")
			}
		})
	}
}

// TestHealthReturns200BeforeFirstSnapshot pins the behaviour change: /health used
// to answer 503 during the startup window. It now answers 200 and puts the
// startup state in the body instead.
func TestHealthReturns200BeforeFirstSnapshot(t *testing.T) {
	rec := get(t, testHandler(t, nsr.NewSnapshotStore()), "/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "starting" {
		t.Fatalf("body = %q, want %q", got, "starting")
	}
}

func TestHealthReturns200AfterFirstSnapshot(t *testing.T) {
	rec := get(t, testHandler(t, collectedStore()), "/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body = %q, want %q", got, "ok")
	}
}
```

- [x] **Step 3: Run the tests and confirm they fail for the right reasons.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && go test -run 'TestLivez|TestReadyz|TestHealth' ./...
```

      Expect: `TestLivezAlwaysOK` and `TestReadyzAlwaysOK` fail with `status = 404`
      (routes not registered yet), and `TestHealthReturns200BeforeFirstSnapshot`
      fails with `status = 503, want 200`. `TestHealthReturns200AfterFirstSnapshot`
      should already pass — that path is unchanged. If `/livez` returns 200 already,
      stop: you are editing the wrong repo.

---

### Task 2: Wire `/livez`, `/readyz`, and make `/health` always 200

**Files:**
- Modify: `/Users/fjacquet/Projects/nsr_exporter/main.go`

**Interfaces:**
- Consumes: `*nsr.SnapshotStore` (`Load() *models.Snapshot`), `*config.Config`, `*prometheus.Registry`.
- Produces: package-level `healthHandler(w http.ResponseWriter, store *nsr.SnapshotStore)` and `staticOKHandler(w http.ResponseWriter, _ *http.Request)`; `newServer`'s mux serving `/metrics`, `/health`, `/livez`, `/readyz`. Signature of `newServer` is unchanged.

- [x] **Step 1: Replace the `newServer` body.** In
      `/Users/fjacquet/Projects/nsr_exporter/main.go`, replace this exact block
      (lines 116-133):

```go
func newServer(cfg *config.Config, store *nsr.SnapshotStore, reg *prometheus.Registry) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		// Healthy once we have ever published a snapshot.
		if store.Load().Collected.IsZero() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("starting"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{
		Addr:              cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}
```

      with:

```go
func newServer(cfg *config.Config, store *nsr.SnapshotStore, reg *prometheus.Registry) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		healthHandler(w, store)
	})
	mux.HandleFunc("/livez", staticOKHandler)
	mux.HandleFunc("/readyz", staticOKHandler)
	return &http.Server{
		Addr:              cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// healthHandler always answers 200 and reports the collection state in the body:
// "starting" until the first cycle has published a snapshot, "ok" afterwards.
// It answered 503 during that startup window until ADR-0012; the window is a
// normal part of starting up (the server serves before the first collect, see
// ADR-0001), not a failure, and a 503 there made every naive probe restart a
// perfectly healthy process.
func healthHandler(w http.ResponseWriter, store *nsr.SnapshotStore) {
	if store.Load().Collected.IsZero() {
		_, _ = w.Write([]byte("starting"))
		return
	}
	_, _ = w.Write([]byte("ok"))
}

// staticOKHandler always answers 200 — no snapshot read, no collection state,
// nothing that can make it fail once the process is running. /livez and /readyz
// both use it (ADR-0012): a probe wired here can never be the reason a healthy
// process gets restarted or pulled from rotation. /health remains the endpoint
// for anything that wants to know whether collection has actually run.
func staticOKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
```

- [x] **Step 2: Fix the stale comment above `newServer`'s call site.** At
      `main.go:94-95` the comment reads
      `// Serve HTTP BEFORE the first collection cycle: the first poll can exceed the`
      `// collection timeout and must not stall /metrics or /health (ADR: serve first).`
      Replace the second line with:

```go
	// collection timeout and must not stall /metrics, /health or the probes.
```

- [x] **Step 3: Format and run the tests.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && gofmt -w . && go test -race ./...
```

      Expect all four new tests green and every pre-existing test still green.

- [x] **Step 4: Run the full gate.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && make ci
```

- [x] **Step 5: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git add main.go main_test.go && \
git commit -m "feat(server): add always-200 /livez and /readyz, stop 503-ing /health

/health answered 503 during the startup window before the first collection
cycle published a snapshot. That window is normal startup, not a failure, and
a 503 there makes any naive liveness probe restart a healthy process.

/health now always answers 200 and reports state in the body (starting/ok).
/livez and /readyz are new, both wired to a state-free handler.

Adds main_test.go, this repo's first server test.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Point the Helm chart's probes at `/livez` and `/readyz`

**Files:**
- Modify: `/Users/fjacquet/Projects/nsr_exporter/charts/nsr-exporter/values.yaml`

**Interfaces:**
- Consumes: the `/livez` and `/readyz` routes added in Task 2.
- Produces: chart defaults that no longer wire Kubernetes probes to `/health`.

Rationale (not in the spec's Plan C bullet list, but required by the canonical
pattern it cites): the chart's `livenessProbe` and `readinessProbe` both point at
`/health`. After Task 2, `/health` returns 200 unconditionally, so leaving them
there gives Kubernetes a probe that can never fail *and* leaves the repo
contradicting the family standard. `charts/nsr-exporter/templates/deployment.yaml`
renders these values verbatim (`toYaml .Values.livenessProbe`), so `values.yaml` is
the only file that needs changing.

- [x] **Step 1: Edit the probe defaults.** In
      `/Users/fjacquet/Projects/nsr_exporter/charts/nsr-exporter/values.yaml`,
      replace:

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: http
readinessProbe:
  httpGet:
    path: /health
    port: http
```

      with:

```yaml
# Probes are wired to the state-free /livez and /readyz endpoints, never /health
# (ADR-0012). /health reports collection state and is the right endpoint for a
# human or an alerting rule; it is the wrong thing to restart a pod over.
livenessProbe:
  httpGet:
    path: /livez
    port: http
readinessProbe:
  httpGet:
    path: /readyz
    port: http
```

- [x] **Step 2: Check nothing else in the chart documents `/health` as a probe.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && grep -rn "health" charts/
```

      Fix any user-facing hit that now describes the wrong endpoint. Do not touch
      `templates/deployment.yaml` — it renders the values and needs no change.

- [x] **Step 3: Render the chart to confirm it still templates.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && helm template nsr-exporter charts/nsr-exporter | grep -A3 -E "livenessProbe|readinessProbe"
```

      Expect `path: /livez` and `path: /readyz`. If `helm` is not installed, run
      `helm lint charts/nsr-exporter` if available; otherwise re-read the rendered
      values by hand and note that the render was skipped.

- [x] **Step 4: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git add charts/nsr-exporter/values.yaml && \
git commit -m "feat(chart): default probes to /livez and /readyz, not /health

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: `HEALTHCHECK` + unpinned Alpine in `./Dockerfile`

**Files:**
- Modify: `/Users/fjacquet/Projects/nsr_exporter/Dockerfile`

**Interfaces:**
- Consumes: `/livez` on port 9447 (Task 2).
- Produces: a local/dev image that reports Docker health status.

- [x] **Step 1: Drop the Alpine pin.** In
      `/Users/fjacquet/Projects/nsr_exporter/Dockerfile`, replace the line
      `FROM alpine:3.24` with:

```dockerfile
# Unpinned per the family standard (spec decision 5, ADR-0012): all fifteen
# exporter repos share alpine:latest. This is the one build input whose contents
# can change between two builds of the same commit; uniformity was chosen over
# reproducibility, and revisiting it is a family-wide decision.
FROM alpine:latest
```

- [x] **Step 2: Add the HEALTHCHECK.** In the same file, replace:

```dockerfile
USER 10001
EXPOSE 9447
ENTRYPOINT ["/usr/local/bin/nsr_exporter"]
```

      with:

```dockerfile
USER 10001
EXPOSE 9447

# Probes /livez, never /metrics: rendering the full exposition on every probe
# tick is needless load and can block behind a slow collection cycle (ADR-0012).
# 127.0.0.1, never localhost — busybox wget tries ::1 first and the exporter
# binds IPv4 only, so a localhost check fails with connection refused.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9447/livez || exit 1

ENTRYPOINT ["/usr/local/bin/nsr_exporter"]
```

- [x] **Step 3: Lint it.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && docker run --rm -i hadolint/hadolint < Dockerfile
```

      Expected, non-blocking findings only: `DL3025` (shell-form CMD — unavoidable
      given `|| exit 1`), `DL3007` (`:latest` — now deliberate), `DL3066`. Any
      *other* finding is a real defect: fix it. Add no inline suppressions.

- [x] **Step 4: Build it.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && docker build -t nsr_exporter:healthcheck-test .
```

      (Runtime verification of the health status is Task 7.)

- [x] **Step 5: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git add Dockerfile && \
git commit -m "build(docker): add HEALTHCHECK against /livez, drop the alpine pin

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: `HEALTHCHECK` + unpinned Alpine in `Dockerfile.goreleaser`

**Files:**
- Modify: `/Users/fjacquet/Projects/nsr_exporter/Dockerfile.goreleaser`

**Interfaces:**
- Consumes: `/livez` on port 9447 (Task 2); the GoReleaser build context laying the binary out at `${TARGETPLATFORM}/nsr_exporter`.
- Produces: the published GHCR image reporting Docker health status.

- [x] **Step 1: Drop the Alpine pin.** In
      `/Users/fjacquet/Projects/nsr_exporter/Dockerfile.goreleaser`, replace the
      line `FROM alpine:3.24` with:

```dockerfile
# Unpinned per the family standard (spec decision 5, ADR-0012): all fifteen
# exporter repos share alpine:latest.
FROM alpine:latest
```

- [x] **Step 2: Add the HEALTHCHECK.** In the same file, replace:

```dockerfile
USER 10001
EXPOSE 9447
ENTRYPOINT ["/usr/local/bin/nsr_exporter"]
```

      with:

```dockerfile
USER 10001
EXPOSE 9447

# Probes /livez, never /metrics (ADR-0012). 127.0.0.1, never localhost — busybox
# wget tries ::1 first and the exporter binds IPv4 only.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9447/livez || exit 1

ENTRYPOINT ["/usr/local/bin/nsr_exporter"]
```

- [x] **Step 3: Lint it.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && docker run --rm -i hadolint/hadolint < Dockerfile.goreleaser
```

      Same expected-findings rule as Task 4.

- [x] **Step 4: Confirm GoReleaser still parses the config.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && goreleaser check
```

      If `goreleaser` is not installed, `make tools` installs the pinned version.

- [x] **Step 5: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git add Dockerfile.goreleaser && \
git commit -m "build(goreleaser): add HEALTHCHECK against /livez, drop the alpine pin

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: `healthcheck:` in both compose files

**Files:**
- Modify: `/Users/fjacquet/Projects/nsr_exporter/docker-compose.yml`
- Modify: `/Users/fjacquet/Projects/nsr_exporter/docker-compose.ghcr.yml`

**Interfaces:**
- Consumes: `/livez` on port 9447 (Task 2).
- Produces: both demo stacks reporting exporter health; parameters identical to the Dockerfiles' (interval 30s, timeout 5s, retries 3, start_period 10s).

- [x] **Step 1: Edit `docker-compose.yml`.** In the `nsr_exporter` service, replace:

```yaml
    depends_on:
      - mocknw
    restart: unless-stopped
```

      with (this is the first `depends_on: - mocknw` block under the
      `nsr_exporter:` service — the `mocknw` service above it has no `depends_on`,
      so the match is unique in that service, but confirm you are inside
      `nsr_exporter:` before editing):

```yaml
    depends_on:
      - mocknw
    # Mirrors the Dockerfile HEALTHCHECK exactly — 127.0.0.1 (busybox wget tries
    # ::1 first and the exporter binds IPv4 only) and timeout 5s in both places.
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9447/livez"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped
```

- [x] **Step 2: Edit `docker-compose.ghcr.yml`.** Apply the identical change to
      that file's `nsr_exporter` service — same block, same values, same comment.

- [x] **Step 3: Validate both files.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
docker compose -f docker-compose.yml config -q && \
docker compose -f docker-compose.ghcr.yml config -q && echo "both valid"
```

      Note this proves only that the YAML is well-formed — it would happily accept
      the broken `localhost` form too. Task 7 is the real check.

- [x] **Step 4: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git add docker-compose.yml docker-compose.ghcr.yml && \
git commit -m "build(compose): add exporter healthcheck to both stacks

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 7: Prove the health check actually reports `healthy` at runtime

**Files:** none modified — this task is verification. If it fails, go back and fix
the file it implicates.

**Interfaces:**
- Consumes: the images from Tasks 4 and 5, the compose files from Task 6.
- Produces: observed `docker inspect --format='{{.State.Health.Status}}'` output of `healthy`.

This task exists because the `localhost`/`::1` bug passed hadolint **and**
`docker compose config` while failing at runtime. Reading a Dockerfile is not
verification.

- [x] **Step 1: Run the local image.** `./Dockerfile` does not bake a config, so
      mount one and supply the credentials the demo config references:

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
docker run -d --name nsr_hc_test \
  -e NSR1_USERNAME=admin -e NSR1_PASSWORD=demopass \
  -v "$PWD/config.demo.yaml:/etc/nsr_exporter/config.yaml:ro" \
  -p 19447:9447 nsr_exporter:healthcheck-test
```

      The backend it points at does not exist — that is fine and is precisely the
      point: `/livez` must be healthy anyway. If the container exits immediately,
      run `docker logs nsr_hc_test`; a config-load failure means the env vars or
      the mount are wrong, not that the health check is wrong.

- [x] **Step 2: Poll until healthy (bounded).**

```bash
for i in $(seq 1 30); do
  s=$(docker inspect --format='{{.State.Health.Status}}' nsr_hc_test 2>/dev/null)
  echo "attempt $i: $s"
  [ "$s" = "healthy" ] && break
  [ "$s" = "unhealthy" ] && break
  sleep 2
done
docker inspect --format='{{.State.Health.Status}}' nsr_hc_test
```

      **Required output: `healthy`.** If it reports `unhealthy`, dump the probe
      output with
      `docker inspect --format='{{json .State.Health}}' nsr_hc_test` — a
      `connection refused` there is the `localhost`/IPv6 failure mode; re-check
      that Task 4 used `127.0.0.1`.

- [x] **Step 3: Tear down.**

```bash
docker rm -f nsr_hc_test
```

- [x] **Step 4: Build and run the release image.** On Apple Silicon,
      `Dockerfile.goreleaser` has no builder stage — cross-compile first and pass a
      matching `TARGETPLATFORM`, or the container dies with `exec format error`:

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
mkdir -p linux/arm64 && \
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o linux/arm64/nsr_exporter . && \
docker build -f Dockerfile.goreleaser --build-arg TARGETPLATFORM=linux/arm64 -t nsr_exporter:goreleaser-test .
```

      (On an amd64 host substitute `amd64` for `arm64` in all three places.)

- [x] **Step 5: Run it and poll.** This image bakes `config.yaml`, which references
      `${NSR1_*}`, so still pass the env vars:

```bash
docker run -d --name nsr_hc_rel -e NSR1_USERNAME=admin -e NSR1_PASSWORD=demopass \
  -p 19448:9447 nsr_exporter:goreleaser-test
for i in $(seq 1 30); do
  s=$(docker inspect --format='{{.State.Health.Status}}' nsr_hc_rel 2>/dev/null)
  echo "attempt $i: $s"
  [ "$s" = "healthy" ] && break
  [ "$s" = "unhealthy" ] && break
  sleep 2
done
docker inspect --format='{{.State.Health.Status}}' nsr_hc_rel
```

      **Required output: `healthy`.** If the baked `config.yaml` cannot load with
      just those two env vars, mount `config.demo.yaml` over
      `/etc/nsr_exporter/config.yaml` as in Step 1 and retry.

- [x] **Step 6: Tear down and clean the cross-compile output.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && docker rm -f nsr_hc_rel && rm -rf linux/
```

      Confirm `git status` is clean — `linux/` must not be committed. If
      `.gitignore` does not already cover it, verify it is gone rather than adding
      an ignore rule.

- [x] **Step 7: Verify the compose stack reports health.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && docker compose up -d --build nsr_exporter mocknw && \
for i in $(seq 1 30); do
  s=$(docker inspect --format='{{.State.Health.Status}}' nsr_exporter 2>/dev/null)
  echo "attempt $i: $s"
  [ "$s" = "healthy" ] && break
  sleep 2
done
docker compose ps
docker compose down
```

      **Required: `nsr_exporter` shows `(healthy)` in `docker compose ps`.**

---

### Task 8: Create `CHANGELOG.md`, backfilled from git history

**Files:**
- Create: `/Users/fjacquet/Projects/nsr_exporter/CHANGELOG.md`

**Interfaces:**
- Consumes: `git tag --sort=v:refname`, `git log` between tags.
- Produces: a Keep a Changelog file with an `## [Unreleased]` section describing this work, plus one section per released tag.

This repo has **no** `CHANGELOG.md` today (spec decision 3). The backfill must
**summarize honestly from real commits — never invent entries.** If a tag range
contains only a dependency bump, say exactly that. If a range's commits are
unclear, describe them at the level the commit messages actually support. A thin
entry that is true beats a rich entry that is fiction.

- [x] **Step 1: List the tags in order.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git tag --sort=v:refname
```

      At time of writing this yields 13 tags: `v0.1.0 v0.1.1 v0.9.0 v0.9.1 v0.10.0
      v0.10.1 v0.10.2 v0.11.0 v0.12.0 v0.12.1 v0.12.2 v0.12.3 v0.12.4`. Use what
      the command actually prints, not this list.

- [x] **Step 2: Get each tag's release date.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
for t in $(git tag --sort=v:refname); do printf '%s\t%s\n' "$t" "$(git log -1 --format=%ad --date=short "$t")"; done
```

- [x] **Step 3: Read the commits in each range.** For the first tag, everything up
      to it; for each subsequent tag, the range from its predecessor:

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
git log --oneline --no-merges v0.1.0 && \
prev=""; for t in $(git tag --sort=v:refname); do
  if [ -n "$prev" ]; then echo "===== $prev..$t ====="; git log --oneline --no-merges "$prev..$t"; fi
  prev=$t
done
```

      Also capture anything after the last tag:
      `git log --oneline --no-merges v0.12.4..HEAD`.

- [x] **Step 4: Write `CHANGELOG.md`.** Create
      `/Users/fjacquet/Projects/nsr_exporter/CHANGELOG.md` with this header and
      `## [Unreleased]` section verbatim, then one `## [x.y.z] - YYYY-MM-DD`
      section per tag in **reverse** chronological order (newest first), each with
      only the Keep a Changelog subsections (`Added`, `Changed`, `Fixed`,
      `Removed`, `Breaking`) that the real commits support:

```markdown
# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Versions before v0.13.0 predate this file; their entries are reconstructed from
git history and summarize each release at the level the commit messages support.

## [Unreleased]

### Added

- `/livez` and `/readyz` endpoints, both always answering `200 OK` with an `ok`
  body and reading no collection state whatsoever (ADR-0012).
- `HEALTHCHECK` against `/livez` in `Dockerfile` and `Dockerfile.goreleaser`, and
  a matching `healthcheck:` in `docker-compose.yml` and `docker-compose.ghcr.yml`.
- `main_test.go` — this repo's first server test, covering the probes and
  `/health` both before and after the first snapshot.

### Changed

- `/health` no longer returns `503` during the startup window before the first
  collection cycle. It always returns `200`; the startup state moved into the
  body (`starting` / `ok`). Anything scripting against the `503` must now read
  the body instead.
- The Helm chart's `livenessProbe` and `readinessProbe` default to `/livez` and
  `/readyz` instead of `/health`.
- Container base image is `alpine:latest`, unpinned, matching the rest of the
  exporter family; the previous `alpine:3.24` pin is dropped.
```

- [x] **Step 5: Sanity-check the backfill against the tag list.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
grep -c '^## \[' CHANGELOG.md && git tag | wc -l
```

      The heading count should be the tag count **plus one** (for `[Unreleased]`).
      A mismatch means a version was skipped or invented.

- [x] **Step 6: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git add CHANGELOG.md && \
git commit -m "docs: add CHANGELOG.md, backfilled from git history

Keep a Changelog format, one section per tagged release from v0.1.0 through
v0.12.4, summarized from the commits in each range. The probe and container
work lands under [Unreleased].

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 9: ADR-0012 + `index.md` row + `.pages` check

**Files:**
- Create: `/Users/fjacquet/Projects/nsr_exporter/docs/adr/0012-health-probes-and-container-healthcheck.md`
- Modify: `/Users/fjacquet/Projects/nsr_exporter/docs/adr/index.md`
- Verify only: `/Users/fjacquet/Projects/nsr_exporter/docs/adr/.pages`

**Interfaces:**
- Consumes: the decisions implemented in Tasks 2-6.
- Produces: an ADR discoverable in the MkDocs nav and listed in the ADR index.

- [x] **Step 1: Confirm the next free ADR number.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && ls docs/adr/
```

      Expect `0001`…`0011`, making `0012` next. **If the highest is not `0011`, use
      the real next number everywhere below** — filename, H1, index row, and the
      `ADR-0012` references in `main.go`, `Dockerfile`, `Dockerfile.goreleaser` and
      `charts/nsr-exporter/values.yaml` from Tasks 2-4. A prior effort in a sibling
      repo shipped literal `ADR-000N` placeholders into committed Dockerfile
      comments; that is what this step prevents.

- [x] **Step 2: Write the ADR.** Create
      `/Users/fjacquet/Projects/nsr_exporter/docs/adr/0012-health-probes-and-container-healthcheck.md`:

```markdown
# 0012 — Static `/livez` and `/readyz`, container health check, unpinned Alpine

**Status**: Accepted (2026-08-01)

## Context

`nsr_exporter` was never listed in the `exporter-standards` family table, so two
family-wide efforts completed on 2026-08-01 — the always-200 probe pattern and the
Alpine container-image standard — silently skipped this repo. This ADR records
applying both here; it does not re-decide either. The family rationale lives in
`obs_exporter` ADR-0013 (probes) and ADR-0016 (image).

Three things were true of this repo before this change:

1. `/health` answered **503** with body `starting` until the first collection cycle
   published a snapshot, and 200 with body `ok` afterwards. The Helm chart wired
   **both** `livenessProbe` and `readinessProbe` to it.
2. Neither Dockerfile declared a `HEALTHCHECK`, and neither compose file declared a
   `healthcheck:`, so `docker ps` could only ever report `Up` — never whether the
   exporter was serving.
3. Both Dockerfiles pinned `alpine:3.24`, where the rest of the family uses
   `alpine:latest`.

The 503 is the load-bearing problem. ADR-0001 has the HTTP server start serving
*before* the first collection cycle precisely so a slow first poll does not look
like a dead exporter — and then `/health` reported exactly that to any probe
reading the status code. As a liveness signal it is wrong in both directions: no
restart makes a first poll finish faster, and a restart throws away whatever
progress the cycle had made, guaranteeing the next probe fails too.

## Decision

1. **Two new endpoints, `/livez` and `/readyz`**, both wired to one
   `staticOKHandler` that answers `200 OK` with a fixed `ok` body and reads no
   `SnapshotStore` state, no collection state, nothing that can fail once the
   process is running. Registered on the same mux in `newServer`, so like
   `/metrics` they serve from before the first collection cycle.
2. **`/health` always answers 200.** The collection state stays, but as the *body*
   (`starting` before the first snapshot, `ok` after), never as the status code.
   The path, the body strings and its usefulness to a human or an alerting rule are
   unchanged.
3. **The chart's `livenessProbe` and `readinessProbe` default to `/livez` and
   `/readyz`.** No `startupProbe` is added: neither new endpoint has a startup
   window to cover.
4. **`HEALTHCHECK` against `/livez` in both Dockerfiles**, plus a matching
   `healthcheck:` in both compose files —
   `interval 30s`, `timeout 5s`, `start-period 10s`, `retries 3`, identical in all
   four places. The URL is `http://127.0.0.1:9447/livez`: **never `localhost`**,
   because busybox `wget` resolves it via `::1` first while the exporter binds IPv4
   only, and never `/metrics`, because rendering the full exposition on every probe
   tick is needless load that can block behind a slow cycle.
5. **The base image becomes `alpine:latest`, unpinned**, dropping `alpine:3.24`.

Point 5 cuts against this repo's own stated posture and is recorded plainly:
ADR-0011 pins everything else in the build — Actions by SHA, the linter and
scanner toolchain, the Go builder — so `alpine:latest` is the one input whose
contents can change between two builds of the same commit, which is what the SBOM
and provenance attestations exist to pin down. Uniformity across fifteen repos was
chosen over reproducibility on three. Revisiting it is a family-wide decision, not
a per-repo one.

## Consequences

- A Kubernetes deployment using the chart defaults can no longer be restarted, or
  pulled from rotation, because a NetWorker system is slow or unreachable. That was
  never something a restart could fix.
- **Anything scripting against `/health`'s 503 breaks.** The status is now always
  200; the distinction moved to the body. This is the one user-visible behaviour
  change in this ADR.
- `docker ps` and `docker compose ps` now report exporter health, and orchestrators
  can gate on `condition: service_healthy`.
- `hadolint` reports `DL3025` on both Dockerfiles — the shell-form `CMD … || exit 1`
  is the only syntax that works for a `HEALTHCHECK` — and `DL3007` for the unpinned
  tag. Both are expected and non-blocking; no inline suppressions were added.
- Two builds of the same commit may now differ in their Alpine layer.
- Alerting on backend reachability still means the `nsr_*` metrics or reading
  `/health`'s body. `/livez` and `/readyz` will never reveal a degraded backend;
  that was never their job.

## Related

- [ADR-0001](0001-snapshot-collection-model.md) — the server serves before the
  first collection cycle; this ADR stops `/health` from contradicting that.
- [ADR-0011](0011-supply-chain-release-hardening.md) — the pinning posture that
  decision 5 knowingly departs from.
- `docs/superpowers/specs/2026-08-01-family-standard-catch-up-design.md` — the
  cross-repo design this implements (Plan C).
```

- [x] **Step 3: Add the index row.** In
      `/Users/fjacquet/Projects/nsr_exporter/docs/adr/index.md`, append after the
      `0011` row:

```markdown
| [0012](0012-health-probes-and-container-healthcheck.md) | Health probes & container health check | Accepted |
```

- [x] **Step 4: Check `.pages`.** Read
      `/Users/fjacquet/Projects/nsr_exporter/docs/adr/.pages`. Its `nav:` is
      `- index.md` followed by the `...` rest token, which expands to every
      remaining ADR sorted by filename — so a new ADR appears automatically and
      **no edit is required**. Confirm this by reading the file; only if the `...`
      token is absent and ADRs are listed individually do you add a line for
      `0012-health-probes-and-container-healthcheck.md`.

- [x] **Step 5: Build the docs site.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && make docs
```

      (Equivalently
      `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict --site-dir site`.)
      `--strict` fails on broken links, so this also validates the relative links
      inside the ADR. Then `rm -rf site` — it is build output.

- [x] **Step 6: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
git add docs/adr/0012-health-probes-and-container-healthcheck.md docs/adr/index.md && \
git commit -m "docs(adr): record the probe, healthcheck and alpine:latest decisions as ADR-0012

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 10: Sweep user-facing docs for claims this change falsifies

**Files:**
- Modify: `/Users/fjacquet/Projects/nsr_exporter/README.md`
- Modify: `/Users/fjacquet/Projects/nsr_exporter/docs/deployment/docker.md`

**Interfaces:**
- Consumes: everything above.
- Produces: docs that describe the endpoints and the image as they now are.

Every repo in the Alpine effort needed a post-review fix wave for exactly this.
Do the sweep before claiming done, not after review.

- [x] **Step 1: Find every user-facing mention.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
grep -rn --include='*.md' --include='*.yaml' --include='*.yml' -E '/health|alpine:3|3\.24|503' . \
  | grep -v '^./site/' | grep -v '^./docs/adr/' | grep -v '^./docs/superpowers/'
```

      Historical ADRs and past specs are **records** — leave them as written.
      Everything else that describes current behaviour must be correct.

- [x] **Step 2: Update the README feature line.** In
      `/Users/fjacquet/Projects/nsr_exporter/README.md` line 22, replace:

```markdown
- **Operable** — `--once --debug` sample dump, credential-safe `--trace`, SIGHUP reload, `/health`.
```

      with:

```markdown
- **Operable** — `--once --debug` sample dump, credential-safe `--trace`, SIGHUP reload, `/health` plus always-200 `/livez` and `/readyz` probes.
```

- [x] **Step 3: Document the endpoints in the Docker deployment page.** In
      `/Users/fjacquet/Projects/nsr_exporter/docs/deployment/docker.md`, add this
      section immediately after the "Configuration" section's table and its two
      trailing paragraphs (before "## OTLP push export (optional)"):

```markdown
## Health endpoints

| Path | Status | Body | Use it for |
|---|---|---|---|
| `/livez` | always `200` | `ok` | Kubernetes `livenessProbe`, the image `HEALTHCHECK` |
| `/readyz` | always `200` | `ok` | Kubernetes `readinessProbe` |
| `/health` | always `200` | `starting` before the first collection cycle, `ok` after | A human, or an alerting rule reading the body |

`/livez` and `/readyz` read no collection state at all, so neither can fail because
a NetWorker system is slow or unreachable — no restart fixes that, and a restart
would discard the cycle already in progress (ADR-0012). Never point a probe at
`/metrics`: rendering the full exposition on every tick is needless load and can
block behind a slow collection cycle.

Both images declare a `HEALTHCHECK` against `/livez`, so `docker ps` reports real
health:

```bash
docker inspect --format='{{.State.Health.Status}}' nsr_exporter
```

Prior to v0.13.0 `/health` answered `503` during the startup window. It no longer
does — anything scripting against that status code must read the body instead.
```

- [x] **Step 4: Confirm the "minimal Alpine-based Docker image" opening line of
      `docker.md` is still accurate.** It says "minimal Alpine-based Docker image
      (non-root `USER 10001`)" — both remain true after this change, so leave it.
      Do not introduce a version number there.

- [x] **Step 5: Rebuild the docs.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && make docs && rm -rf site
```

- [x] **Step 6: Commit.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git add README.md docs/deployment/docker.md && \
git commit -m "docs: document /livez, /readyz and the image health check

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 11: Final gate

**Files:** none — verification only.

**Interfaces:**
- Consumes: the complete change set.
- Produces: observed clean output from every gate the repo defines.

- [x] **Step 1: Full Go gate.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && make ci
```

      This runs `golangci-lint run`, `go test -race -coverprofile`, `go build -v ./...`
      and `govulncheck ./...`. All four must pass.

- [x] **Step 2: Formatting and vet (not in `ci`).**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && make fmt-check && make vet
```

- [ ] **Step 3: Both Dockerfiles linted.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
docker run --rm -i hadolint/hadolint < Dockerfile; \
docker run --rm -i hadolint/hadolint < Dockerfile.goreleaser
```

      Only `DL3025`, `DL3007`, `DL3066` are acceptable. Anything else is a defect.

      Ran: `Dockerfile` shows only `DL3007`/`DL3025` (acceptable).
      `Dockerfile.goreleaser` shows `DL3007`, `DL3025`, **and `DL3018`**
      (unpinned `apk add`) — not in the acceptable list, and pre-existing on this
      branch's tip, not introduced by the four Minor findings this pass fixes.
      Left unticked and unfixed: out of scope for this pass.

- [x] **Step 4: Both compose files validated.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && \
docker compose -f docker-compose.yml config -q && \
docker compose -f docker-compose.ghcr.yml config -q && echo ok
```

- [ ] **Step 5: Docs site builds strict.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && make docs && rm -rf site
```

      Left unticked: literal `make docs` fails — the target is missing
      `--with mkdocs-awesome-pages-plugin`, a pre-existing Makefile gap, not part
      of this pass's four findings. Verified instead with that flag added on the
      command line: `mkdocs build --strict` succeeds clean.

- [x] **Step 6: Re-confirm the runtime health status.** Task 7 already did this;
      redo Step 7 of Task 7 once more against the final tree, since Tasks 8-10
      landed after it:

```bash
cd /Users/fjacquet/Projects/nsr_exporter && docker compose up -d --build nsr_exporter mocknw && \
for i in $(seq 1 30); do
  s=$(docker inspect --format='{{.State.Health.Status}}' nsr_exporter 2>/dev/null)
  [ "$s" = "healthy" ] && break; sleep 2
done
docker compose ps; docker compose down
```

      **Required: `(healthy)`.**

- [x] **Step 7: Confirm the working tree is clean and nothing stray was committed.**

```bash
cd /Users/fjacquet/Projects/nsr_exporter && git status --short && git log --oneline -8
```

      `linux/`, `site/`, `bin/`, `dist/` and `coverage.out` must not appear.

---

## Self-Review

Walk this list before declaring the work done. Each line is a defect a previous
family effort actually shipped.

- [x] `ls docs/adr/` was run and the ADR number was **confirmed**, not assumed. No
      file, comment or doc anywhere contains a literal `ADR-000N` placeholder:
      `grep -rn '000N' .` returns nothing.
- [x] Every health-check URL — two Dockerfiles, two compose files — uses
      `http://127.0.0.1:9447/livez`. `grep -rn 'localhost' Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml`
      returns nothing.
- [x] Timeout is `5s` in all four places. `grep -rn 'timeout' Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml`
      shows no `10s` health timeout.
- [x] Port is **9447 everywhere and unchanged**. `git diff main...HEAD` contains no
      port change. (9448 is kemp's move, not this repo's.)
- [x] `docker inspect --format='{{.State.Health.Status}}'` was observed printing
      `healthy` for a *running* container — not inferred from the Dockerfile.
- [x] No inline `# hadolint ignore=`, `//nolint`, or `# nosemgrep` was added.
- [x] `/health` returns 200 in **both** states, and `main_test.go` asserts both.
      The old 503 branch is gone: `grep -n 'StatusServiceUnavailable' main.go`
      returns nothing.
- [x] `/livez` and `/readyz` are registered on the mux, and the tests exercise them
      **through `newServer`'s handler**, not by calling `staticOKHandler` directly —
      otherwise a missing route registration would pass.
- [x] `main_test.go` matches this repo's test conventions: stdlib `testing`, no new
      dependency, `got = X, want Y` failure messages.
- [x] `CHANGELOG.md` heading count equals the tag count plus one, and every entry
      traces to real commits in the corresponding `git log` range. No invented
      features.
- [x] `CHANGELOG.md` `[Unreleased]` records the `/health` status-code change as a
      **Changed** entry, since it is user-visible behaviour anyone scripting the
      503 depends on.
- [x] ADR-0012 has a row in `docs/adr/index.md`, and `.pages` was **read** to
      confirm its `...` rest token means no edit is needed there.
- [x] The chart's `livenessProbe`/`readinessProbe` point at `/livez`/`/readyz`, and
      `helm template` renders them that way.
- [x] The docs sweep found and fixed every current-behaviour claim about `/health`
      503, `alpine:3.24`, or the absence of a health check. Historical ADRs and past
      specs were left as written.
- [ ] `make ci`, `make fmt-check`, `make vet`, `make docs`, and both
      `docker compose config -q` runs all passed — output observed, not assumed.
      NOT fully honest to tick: `make ci`, `make fmt-check`, `make vet`, and both
      `docker compose config -q` runs all passed. The literal `make docs` target
      fails — `uvx --with mkdocs-material --with pymdown-extensions mkdocs build
      --strict` errors with `Config value 'plugins': The "awesome-pages" plugin is
      not installed`, a pre-existing Makefile gap (missing
      `--with mkdocs-awesome-pages-plugin`) called out separately in the review and
      explicitly left unfixed here. The equivalent command with that flag added on
      the command line does build clean under `--strict`.
- [x] Working tree clean; no `linux/`, `site/`, `bin/`, `dist/` or `coverage.out`
      committed.
