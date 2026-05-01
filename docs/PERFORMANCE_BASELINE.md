# Performance Baseline

Baseline date: 2026-04-30

Scope: local Go benchmark baseline for DNS hot paths, policy matching, blocklist parsing,
SQLite operational paths, and the in-process DoH client benchmark.

ASSUMPTION: This is a repeatable local package benchmark baseline on one developer
workstation. It is not a sustained live-host load test, does not exercise real router/client
traffic, and does not prove production latency under network contention.

## Host

- OS/arch: `linux/amd64`
- CPU: `AMD Ryzen 7 PRO 6850H with Radeon Graphics`
- Go toolchain: `/tmp/sis-go1.25.9/go/bin/go`
- Command:

```sh
packages="$(go list ./... | grep -v '/webui/node_modules/')"
go test -run '^$' -bench=. -benchmem -benchtime=500ms -count=3 ${packages}
```

CI also runs the same benchmark suite with `-benchtime=100ms -count=1` on manual
release-hardening validation runs.

## Results

Representative ranges from the local 3-run baseline:

| Path | Benchmark | Range |
|---|---|---:|
| DNS cache lookup | `BenchmarkCacheHit` | 514-525 ns/op |
| DNS cache put/evict | `BenchmarkCachePutEvict` | 533-549 ns/op |
| DNS pipeline cache hit | `BenchmarkPipelineCacheHit` | 1.12-1.16 us/op |
| DNS pipeline policy block | `BenchmarkPipelinePolicyBlock` | 1.80-1.87 us/op |
| Domain suffix match | `BenchmarkDomainsMatch` | 136-138 ns/op |
| Policy evaluate | `BenchmarkPolicyEvaluate` | 747-776 ns/op |
| Policy snapshot with many lists | `BenchmarkPolicySnapshotWithManyLists` | 48.3-56.0 us/op |
| Blocklist parse | `BenchmarkParseBlocklist` | 6.46-6.66 ms/op |
| SQLite client upsert | `BenchmarkSQLiteClientUpsert` | 94.6-99.8 us/op |
| SQLite client get | `BenchmarkSQLiteClientGet` | 15.7-16.5 us/op |
| SQLite session upsert | `BenchmarkSQLiteSessionUpsert` | 112-115 us/op |
| SQLite stats put/get | `BenchmarkSQLiteStatsPutGet` | 125-128 us/op |
| SQLite config history append/list | `BenchmarkSQLiteConfigHistoryAppendList` | 133-134 us/op |
| In-process DoH forward | `BenchmarkDoHClientForward` | 64.4-66.4 us/op |

## Post-Optimization Spot Check

After replacing per-query policy list-map copies with copy-on-write engine snapshots,
`BenchmarkPolicySnapshotWithManyLists` was rerun on the same CPU with Go 1.26.2. UDP
ingress read buffers are also pooled after this baseline; the package benchmarks below do
not exercise socket read-loop allocation behavior directly.

| Path | Benchmark | Range |
|---|---|---:|
| Policy snapshot with many lists | `BenchmarkPolicySnapshotWithManyLists` | 47.4-54.5 ns/op, 48 B/op, 1 alloc/op |
| Policy evaluate | `BenchmarkPolicyEvaluate` | 4.44-5.68 us/op, 490 B/op, 10 allocs/op |

## Interpretation

- DNS cache and pipeline cache-hit paths are comfortably sub-2 us/op in process on this host;
  policy evaluation remains in the low-single-digit microsecond range depending on query mix
  and toolchain.
- Cache sharding is not implemented because the current package benchmark baseline does not
  show lock contention; revisit with sustained live-host concurrency profiles.
- Live top-domain/client maps are bounded with low-frequency pruning to avoid unbounded memory
  growth under very high-cardinality traffic.
- SQLite operational writes are sub-140 us/op in isolated package benchmarks.
- Blocklist parsing remains the slowest measured local path and is background/sync-time work,
  not per-query work.
- Allocation counts are non-zero on DNS/policy hot paths; reducing per-query allocations
  remains the main future performance opportunity.

## Remaining Load Work

1. Use `scripts/local-load.sh` for a repeatable local DNS/API load smoke while developing.
2. Run sustained DNS UDP/TCP and authenticated API load against a real target host.
3. Record QPS, latency percentiles, CPU, memory, goroutine count, and error/rate-limit totals.
4. Repeat against both JSON and SQLite backends when store-heavy API paths are in scope.
5. Import live results into `docs/PRODUCTION_VALIDATION.md` before any broad production claim.
