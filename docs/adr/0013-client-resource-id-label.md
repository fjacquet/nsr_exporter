# 0013 — `resource_id` label on client metrics

**Status**: Accepted

## Context

NetWorker lets one host be registered as several distinct **client resources** —
e.g. multiple parallel-backup configurations for the same hostname, each with its
own save sets/tags. `nsr_client_info` and `nsr_client_parallelism` were labeled only
by `client_name` (the hostname), so two client resources sharing a hostname produced
two series with identical name **and** label values. `prometheus/client_golang`'s
`Registry.Gather` rejects that as a duplicate metric ("collected metric ... was
collected before with the same name and label values"), turning every scrape into an
HTTP 500 — reported in issue #36.

## Decision

Add a `resource_id` label to both `nsr_client_info` and `nsr_client_parallelism`,
sourced from the client resource's `resourceId.id` (swagger `Client.resourceId`,
`$ref: ResourceId`): "the id attribute of the client resource's resourceId... [which]
uniquely identifies a client resource instance." This is the same family pattern
`ppdm_exporter` uses for its `activities` collector (`activity_id` alongside
descriptive labels) to keep series unique per resource instance rather than per
display name.

`resource_id` is appended right after `client_name` on both metrics (label-key
invariant, ADR-0006) and requested via `fl=...,resourceId` alongside the existing
projection. An older NetWorker version that omits the field yields an empty-string
label value — a label, not a numeric sample, so ADR-0008 (absent, never zero) does
not apply.

**Breaking change**: any dashboard, alert, or scrape-side aggregation that grouped
`nsr_client_info`/`nsr_client_parallelism` by `client_name` alone will now see
multiple series per hostname where multiple client resources exist. The shipped
Grafana dashboard (`grafana/dashboards/nsr-activity.json`) and `docs/metrics.md`
were updated in the same change.

## Consequences

- The exporter no longer 500s when a NetWorker fleet has multiple client resources
  per hostname (the common case for large parallel-stream backups).
- Every `nsr_client_info`/`nsr_client_parallelism` series unconditionally carries
  `resource_id`, including single-resource-per-host fleets, where it is simply
  unique per host and mostly redundant with `client_name` for filtering.
- Consumers that assumed `client_name` alone was unique per series must add
  `resource_id` to their grouping/legend.
