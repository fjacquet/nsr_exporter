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
