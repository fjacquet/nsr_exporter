# 0001 — Snapshot collection model

**Status**: Accepted

## Context

Prometheus scrapes and OTLP pushes can arrive more often, and from more clients, than
a NetWorker server can safely answer. Fetching from the backend on each scrape couples
backend load to scraper count and risks overwhelming the appliance — the explicit
non-goal in the design spec.

## Decision

A single background loop polls every configured system on `collection.interval`, builds
an **immutable** `Snapshot`, and atomically swaps it into a `SnapshotStore`
(`RWMutex` pointer-swap). Both export paths — the Prometheus unchecked collector and the
OTLP observable gauges — read the latest snapshot and never touch the backend. The HTTP
server starts **before** the first collection cycle so `/metrics` and `/health` respond
immediately even while the first (potentially slow) poll runs.

**Per-target graceful degradation.** Systems are polled concurrently (an `errgroup` with
`SetLimit`), but one system's failure MUST NOT fail the cycle or void the other systems'
data. Each per-system fetch is isolated: on error it contributes an
`nsr_up{system="<name>"} = 0` gauge (and emits no other series for that system) while
healthy systems contribute `nsr_up = 1` plus their full metric set. The snapshot is built
from whatever succeeded — a partial snapshot is always preferable to no snapshot. `nsr_up`
is the canonical health signal for alerting on an unreachable or failing target.

## Consequences

- Backend API load is constant (one poll per interval per system) regardless of scraper
  or OTLP-push cadence.
- Scrapes are served from memory in microseconds.
- Metrics are at most `interval` stale — acceptable for backup monitoring.
- Per-system failures degrade gracefully (`nsr_up{system}=0`) without failing the cycle.
