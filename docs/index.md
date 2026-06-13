# nsr_exporter

A **Prometheus + OTLP** metrics exporter for **Dell EMC NetWorker** backup servers.

A single process polls every configured NetWorker system on a background interval,
caches an immutable snapshot, and serves `/metrics` (and an OTLP push) instantly from
cache — so scrapes never reach the backend.

## Architecture

```
collection loop → fetch all systems → build immutable Snapshot → SnapshotStore.Swap()
                                                                    ├── PromCollector  (/metrics)
                                                                    └── OTLPExporter   (push)
```

- **[Metrics catalog](metrics.md)** — every metric, its type, labels, and meaning.
- **[Architecture decisions](adr/index.md)** — the load-bearing design choices.

## Authentication

NetWorker authenticates with HTTP Basic on every request against
`/nwrestapi/v3/global`. See [ADR-0007](adr/0007-http-basic-auth.md).

## Field-name validation

Several collectors use field names inferred from the NetWorker REST convention (marked
`// INFERRED` in code). Confirm them against a live appliance with:

```bash
nsr_exporter --config real.yaml --once --debug --trace 2>trace.log | sort > samples.txt
```

A wrong guess surfaces as a *missing* metric in the dump, never a false zero.
