# Local performance regression workload

This is a reproducible development workload, not a supported household count or
measurement of production. Tests use synthetic data in a randomly named database
created from `TEST_DATABASE_URL`, and drop only that database when finished.

The reference host is an AMD Ryzen 7 5700X3D virtualized Linux environment with
16 visible logical CPUs and 15 GiB host RAM. PostgreSQL 17 runs in Podman; the
application tests use Go 1.26.8. This does not model production CPU quotas, storage,
network round trips, cold caches, or multi-device response times.

Run:

```bash
NABU_PERF=1 NABU_PERF_ENFORCE=1 \
  NABU_PERF_REPORT=/tmp/nabu-history-workload.json \
  TEST_DATABASE_URL='postgres://nabu:nabu@localhost:5432/nabu?sslmode=disable' \
  go test -v -timeout 360s ./internal/log -run TestLocalHistoryWorkload
```

The starting fixture has ten households, 25 chores each (one private), 50,000 logs
per household over roughly 2.85 years, amounts, JSON indicators, duration, and
searchable notes. Query measurements use 20 samples after a warm-up. Reports
include the dataset size, nearest-rank p95, query count, allocation bytes/objects,
`EXPLAIN (ANALYZE, BUFFERS)` and pool waits. Both latest-query implementations use
the same permissions and return 24 visible results. Write phases each add five
2,000-row batches; the fixture grows to 530,000 logs. This modest growth and warmed
caches are limits of the comparison, not a controlled production write benchmark.

The calibrated run (`/tmp/nabu-history-workload-v3.json`, exit 0) measured:

| Read | Before median | After median | After p95 |
| --- | ---: | ---: | ---: |
| Latest per visible chore | 63.0 ms (DISTINCT/sort) | 0.93 ms (indexed lateral) | 1.07 ms |
| Missing substring search | 47.7 ms | 0.75 ms (GIN) | 0.91 ms |
| Rare substring search | 47.5 ms | 2.99 ms (GIN) | 3.16 ms |
| Full log hydration / grouped counts | 236.3 ms | 11.85 ms | 12.24 ms |

Full hydration allocated about 117 MB per call; grouped counts allocated about
15 KB and returned only scalar groups. This is total Go allocation, not peak live
heap or an end-to-end HTTP allocation claim. Each measured read uses one query.
The indexed latest plan reads one log per visible chore. Substring plans use the
GIN index; one- and two-character terms can still require broad scans because
there may be no usable trigram. Search semantics remain literal, case-insensitive
note/title substring matching across all history. [PostgreSQL pg_trgm](https://www.postgresql.org/docs/17/pgtrgm.html).

Median 2,000-row write batches were 124.6 ms before these indexes, 127.8 ms with the
latest index, and 148.1 ms with both (about 19% over baseline). All log indexes
occupied about 128 MB at the end; that total includes pre-existing indexes.

The concurrent fixture uses a 12-connection pool and two queries per operation
(latest plus counts). Ten workers completed 200 operations with a 35.2 ms p95 and
no waits. Twenty-four workers completed 480 operations with a 77.0 ms p95, 934 pool
acquisitions that waited, and 10.2 seconds of cumulative wait time. Cumulative wait
sums simultaneous callers and exceeds the 0.92-second wall time of that phase.
It is useful pressure telemetry, not evidence for raising production pool size.

Regression budgets on this reference host are p95 <=100 ms for indexed latest and
search, <=250 ms for grouped counts, and <=1 second for the paired concurrent
operation. `NABU_PERF_ENFORCE=1` enables these latency checks. Every opt-in run also
checks the hardware-independent limits: one query and <=1 MiB allocated per
optimized read; two queries per paired operation. Final acceptance additionally
requires the PostgreSQL stats-versus-memory semantic tests, visibility failure and
revocation cases, and normal browser tests. A fast empty or unauthorized result is
not a passing optimization.


## Reminder workload and pool reservation

The normal PostgreSQL integration lane also runs `TestPostgresSchedulerCandidateBudget`:
60 households, six members each, 12 schedules each and enabled per-chore
preferences. With no due reminders it checks exactly 17 candidate queries for
4,320 eligible pairs. With one due assigned schedule per household it checks 60
deliveries and at most 380 queries, including live authorization, independent
transactions and dedup writes. A repeat tick must issue only page queries and
must not send again. `NABU_SCHEDULER_REPORT=/tmp/nabu-scheduler-workload.json`
optionally records the three tick reports.

The reference race run measured 309 ms for the first tick, 595 ms for 60 due
deliveries and 268 ms for the repeat. The fake provider returns immediately;
these are not real push latency or delivery guarantees. Exact local-day
recurrence remains in Go; the SQL pages select eligible candidates and bulk-load
all scalar preferences. Tick limits are 25 seconds, 8,192 candidates and 64
attempts. A stable cursor resumes remaining candidates at the next tick instead
of continually retrying the first failing recipient. Five potentially relevant
sent dates per candidate bound historical dedup payloads.

`TestReservedDeliveryPoolKeepsHTTPAvailableAndReleasesLeader` pauses three
providers inside actual nested membership/session transactions, with a real
advisory leader lock. Seven of eight delivery connections are held; 30 real HTTP
requests that query the separate HTTP pool complete with no pool waits. The
reference maximum request was 2.46 ms. The test cancels and joins providers,
checks zero held delivery connections, and verifies follower takeover. The
application-level `TestHTTPLogReadsDoNotUseReservedDeliveryPool` additionally
occupies all delivery slots and checks current/latest/history/search routes.

`DB_MAX_OPEN_CONNS` is a **total per-process budget**, default 25. Eight slots are
reserved for delivery, leaving 17 for HTTP by default. This preserves capacity
for nested guards without increasing the previous default total. The lower bound
is 10. Both pools report their own use/waits/query timing. Multiply the budget by
replicas and include administration/recovery connections when planning the
PostgreSQL server limit. Repeated `budget_reached=true`, increasing lag, or pool
waits require workload investigation; this fixture does not establish a supported
household count.

## Notification history

`TestLocalNotificationWorkload` creates 500,000 synthetic notifications for ten
recipients, with 10% unread, then measures 20 warmed reads and five 1,000-row write
batches per index phase. Run it using the same isolated `TEST_DATABASE_URL` setup:

```sh
NABU_PERF=1 NABU_PERF_ENFORCE=1 NABU_PERF_REPORT=/tmp/nabu-notification-workload.json \
  go test -timeout 4m ./internal/notification -run '^TestLocalNotificationWorkload$' -count=1 -v
```

The local sample (`/tmp/nabu-notification-workload-v1.json`) measured a two-query
cursor page plus exact unread count at 12.89 ms p95 before matching indexes and
1.66 ms afterward. Deep offset pagination took 32.30 ms p95. Unread count alone
fell from 5.42 to 0.80 ms p95. Pages allocated about 38 KB and used exactly two
queries; no pool waits occurred. The enforced local budget is 100 ms p95 and
1 MiB allocated per page. These timings do not establish production capacity.

The ordered `(user_id, created_at DESC, id DESC)` index supports stable cursor
seeks, including tied timestamps and a deleted boundary row. The partial unread
index avoids examining read notifications for counts. The fixture's 1,000-row
inserts rose from about 25 ms to 28–30 ms with both indexes, and total notification
indexes occupied about 34.4 MiB. Migration 049 adds the measured indexes, with
bounded startup locks and an invalid-prebuild check; see the deploy runbook for
optional concurrent prebuilding on large databases. No production index was built.

## Stats navigation with four months of activity

`TestLocalStatsWorkload` grows an isolated household through 6,000, 24,000 and
60,000 activities across 120 days, four authors and 25 chores (one private).
Entries include long notes, indicators, per-indicator amounts and duration.
It compares the original full-log reader with the optimized Stats readers and
requires identical results at each size. No production data is used.

```sh
make perf-stats
```

Run from a worktree with local PostgreSQL already running (`make local` starts
the development stack). The target defaults to the local stack's
`postgres://nabu:nabu@localhost:5432/nabu?sslmode=disable` connection and writes
`/tmp/nabu-stats-workload.json`. Override `TEST_DATABASE_URL` or
`NABU_PERF_REPORT` through the environment or Make command-line assignments.
The database role needs `CREATEDB`; the workload creates and removes its own
randomly named database. An empty connection setting fails instead of silently
skipping the test.

The target always runs fresh measurements with latency, allocation and query
budgets enforced. It is an optional local check before pushing; `make test`,
the pre-push hook and CI do not run this large workload automatically. Browser
responsiveness regressions remain part of `make e2e`.

On a macOS host with Go 1.26.8 (darwin/amd64) and PostgreSQL 17 in a Podman VM
with six CPUs and 16 GiB, the 60,000-activity run measured 20 warmed calls:

| Stats service read | Original median | Optimized median | Optimized p95 | Original / optimized Go allocation |
| --- | ---: | ---: | ---: | ---: |
| Daily chore time series | 348.2 ms | 4.6 ms | 5.1 ms | 236.4 MB / 0.54 MB |
| Three-month heatmap | 281.3 ms | 22.7 ms | 24.1 ms | 187.6 MB / 0.15 MB |

The original daily chart hydrates all visible household activity over a year for
each chore. The optimized reader uses the existing household/chore/completion
index to read only metrics for that chore's displayed buckets and member totals
(280 rows in the final fixture). It preserves the separate, widened canonical
date window, so older `log_date` values do not silently drop recent completions.
Heatmap and busy-hour counts are grouped in PostgreSQL. Remaining detailed Stats
reads omit notes, titles and other unrelated fields, and feeding-gap reads select
only the requested chore and completion interval.

Reports include allocations, query count, pool waits, and `EXPLAIN ANALYZE EXECUTE`
of the actual warmed prepared statement. A fresh EXPLAIN can conceal a slow
generic plan: the unmaterialized aggregate performed roughly 600,000 membership
lookups at 60,000 activities. Materializing the live permitted-chore set once per
aggregate query bounds permission work independently of activity count. It does
not cache permissions between requests. The fixture deliberately leaves metadata
tables unanalyzed to exercise this adverse planning case. SQL/memory differential
tests cover DST, exact end bounds, mismatched canonical dates, metric replacement,
member attribution, visibility changes and independently revoked membership.

Optimized workload reads must allocate at most 2 MiB and use at most five queries
(including service metadata/permission reads). `NABU_PERF_ENFORCE=1` additionally
checks p95 <=250 ms. These local service timings exclude HTTP middleware, network
latency, concurrent navigation, and browser rendering; they are not production
latency or capacity guarantees. The PWA regression `stats-performance.spec.js`
holds an overview or time-series response open and verifies that the page and
ready charts render, customization works, and navigation remains usable. Stable
section containers also preserve a focused control's identity as loading messages
disappear and previously empty sections fill in.

A separate local Chromium navigation comparison used 60,000 activities, four
authors, 25 amount chores and the same 22 Stats requests before and after. Six
reload-and-navigate samples were collected, discarding the first as warm-up. At a
390×844 viewport, median navigation-to-page DOM dropped from 1,184.5 ms to 4.3 ms;
the first populated chore card appeared at 58.8 ms and all charts completed at
88.5 ms. This is a local synthetic DOM timing, with service workers blocked and
no CPU/network throttling; it does not measure production or a mobile device.
