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

Peak memory (`/usr/bin/time -l`, covers the whole benchmark run): maximum resident set size
465,305,600 B = 444 MiB (peak memory footprint 446,891,064 B = 426 MiB).

Raw sweep lines are in the "After" section below.

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
150,000 `job_id` series which are 86% of the body (19.0 of 22.1 MB). Time is nowhere near the
limits on this machine with zero network latency; real latency and a slower host would add to the
cycle but would need a 75x margin to reach 30 s. Memory is also notable: 716 MB allocated per
cycle+render; peak RSS about 444 MiB (whole benchmark run, 3 iterations of both benchmarks).
Gzip, which Prometheus scrapes normally use, is not measured: all sizes here are uncompressed.

## Body size per job

Body bytes by job count (5,000 clients, 10,000 sessions fixed):

| Jobs | Body bytes | MB |
|---|---|---|
| 0 | 3,055,692 | 3.06 |
| 4,167 (about 6h window) | 4,628,988 | 4.63 |
| 8,333 (about 12h window) | 6,204,885 | 6.20 |
| 12,500 (about 18h window) | 7,788,658 | 7.79 |
| 16,667 (about 24h window) | 9,377,448 | 9.38 |
| 25,000 (about 36h window) | 12,554,598 | 12.55 |
| 50,000 (72h, unbounded) | 22,086,697 | 22.09 |

Window labels assume the 72h-spread fixture (50,000 jobs over 72h, so 694 jobs per hour).

Slope: (22,086,697 - 3,055,692) / 50,000 = 380.6 bytes per job, about 127 bytes per series
(3 `job_id` series per job). Marginal slope between 4,167 and 8,333 is 378 B/job, so it is linear.
The measured 381 B/job sits below the spec's 400 to 500 B estimate because this fixture's labels
are short (`policyN`, `clientNNNNN.example.com`). That longer production labels would land in the
400 to 500 range is a guess, not a measurement.

Baseline without jobs is already 3.06 MB (clients, sessions, other families).

Extrapolation: a 24h window (about 16,667 jobs) gives about 9.4 MB, 12h about 6.2 MB, 6h about
4.6 MB, all well under 20 MB (even at 500 B/job, 24h is about 11.4 MB). The 20 MB cap at 381
B/job is reached at about 44,500 jobs; at 500 B/job at about 34,000 jobs. Collect and render times
scale linearly too (24h: collect about 0.18 s, render about 0.14 s).

## Notes

- Fixture jobs are all in the same state/status, so series cardinality per job is the
  worst case of one unique `job_id` each, which is what production does.
- Loopback only: real REST latency for the 3 large responses is not included.

# After: bounded `/jobs` (`collection.jobWindow`, default 24h)

Same machine, fixture and sizes as above, with `JobsCollector` cutting `/jobs` to the last 24h on
the real clock. Code at `adc4604` (adds `TestScaleBudget_5000Clients`).

Fixture fix made on the way: `scaleMock` computed `time.Duration(i) * 72h`, which overflows
int64 above about 35,500 jobs, so jobs 35,600 and up got negative offsets (start times in the
future) and a 24h cut kept 31,083 jobs instead of 16,667. The baseline above was unbounded, so
its body sizes and counts are unaffected; the "After" numbers are with the fixed spread.

## Raw benchmark output (WARN log lines removed; benchmark names are dropped by the output filter)

Commands: `go test ./internal/nsr/ -run '^$' -bench . -benchmem -count 3 -timeout 20m`. The first
three rows are `BenchmarkCollectAndGather`, the last three `BenchmarkGatherOnly` (no
`body_bytes` column).

```
       3	 345605195 ns/op	   9377473 body_bytes	325768970 B/op	 4173620 allocs/op
       3	 335089708 ns/op	   9377408 body_bytes	325728925 B/op	 4173652 allocs/op
       3	 334344639 ns/op	   9377475 body_bytes	325683976 B/op	 4173610 allocs/op
      12	  92638726 ns/op	100794856 B/op	 2439535 allocs/op
      13	  91487407 ns/op	100794694 B/op	 2439535 allocs/op
      12	  90889549 ns/op	100794728 B/op	 2439535 allocs/op
```

`TestScaleBudget_5000Clients` log line (one run): `collect=220.7ms render=141.4ms
body=9377418 bytes`. Another run: `collect=301ms render=242ms` (before the fixture fix, so
not comparable). Collect and render vary 0.2 to 0.3 s between runs.

Peak memory (`/usr/bin/time -l`, whole benchmark run, 3 iterations of both benchmarks): maximum
resident set size 261,947,392 B = 250 MiB (peak memory footprint 246,203,280 B = 235 MiB).
Before: 444 MiB.

## Raw sweep lines (throwaway helper, deleted; unbounded `/jobs`, 5,000 clients, 10,000 sessions)

```
SWEEP jobs=4167 body_bytes=4628956
SWEEP jobs=8333 body_bytes=6204875
SWEEP jobs=12500 body_bytes=7788696
SWEEP jobs=16667 body_bytes=9377442
SWEEP jobs=25000 body_bytes=12554688
```

These re-run values differ from the table above by under 100 B (timestamp formatting noise);
the 0 and 50,000 points were not re-run.

## Measured vs thresholds (spec section C), 24h window

| Threshold | Limit | Before | After | Result |
|---|---|---|---|---|
| Cycle processing < 50% of 60s timeout | < 30 s | about 0.40 s | about 0.22 s collect (0.33 to 0.35 s collect + render) | PASS |
| `/metrics` answers | < 1 s | about 0.38 s | about 0.14 s render (0.09 s gather only) | PASS |
| `/metrics` body | < 20 MB | 22,086,697 B (21.1 MiB) | 9,377,418 B = 8.9 MiB (9.4 MB) | PASS |

Allocation per cycle+render fell from 717 MB to 326 MB. Loopback only: no network latency, no
gzip, one machine.

## Recommended `jobWindow` for a 5,000-client server

Keep the 24h default: it holds about 16,700 jobs (at this fixture's rate of 694 jobs/h) and gives
9.4 MB, less than half the cap. By extrapolation at 381 B/job the cap is reached at about 44,500
jobs for 20 MB (47,000 for 20 MiB; the test asserts the 20 MB figure), about 63 to 68h of this fixture. So
raising the window to 48h (about 33,000 jobs, about 15.6 MB by extrapolation) is still under the
cap, and 72h is not. Caveats: the job rate is the fixture's (50,000 jobs per 72h), not a
measurement of any real server; labels in production may be longer than the fixture's, so scale
the job count by real bytes per job from `/metrics`; all numbers are loopback only. Live
validation of the `startTime` filter is still pending.

## Decision: conditional levers not built

The spec says to build levers only as far as a missed threshold requires. All three thresholds
pass at the 24h window, so concurrent per-system collectors, a `prometheus.Desc` cache and
`PageSize` chunking are not built.

How to tell in production that they are needed:

- `nsr_collection_duration_seconds` approaching `collection.timeout` (for example above half of
  it): concurrent collectors, or a shorter `jobWindow`.
- `/metrics` latency near 1 s or a growing body: shorten `jobWindow`, then consider the `Desc`
  cache.
- A single `/jobs` request timing out on a real server: then `PageSize` chunking, only with that
  measurement in hand.
