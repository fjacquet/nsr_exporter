# Scale baseline: 5,000 clients (before bounding `/jobs`)

Measured on the unmodified, unbounded `/jobs` code (base `4d37456` plus the benchmark
double-gather fix `a5081e0`). Fixture: 5,000 clients, 10,000 sessions, 50,000 jobs spread over
72h, served by `scaleMock` over loopback `httptest` (no network latency).

## Machine

- Apple M1 Pro, 10 cores, 16 GiB RAM, darwin/arm64
- go1.27.3

## Commands

```
go test ./internal/nsr/ -run '^$' -bench . -benchmem -count 3 -timeout 20m
go test -c -o nsr.test ./internal/nsr/
/usr/bin/time -l ./nsr.test -test.run '^$' -test.bench . -test.benchmem -test.count 3
```

Sizes sweep: a throwaway test (not committed) ran `scaleMock` with 5,000 clients, 10,000
sessions and jobs in {0, 4167, 8333, 12500, 16667, 25000, 50000}, one `CollectOnce`, then three
renders through the `promhttp` handler (best of 3).

## Raw benchmark output (WARN log lines removed)

```
BenchmarkCollectAndGather-10   2   795357875 ns/op   22086548 body_bytes   716625444 B/op   9507850 allocs/op
BenchmarkCollectAndGather-10   2   791086625 ns/op   22086672 body_bytes   716542904 B/op   9507761 allocs/op
BenchmarkCollectAndGather-10   2   759997542 ns/op   22086521 body_bytes   716543624 B/op   9507732 allocs/op
BenchmarkGatherOnly-10         4   264519354 ns/op                         239114788 B/op   5806696 allocs/op
BenchmarkGatherOnly-10         4   261799771 ns/op                         239114828 B/op   5806696 allocs/op
BenchmarkGatherOnly-10         4   303702364 ns/op                         239114912 B/op   5806697 allocs/op
```

Peak memory (`/usr/bin/time -l`, whole benchmark run): maximum resident set size 465,305,600 B
(about 444 MiB); peak memory footprint 446,891,064 B (about 426 MiB).

## Which benchmark is which

- `BenchmarkCollectAndGather` = one full cycle (fetch 3 big endpoints + decode + build snapshot
  + swap) plus one `/metrics` render (gather and text-encode). About 0.78 s/op.
- `BenchmarkGatherOnly` = `Registry.Gather()` from the cached snapshot, no HTTP encoding. About
  0.26 to 0.30 s/op. It under-states the scrape path.
- The HTTP scrape path is the `promhttp` handler: gather plus encode. The sweep measured it at
  376 ms best-of-3 for 50,000 jobs (collect for the same run: 403 ms). The scrape never fetches.
  Derived: 0.78 s minus about 0.40 s collect is about 0.38 s render, consistent.

## Measured vs thresholds (spec section C)

| Threshold | Limit | Measured | Result |
|---|---|---|---|
| Cycle processing < 50% of 60s timeout | < 30 s | about 0.40 s collect (0.78 s collect + render) | PASS |
| `/metrics` answers | < 1 s | about 0.38 s render (0.26 to 0.30 s gather only) | PASS |
| `/metrics` body | < 20 MB | 22,086,697 B = 21.1 MiB (22.1 MB) | FAIL |

Only the body-size threshold fails. The cause is `/jobs` being unbounded: 50,000 jobs emit
150,000 `job_id` series which are 85% of the body (19.0 of 22.1 MB). Time is nowhere near the
limits on this machine with zero network latency; real latency and a slower host would add to the
cycle but would need a 75x margin to reach 30 s. Memory is also notable: 716 MB allocated per
cycle+render, 445 MB peak RSS.

## Body size per job

Body bytes by job count (5,000 clients, 10,000 sessions fixed):

| Jobs | Body bytes | MB |
|---|---|---|
| 0 | 3,055,692 | 3.06 |
| 4,167 (about 6h window) | 4,628,988 | 4.63 |
| 8,333 (about 12h window) | 6,204,885 | 6.20 |
| 12,500 | 7,788,658 | 7.79 |
| 16,667 (about 24h window) | 9,377,448 | 9.38 |
| 25,000 | 12,554,598 | 12.55 |
| 50,000 (72h, unbounded) | 22,086,697 | 22.09 |

Slope: (22,086,697 - 3,055,692) / 50,000 = 380.6 bytes per job, about 127 bytes per series
(3 `job_id` series per job). Marginal slope between 4,167 and 8,333 is 378 B/job, so it is linear.
The spec's 400 to 500 bytes per job estimate is slightly high: measured 381 with this fixture's
short labels (`policyN`, `clientNNNNN.example.com`). Real policy and client names are longer, so
expect 400 to 500 in production; the estimate is reasonable but not conservative for the fixture.

Baseline without jobs is already 3.06 MB (clients, sessions, other families).

Extrapolation: a 24h window (about 16,667 jobs) gives about 9.4 MB, 12h about 6.2 MB, 6h about
4.6 MB, all well under 20 MB (even at 500 B/job, 24h is about 11.4 MB). The 20 MB cap at 381
B/job is reached at about 44,500 jobs; at 500 B/job at about 34,000 jobs. Collect and render times
scale linearly too (24h: collect about 0.18 s, render about 0.14 s).

## Notes

- Fixture jobs are all in the same state/status, so series cardinality per job is the
  worst case of one unique `job_id` each, which is what production does.
- Loopback only: real REST latency for the 3 large responses is not included.
