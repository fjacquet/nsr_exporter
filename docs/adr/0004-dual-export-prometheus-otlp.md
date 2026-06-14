# 0004 — Dual export: Prometheus and OTLP from one snapshot

**Status**: Accepted

## Context

The design spec describes only the Prometheus `/metrics` path. The exporter-standards
family, however, requires every exporter to *also* push metrics over OTLP, so the same
data lands in OpenTelemetry-native backends without a separate scrape. Bolting OTLP on as
a second, independently-fetching path would double backend load and risk the two paths
disagreeing — a series present on `/metrics` but absent from the OTLP push, or carrying
different label keys. Both paths must therefore derive from the **same** source of truth.

## Decision

Both export paths read the single immutable `Snapshot` produced by the collection loop
(ADR-0001); neither ever touches the backend.

- **Prometheus** — an unchecked collector (ADR-0006) whose `Collect` walks the latest
  snapshot and emits gauges on each scrape.
- **OTLP** — `NewOTLPExporter(store, reader)` registers one async callback
  (`meter.RegisterCallback`) that, on every collection by the SDK reader, reads the latest
  snapshot and observes the same gauges via `Float64ObservableGauge`. The reader is
  **injectable**: a `PeriodicReader` wrapping the `otlpmetricgrpc` exporter in production,
  a `ManualReader` in tests.

Per-second values are observable **gauges**, not counters (ADR-0009): both paths emit
identical names, units, and label sets. The label-key consistency invariant (ADR-0006)
applies across paths — where the two would otherwise differ, the union label set in
canonical order is emitted with empty values for inapplicable keys.

**Test invariant**: collector tests assert behaviour through **both** paths — the
Prometheus registry `Gather` **and** an OTLP `ManualReader.Collect` — so a regression in
either path fails the build.

## Consequences

- One poll per interval per system feeds both export paths; OTLP adds no backend load.
- The two paths cannot silently diverge: shared snapshot, shared metadata, and a
  two-path test gate enforce parity.
- OTLP is push-based and self-paced by the `PeriodicReader`; Prometheus stays pull-based.
  Both are at most `interval` stale, consistent with ADR-0001.
- Adding a metric means wiring it once into the snapshot projection; both paths pick it up.
