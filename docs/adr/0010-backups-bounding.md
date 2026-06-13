# 0010 — Bounding the /backups sizing query

**Status**: Accepted

## Context

The sizing collector (design spec §5.6) reads `GET /backups` to compute capacity
forecasting metrics (FETB, change size, retention). Unlike `/clients` or `/volumes`,
`/backups` is the **entire backup catalog** — on a production NetWorker server this can
hold millions of save sets. Fetching it whole every `collection.interval` would hammer
the server and exhaust the exporter's memory, defeating the snapshot model's entire
purpose (decoupling backend load from scrape load).

## Decision

The sizing collector **never fetches the full catalog**. It bounds the query two ways:

1. **Time window** — a `collection.backupWindow` config (default `24h`) drives a NetWorker
   `q=savetime>'<now-window>'` server-side filter, so only recently-written save sets are
   returned. The window is the operator's knob for the freshness/cost trade-off.
2. **Field projection** — `fl=client,name,level,size,saveTime,retentionTime,pool,duration`
   returns only the fields the metrics need.

Aggregation (max Full size per client/saveset for FETB, etc.) happens in-process. The
`SizingCollector` is constructed with the window and a clock so the filter is always
applied; the filter-building logic is isolated in a single `backupWindowFilter` function.

The exact NetWorker `q=` savetime syntax and the `level` vocabulary (full/incr/1..9) are
**INFERRED** and flagged for live validation; isolating the filter in one function means a
correction is a one-line change with a pinning unit test (`TestBackupWindowFilter`).

## Consequences

- The exporter is safe to run against large production catalogs.
- Sizing metrics reflect the trailing `backupWindow`, not all-time history — appropriate
  for capacity-trend monitoring; operators widen the window if they need a longer view.
- The inferred filter syntax is the highest-priority live-validation item.
