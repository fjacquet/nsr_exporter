# 0002 — Modular resource collectors

**Status**: Accepted

## Context

The exporter covers multiple NetWorker metric domains (alerts, clients, jobs, sessions,
storage, sizing), and the API field mappings are inferred from documentation only, so
individual endpoints and JSON fields are expected to need correction as they are validated
against real appliances. We need a structure that isolates that risk, allows domains to be
added or corrected independently, and keeps a single failing domain from taking down the
whole collection cycle.

## Decision

Each metric domain implements the `ResourceCollector` interface (`internal/nsr/resource.go`)
and owns its endpoint path, response struct, and parse → `[]Sample` logic. Collectors are
composed in `DefaultCollectors()`, and the per-system cycle iterates them via an `errgroup`
fan-out. A domain failure degrades gracefully — the collector emits `nsr_up{system}=0` rather
than crashing the cycle. Adding a new domain is a fixed checklist: one file implementing
`ResourceCollector`, one entry in `catalog.go`, one fixture in `cmd/mocknw`, and one
dual-export test.

## Consequences

- Each API correction is contained to one file plus one `testdata/` fixture — corrections
  are localized.
- Domains can be phased in and tested in isolation via `httptest`-driven dual-export tests
  (Prometheus gather + OTLP `ManualReader`).
- Per-domain failures are observable in-band via `nsr_up`.
- A small amount of per-domain boilerplate is required (struct + parse + registration +
  fixture), but the pattern is consistent and the checklist makes it mechanical.
