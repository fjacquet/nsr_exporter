# 0008 — Absent-never-zero parsing

**Status**: Accepted

## Context

Several NetWorker metrics represent operational capacity and error counts (e.g. storage
volume capacity, data-domain system used space, alert counts). When a backend field is
missing, unset, or fails to parse, there are two choices: emit `0` or emit no sample at all.
A phony `0` on a capacity or error metric silently corrupts dashboards and alerts: a
volume that "used 0 bytes" will never fire a fullness alert; a group with "0 failed jobs"
will never page on-call. Because field names are inferred from documentation (no live
appliance during development), missing-field situations are expected to occur during initial
deployment.

## Decision

Optional numeric fields use **pointer types** in the Go response structs. A `nil` pointer
after JSON decode means the field was absent or unparseable; the collector emits **no
sample** (absent metric) rather than `0`. Tolerant parsing helpers are localised to a
small set of functions in one place so corrections are a one-line change.

The rule is: **absent, never zero** for any capacity, utilisation, or error-count metric.
A missing value is silently skipped; the metric does not appear in that scrape's output.
Downstream PromQL queries on absent metrics return "no data" rather than a misleading `0`,
which correctly alerts as "data missing" rather than "everything fine".

Information-only metrics (`_info` label-set gauges with fixed value `1`, or binary
`_status` gauges) are exempt: a value of `0` is semantically meaningful for those and
there is no capacity-corruption risk.

## Consequences

- Capacity and error dashboards surface real absences rather than hiding them behind a
  false `0`.
- Incorrect field-name guesses during the inferred-API phase produce absent samples, not
  corrupt data — a wrong field shows up as "no data" in Grafana, which is immediately
  actionable.
- Every optional numeric must be reviewed and typed as `*float64` / `*int64` /
  `*string`; non-pointer is a code-review gate.
- The tolerant-parsing helpers must be the only place that touches raw JSON → numeric
  conversion; scattering ad-hoc `strconv.ParseFloat` calls is a code-review fail.
