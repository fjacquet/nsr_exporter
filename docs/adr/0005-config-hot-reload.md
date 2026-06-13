# 0005 — Config hot reload

**Status**: Accepted

## Context

Operators need to add or update NetWorker system targets (credentials, host, TLS settings)
without restarting the exporter — a restart means a missed scrape interval and a gap in
dashboard coverage. The exporter already watches the config file path and handles `SIGHUP`.

## Decision

On `SIGHUP` (or a future file-watch trigger), the exporter re-parses the config file and
validates it. If validation succeeds, a **log-only** acknowledgement is emitted for now —
live client swap is deferred to a follow-up. The running config stays active; the operator
must restart to apply changes to `systems` or `collection`. This is the MVP simplicity
choice: re-parse validates syntax and env expansion at signal time, surfacing errors before
a restart is attempted.

Live client rebuild-and-swap (building fresh `nsrclient.Client` instances and atomic store
swap of the collector) is explicitly deferred because it requires serialising concurrent
shutdown of old clients with the running `errgroup` fan-out — scope that is out of place for
an MVP. The watcher is structured so the live-swap upgrade path is a single additional step
inside the existing SIGHUP handler.

## Consequences

- Config syntax errors surface at `SIGHUP` time rather than only at restart.
- Credential and host changes still require a process restart to take effect — operators
  must document this.
- The handler is a one-goroutine watcher; serialisation is trivial.
- Upgrade to full live swap is isolated to `main.go`'s `watchReload` + the future
  `CollectorRunner` abstraction; no interface changes needed.
