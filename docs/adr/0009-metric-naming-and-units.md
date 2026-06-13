# 0009 — Metric naming & units

**Status**: Accepted

## Context

Prometheus metric naming has strong conventions: unit-explicit suffixes, base SI units
(bytes not megabytes, seconds not milliseconds), and a consistent prefix per exporter.
In addition, NetWorker exposes throughput and ingest-rate values that are already
per-second figures from the server — their semantics differ from raw counters and using
`rate()` on them in PromQL would double-differentiate. Naming them ambiguously risks
dashboard authors applying the wrong aggregation and silently producing nonsense values.

## Decision

All metrics follow these rules:

1. **Prefix**: `nsr_` on every metric name.
2. **Port**: `9097` (registered for the exporter family). This is the exporter's own
   `/metrics` port; the NetWorker REST API uses `9090`.
3. **Unit-explicit suffixes**: `_bytes`, `_seconds`, `_bytes_per_second`, `_total` (for
   true counters). Never expose megabytes, minutes, or percentages as a raw gauge without
   a suffix.
4. **Per-second values are gauges, not counters**. NetWorker returns throughput and ingest
   rate as already-computed per-second figures. These are emitted as `Gauge` type. The
   correct PromQL aggregation is `sum()` or `avg()`, never `rate()`. Metric names carry
   `_per_second` to make this unambiguous.
5. **Base units**: sizes in bytes (not KB/MB/GB), durations in seconds (not ms), rates in
   bytes/second.
6. **Counter discipline**: only monotonically increasing values use `_total` and Counter
   type. Anything that can decrease (free space, current session count) is a Gauge.

## Consequences

- Dashboard authors have a clear signal about which aggregation to use (`sum`/`avg` vs
  `rate`) from the metric name alone.
- SI units in metric names allow direct use with Grafana unit auto-formatting.
- `_per_second` suffix makes the "already a rate" semantics visible without reading the
  exporter source.
- Adding a new metric requires one decision: is this a per-second pre-computed value (→
  Gauge + `_per_second`) or a raw counter (→ Counter + `_total`) or a snapshot value (→
  Gauge with unit suffix)?
