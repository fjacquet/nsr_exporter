# 0006 — Label-key consistency invariant

**Status**: Accepted

## Context

The `/metrics` handler is an *unchecked* Prometheus collector (its `Describe` emits
nothing) so that the set of metric names can vary per snapshot (see ADR-0001). The
trade-off: `client_golang` does **not** enforce a consistent variable-label-key set per
metric name for an unchecked collector. If two samples share the same metric name but carry
different label keys, a checked registry would fail `Gather`/scrape with an "inconsistent
label names" error. We must guarantee consistency ourselves.

## Decision

A metric name carries **exactly one label-key set** across all of its series. The rule is
enforced two ways:

1. **Statically**: `TestCatalogCoversAllEmittedMetrics` (and related label-parity tests) run
   every collector against fixtures and fail the build if any metric name appears with two
   different label-key sets.
2. **Union label set for two-path metrics**: when a metric is emitted by both Prometheus and
   OTLP paths and the label sets would otherwise differ, the union label set is used in
   canonical order, with empty string values for keys that are inapplicable to a given
   series. This keeps both paths consistent without special-casing.
3. **At runtime**: `PromCollector.Collect` records the first label-key set seen for each
   metric name within a scrape and drops any sample whose keys disagree — a safety net so
   that a stray inconsistency never breaks the whole scrape.

## Consequences

- Exported series shape is stable; scrapes cannot fail due to per-sample label-key drift.
- The invariant is caught at build time when adding or altering a collector, not in
  production.
- A genuinely divergent sample is silently dropped at runtime (the static test is the real
  gate; the runtime guard is a defensive belt-and-suspenders).
- Collectors must set labels in a fixed order per metric name; `WithSystem` prepends the
  `system` label consistently across all series.
