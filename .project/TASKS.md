# Sis — Tasks

This document breaks the v1 build into discrete, reviewable tasks. Each task is sized to a single pull request: small enough to review in one sitting, large enough to be a meaningful unit. Tasks are grouped into milestones; milestones map roughly to sprints.

- **Total tasks:** 78
- **Total estimate:** ~9,200 LOC (Go) + ~3,800 LOC (TypeScript/TSX)
- **Total effort:** ~340 engineering hours
- **Milestones:** 11
- **Suggested sprints:** 5 (≈ 2 weeks each, single full-time developer)

---

## Conventions

- **ID:** `T###` — stable, never renumbered.
- **Title:** present tense, imperative. ("Implement", "Add", "Wire up".)
- **Scope:** what's in scope. If a bullet is here, it ships in this task; if it isn't, it doesn't.
- **Deps:** task IDs that must be merged before this one starts.
- **Est:** estimated hours (e × LOC ≈ hours; rough but consistent).
- **Acceptance:** observable criteria. If `make test` passes and these criteria hold, the task is done.

---

## Sprint / Milestone Map

| Sprint | Milestones                              | Tasks       | Hours |
|--------|------------------------------------------|-------------|-------|
| 1      | M1 Foundation, M2 DNS Core (start)       | T001–T015   | 64    |
| 2      | M2 finish, M3 Policy, M4 Upstream        | T016–T033   | 72    |
| 3      | M5 Client ID, M6 Stats, M7 API           | T034–T052   | 80    |
| 4      | M8 WebUI                                 | T053–T064   | 64    |
| 5      | M9 Deferred TUI, M10 CLI, M11 Hardening & Release | T065–T078   | 60    |

Notes:

- The current implementation uses `internal/store` interfaces with JSON and SQLite backends.
  SQLite is available for new larger small-site deployments; JSON remains supported for
  simple and existing deployments.
- The TUI/Unix-socket control plane originally listed under M9 is deferred from the current
  v1 release scope. Supported management surfaces are WebUI and HTTP-backed CLI.
- All frontend tasks (M8) can begin once T046 (auth handlers) and T056 (API client) are stubbed; UI screens land independently after that.

---

## M1 — Foundation (8 tasks)

### T001 — Repo bootstrap

- **Scope:**
  - Create `cmd/sis/main.go` (empty `func main()`).
  - `go.mod` with module `github.com/ersinkoc/sis`, Go 1.24.
  - `LICENSE` (MIT).
  - `.gitignore`, `.editorconfig`.
  - `pkg/version` with `Version`, `Commit`, `Date` ldflag-injected vars.
  - `README.md` placeholder pointing to docs.
- **Deps:** —
- **Est:** 2h
- **Acceptance:** `go build ./...` succeeds; `sis version` prints injected build info.

### T002 — Build system

- **Scope:**
  - `Makefile`: `build`, `test`, `lint`, `fmt`, `webui`, `all`, `release`, `clean`.
  - `scripts/build.sh` for cross-compile (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64).
  - `-trimpath`, `CGO_ENABLED=0`, `-ldflags` for version vars.
  - SHA256 checksum file generation in `release` target.
- **Deps:** T001
- **Est:** 3h
- **Acceptance:** `make release` produces 4 binaries + `SHA256SUMS` in `dist/`.

### T003 — Config types + YAML loader

- **Scope:**
  - `internal/config/types.go` — all Config / Server / Upstream / Group / Schedule / etc. structs.
  - `internal/config/load.go` — read file, `yaml.Unmarshal`, env overrides (`SIS_*`).
  - Pin `gopkg.in/yaml.v3`.
  - Default values applied via `applyDefaults`.
- **Deps:** T001
- **Est:** 6h
- **Acceptance:** Loading `examples/sis.yaml` returns a fully-populated `Config`. Unit test for env override precedence.

### T004 — Config validator

- **Scope:**
  - `internal/config/validate.go` — multi-error validator (returns `errors.Join`).
  - Covers all rules in IMPL §3.3.
  - Rich error messages with field paths (`groups[1].schedules[0].from`).
- **Deps:** T003
- **Est:** 5h
- **Acceptance:** Unit tests for each documented misconfiguration produce the expected error path.

### T005 — Config holder + hot reload

- **Scope:**
  - `internal/config/holder.go` — atomic pointer holder.
  - `internal/config/reload.go` — `SIGHUP` handler stub (full reload integration in T077).
  - `OnReload(old, new)` callback registry.
- **Deps:** T003, T004
- **Est:** 4h
- **Acceptance:** `Holder.Replace` is safe under concurrent `Get`. Race detector clean.

### T006 — Query log writer + rotator

- **Scope:**
  - `internal/log/query.go` — `Query` writer with mutex, JSON encoder.
  - `internal/log/rotate.go` — size-based rotation, retention eviction, optional gzip.
  - In-memory fanout (channel) for SSE subscribers; bounded buffers; drop-oldest policy.
  - Privacy modes (`full | hashed | anonymous`).
- **Deps:** T003
- **Est:** 8h
- **Acceptance:** Writes 1M entries with rotation; gzip files appear; old files evicted on schedule. Subscriber receives entries within 10ms.

### T007 — Audit log writer

- **Scope:**
  - `internal/log/audit.go` — separate file, structured entries (actor, action, before/after).
  - Helper `Auditf(action, target, old, new)`.
- **Deps:** T006
- **Est:** 2h
- **Acceptance:** Audit entries land in `sis-audit.log`, never mixed with query log.

### T008 — Store interface + file backend

- **Scope:**
  - `internal/store/store.go` — interface (`Store`, `ClientStore`, `CustomListStore`, `SessionStore`, `StatsStore`, `ConfigHistoryStore`).
  - `internal/store/file.go` — implementation atop an atomic JSON file.
  - Migration registry (`0001_init`).
  - Crash-resilient temp-file, fsync, rename, and parent-directory fsync writes.
- **Deps:** T001
- **Est:** 8h
- **Acceptance:** Open/close cycle clean; basic CRUD on each store passes unit tests.

---

## M2 — DNS Pipeline Core (9 tasks)

### T009 — UDP DNS listener

- **Scope:**
  - `internal/dns/server.go` — UDP socket bind, read loop.
  - Worker pool dispatch.
  - `sync.Pool` for read/write buffers.
- **Deps:** T005
- **Est:** 5h
- **Acceptance:** Listener responds to a `dig` query (echo back NOERROR with empty answer for now).

### T010 — TCP DNS listener

- **Scope:**
  - TCP listener with per-connection goroutine.
  - 2-byte length-prefix framing per RFC 1035.
  - Idle timeout handling.
- **Deps:** T009
- **Est:** 4h
- **Acceptance:** `dig +tcp` against the listener works.

### T011 — Worker pool

- **Scope:**
  - `internal/dns/workers.go` — bounded pool with `Submit(f func())`.
  - Backpressure: drop on full (UDP) vs. block (TCP).
  - Worker count from config (default `NumCPU * 4`).
- **Deps:** T009
- **Est:** 3h
- **Acceptance:** Stress test: 50k QPS sustained, no goroutine leak.

### T012 — Pipeline scaffold

- **Scope:**
  - `internal/dns/pipeline.go` — `Request`, `Response`, `Pipeline` types.
  - `Handle(ctx, *Request) *Response` entry point.
  - Stage skeleton with no-op implementations (filled in later tasks).
- **Deps:** T009
- **Est:** 3h
- **Acceptance:** Pipeline returns SERVFAIL for any query (placeholder).

### T013 — Cache (LRU + TTL)

- **Scope:**
  - `internal/dns/cache.go` — intrusive doubly-linked LRU list, hash map index.
  - TTL clamping (min/max), negative caching window.
  - Stores wire-format response, rewrites ID and question on serve.
  - `Get`, `Put`, `Flush`, `Stats`.
- **Deps:** T012
- **Est:** 8h
- **Acceptance:** Race-free under 32 concurrent goroutines; benchmark ≥ 200k ops/sec on cache hit.

### T014 — Special name handling

- **Scope:**
  - `internal/dns/special.go` — localhost/loopback, `use-application-dns.net`, private PTR rejection.
  - Wired into pipeline before any other stage.
- **Deps:** T012
- **Est:** 3h
- **Acceptance:** Queries for these names never hit upstream; logged appropriately.

### T015 — ECS strip

- **Scope:**
  - `internal/dns/edns.go` — strip ECS option from outbound msg.
  - Configurable (`privacy.strip_ecs`).
  - Other EDNS options preserved.
- **Deps:** T012
- **Est:** 2h
- **Acceptance:** Outbound msg has no ECS option when flag is on; passes through when off.

### T016 — Synthetic responses

- **Scope:**
  - `internal/dns/synthetic.go` — block (`0.0.0.0`/`::`), NXDOMAIN, REFUSED, SERVFAIL, NODATA, loopback.
  - Configurable block response (`use_nxdomain`).
  - TTL from config.
- **Deps:** T012
- **Est:** 3h
- **Acceptance:** Each synthetic response parses cleanly via `dig` and matches expected rcode/answers.

### T017 — Pipeline integration

- **Scope:**
  - Wire stages: special → cache lookup → (policy stub) → (upstream stub) → cache store → log → respond.
  - Stub policy returns "no block"; stub upstream returns SERVFAIL.
  - Per-query latency measured and emitted.
- **Deps:** T013, T014, T015, T016
- **Est:** 4h
- **Acceptance:** End-to-end UDP/TCP query: pipeline routes correctly; cache hit on second query; query log entry appears.

---

## M3 — Policy Engine (10 tasks)

### T018 — Domain tree data structure

- **Scope:**
  - `internal/policy/domains.go` — `Domains`, `node`, `Add`, `Match`.
  - Suffix-tree on labels with wildcard support.
  - Memory-efficient: small leaf nodes.
- **Deps:** T001
- **Est:** 6h
- **Acceptance:** Match correctness fuzz target. Benchmark ≥ 1M ops/sec on 1M-entry list.

### T019 — Blocklist parser

- **Scope:**
  - `internal/policy/parser.go` — accept hosts and domain-only formats.
  - Skip comments, blanks, malformed lines.
  - Strip inline comments after `#`.
  - Reject obvious garbage (IP-only lines, empty domains).
- **Deps:** T018
- **Est:** 4h
- **Acceptance:** Parses StevenBlack hosts + OISD raw correctly; ≥ 99% of valid lines accepted in test fixtures.

### T020 — Blocklist fetcher

- **Scope:**
  - `internal/policy/fetcher.go` — HTTP GET with `If-Modified-Since`/`ETag`.
  - On 304: load compiled cache from disk.
  - On error: keep previous compiled set (never zero out).
  - Local `file://` URL support.
- **Deps:** T019
- **Est:** 5h
- **Acceptance:** First fetch downloads + parses; second fetch returns 304 and skips parse; offline mode loads from cache.

### T021 — Blocklist sync scheduler

- **Scope:**
  - `internal/policy/sync.go` — per-list ticker, jittered to avoid thundering herd.
  - Audit log entry per sync (success/failure).
  - Hot-swap into `Engine.lists[id]` under engine write lock.
- **Deps:** T020, T026
- **Est:** 4h
- **Acceptance:** Three lists with different intervals all sync independently; no in-flight queries dropped during swap.

### T022 — Custom block/allow lists

- **Scope:**
  - Persisted via `CustomListStore`.
  - First-class entries in policy engine (custom block list, custom allow list).
  - `Add`/`Remove`/`List` operations exposed for API consumption.
- **Deps:** T008, T018
- **Est:** 4h
- **Acceptance:** Adding a custom-block domain blocks subsequent queries within one tick.

### T023 — Allowlist (global + per-group)

- **Scope:**
  - Global allowlist as a `Domains` tree.
  - Per-group allowlist compiled once at config-load.
  - Allowlist always wins over block.
- **Deps:** T018
- **Est:** 3h
- **Acceptance:** A blocked domain becomes resolvable after allowlist add; restored block after remove.

### T024 — Group definition + resolver

- **Scope:**
  - `internal/policy/group.go` — `Group` struct, compile from config.
  - `ClientResolver` interface (returns group name for a client key).
  - Default group fallback.
- **Deps:** T003
- **Est:** 3h
- **Acceptance:** Unknown client keys resolve to `default`. Renaming a client's group reflected on next query.

### T025 — Schedule compiler + evaluator

- **Scope:**
  - Parse `days`, `from`, `to`, `block` into `CompiledSchedule`.
  - `ActiveAt(now, tz)` handles weekday filter, midnight cross, DST.
  - Convenience tokens: `all`, `weekday`, `weekend`.
- **Deps:** T024
- **Est:** 6h
- **Acceptance:** Test matrix: every weekday × hour-of-day × cross-midnight combination behaves correctly. DST transition test (Europe/Istanbul switch).

### T026 — Policy engine

- **Scope:**
  - `internal/policy/engine.go` — `Engine`, `For(identity)`, `Policy.Evaluate`.
  - Allowlist precedence, blocklist matching, schedule evaluation, custom lists.
  - `Decision` with reason and list ID for logging.
- **Deps:** T018, T023, T024, T025
- **Est:** 5h
- **Acceptance:** Acceptance scenarios 3, 4, 5 from SPEC §19 pass.

### T027 — Policy hot swap on config reload

- **Scope:**
  - `OnReload` callback rebuilds groups + recompiles allowlists.
  - Blocklist URL changes trigger re-fetch.
  - Existing custom entries preserved.
- **Deps:** T005, T026
- **Est:** 3h
- **Acceptance:** Editing groups via config reload doesn't drop in-flight queries; `audit.log` records the diff.

---

## M4 — Upstream (6 tasks)

### T028 — DoH client (single upstream)

- **Scope:**
  - `internal/upstream/doh.go` — POST `application/dns-message`.
  - Pack/unpack via `miekg/dns`.
  - Timeout per request.
- **Deps:** T012
- **Est:** 4h
- **Acceptance:** Round-trip query to Cloudflare DoH returns valid response.

### T029 — Bootstrap dialer

- **Scope:**
  - Custom `DialContext` that uses configured bootstrap IPs.
  - Per-host transport with correct SNI (`TLSClientConfig.ServerName`).
  - HTTP/2 enforced.
- **Deps:** T028
- **Est:** 5h
- **Acceptance:** With `/etc/resolv.conf` pointing at `127.0.0.1` (Sis itself), upstream DoH still resolves via bootstrap (no chicken-and-egg).

### T030 — Upstream pool with sequential failover

- **Scope:**
  - `internal/upstream/pool.go` — list, health gate, `Forward`.
  - First-healthy-wins selection.
  - On error: try next, mark unhealthy.
- **Deps:** T029
- **Est:** 4h
- **Acceptance:** Acceptance scenario 6 from SPEC §19 passes (failover to Quad9 when Cloudflare returns 5xx).

### T031 — Health prober

- **Scope:**
  - 60s ticker; probe unhealthy upstreams.
  - Recovery requires one successful probe after cooldown.
  - Passive failure tracking from `Forward`.
- **Deps:** T030
- **Est:** 3h
- **Acceptance:** Simulated downtime and recovery reflected in pool state within probe interval.

### T032 — Upstream metrics

- **Scope:**
  - Counters: requests, errors, latency histogram.
  - Per-upstream gauge: consecutive errors, healthy state.
- **Deps:** T030, T038
- **Est:** 2h
- **Acceptance:** Stats endpoint reports per-upstream metrics correctly.

### T033 — Upstream test command

- **Scope:**
  - `Pool.Test(id)` sends a probe query and returns timing + answer for diagnostics.
  - Used by `sis upstream test` and `POST /api/v1/upstreams/{id}/test`.
- **Deps:** T030
- **Est:** 2h
- **Acceptance:** `Test` returns latency + RCODE for `A example.com`.

---

## M5 — Client Identity (4 tasks)

### T034 — ARP parser (Linux)

- **Scope:**
  - `internal/dns/arp_linux.go` — parse `/proc/net/arp` format.
  - Handle hex flags column, normalize MAC.
  - No `os/exec` calls.
- **Deps:** T001
- **Est:** 3h
- **Acceptance:** Returns expected entries for fixture `/proc/net/arp` content.

### T035 — NDP parser (IPv6)

- **Scope:**
  - `/sys/class/net/*/neigh/*` reader where available.
  - Best-effort: returns empty map on unsupported kernel layouts.
  - Fallback path documented.
- **Deps:** T001
- **Est:** 4h
- **Acceptance:** On Linux 5.x, returns at least the entries `ip -6 neigh` shows. On macOS/BSD/Windows, returns empty map without error.

### T036 — ARP table refresh loop

- **Scope:**
  - 30s ticker; merges IPv4 + IPv6 results into one map.
  - Atomic pointer swap for lookup-side race-freedom.
  - Counter: ARP miss rate.
- **Deps:** T034, T035
- **Est:** 3h
- **Acceptance:** Concurrent `Lookup` is race-free under refresh.

### T037 — Client identity resolver + auto-registration

- **Scope:**
  - `internal/dns/client_id.go` — `Resolve(ip) Identity`.
  - Throttled `Touch(identity)` writes to `ClientStore` (debounce: 1 entry / minute / key).
  - Updates `last_seen`, `last_ip`.
- **Deps:** T036, T008
- **Est:** 4h
- **Acceptance:** New device queries; record appears in store with MAC key. Renaming via store reflects on next query.

---

## M6 — Stats (5 tasks)

### T038 — In-memory counters

- **Scope:**
  - `internal/stats/counters.go` — atomics for global counters.
  - Per-upstream sync.Map.
  - `Inc*` methods, `Snapshot()` for read.
- **Deps:** T001
- **Est:** 3h
- **Acceptance:** Race detector clean under 64 goroutines incrementing.

### T039 — Latency histogram

- **Scope:**
  - `internal/stats/histogram.go` — exponential buckets (1µs–10s).
  - p50/p95/p99 extraction.
- **Deps:** T038
- **Est:** 3h
- **Acceptance:** Quantile error within 5% of true value on synthetic data.

### T040 — Top-K (count-min + min-heap)

- **Scope:**
  - `internal/stats/topk.go` — count-min sketch sized for low memory; min-heap of size N.
  - `Add(key)`, `Top(n)`.
  - Promotion logic when sketch estimate exceeds heap floor.
- **Deps:** T038
- **Est:** 6h
- **Acceptance:** With 10M Zipf distributed inserts, heap matches true top-200 with ≥ 95% recall.

### T041 — Stats aggregator (1m flush)

- **Scope:**
  - `internal/stats/aggregator.go` — 1-minute ticker.
  - Snapshot in-memory counters → `StatsStore` rows.
  - Schema: `stats:1m:<unix-min>` global; per-client and per-domain sub-rows.
- **Deps:** T038, T040, T008
- **Est:** 5h
- **Acceptance:** After 5 minutes of activity, 5 minute rows present in store with consistent totals.

### T042 — Stats compaction

- **Scope:**
  - Hourly tick: fold previous hour's `1m` rows into one `1h` row.
  - Daily tick: fold `1h` → `1d`.
  - Retention enforcement (24h / 30d / 365d windows).
- **Deps:** T041
- **Est:** 4h
- **Acceptance:** After 25 hours, exactly 24×60 1m rows + 1 1h row. After 31 days, ≤ 30 1h rows.

---

## M7 — API (10 tasks)

### T043 — HTTP server scaffold + middleware chain

- **Scope:**
  - `internal/api/server.go` — `New(deps)`, `Handler()`.
  - Stdlib `http.ServeMux` with method+pattern routes.
  - Middleware: Recover, RequestID, AccessLog, SecurityHeaders.
- **Deps:** T005
- **Est:** 4h
- **Acceptance:** `GET /healthz` returns 200; panic in handler caught and logged.

### T044 — Auth service

- **Scope:**
  - `internal/api/auth.go` — PBKDF2-SHA256 password verification.
  - Session token generation, store-backed lookup, sliding expiration.
  - Cookie set/clear helpers.
- **Deps:** T008, T043
- **Est:** 5h
- **Acceptance:** Session round-trip via cookie. Wrong password rejected. Expired session cleaned.

### T045 — First-run wizard

- **Scope:**
  - `auth.first_run` flag in config.
  - `POST /api/v1/auth/setup` creates first admin, persists to config file, flips flag.
  - All other API routes return 412 until setup completes.
- **Deps:** T044
- **Est:** 4h
- **Acceptance:** Fresh install: setup endpoint reachable, others 412. After setup, normal operation.

### T046 — Login/logout/me handlers

- **Scope:**
  - `POST /api/v1/auth/login` — returns user info, sets cookie.
  - `POST /api/v1/auth/logout` — clears cookie, deletes session.
  - `GET /api/v1/auth/me` — current user.
  - Per-IP rate limit on `/auth/login`.
- **Deps:** T044, T076
- **Est:** 3h
- **Acceptance:** Cookie-based auth survives page reload. Logout invalidates session.

### T047 — Stats endpoints

- **Scope:**
  - `GET /api/v1/stats/summary?range=`
  - `GET /api/v1/stats/timeseries?metric=&range=&bucket=`
  - `GET /api/v1/stats/top-clients?range=&limit=`
  - `GET /api/v1/stats/top-domains?range=&limit=&blocked=`
  - `GET /api/v1/stats/upstreams`
- **Deps:** T038, T041, T042, T043
- **Est:** 6h
- **Acceptance:** Dashboard data renders correctly; ranges 1h/24h/7d each return consistent shapes.

### T048 — Query log endpoints

- **Scope:**
  - `GET /api/v1/logs/query` — paginated, filterable.
  - `GET /api/v1/logs/query/stream` — SSE live tail.
  - Filters: `client`, `qname`, `blocked`, `since`, `until`.
  - Backed by file scan with inverted index v2 (v1 reads tail of current log + recent rotations).
- **Deps:** T006, T043
- **Est:** 7h
- **Acceptance:** Filter combinations produce expected results. SSE delivers new entries within 100ms.

### T049 — Clients endpoints

- **Scope:**
  - `GET /api/v1/clients`
  - `GET /api/v1/clients/{key}`
  - `PATCH /api/v1/clients/{key}` — `{name?, group?, hidden?}`
  - `DELETE /api/v1/clients/{key}`
  - All mutations write audit log.
- **Deps:** T037, T043
- **Est:** 4h
- **Acceptance:** Acceptance scenario 2 from SPEC §19 passes.

### T050 — Groups endpoints

- **Scope:**
  - Full CRUD for groups including schedule editing.
  - Validation: cannot delete `default`; references to non-existent blocklists rejected.
  - Persisted via config (groups live in YAML, not store).
  - Hot-applies via `Holder.Replace`.
- **Deps:** T026, T027, T043
- **Est:** 6h
- **Acceptance:** Create group → assign client → see policy applied to subsequent queries.

### T051 — Blocklists & allowlist endpoints

- **Scope:**
  - Full CRUD for blocklists (URL-sourced + custom).
  - `POST /api/v1/blocklists/{id}/sync` — force refresh.
  - `GET /api/v1/blocklists/{id}/entries?q=` — search within compiled list.
  - Allowlist endpoints (global + custom-allow).
- **Deps:** T020, T021, T022, T023, T043
- **Est:** 6h
- **Acceptance:** Adding a list and force-syncing it returns updated counts; search returns matching domains.

### T052 — Upstreams + settings + system endpoints

- **Scope:**
  - Upstream CRUD + test (uses T033).
  - `GET/PATCH /api/v1/settings` — privacy, cache, logging, block-response.
  - `GET /api/v1/system/info`, `POST /api/v1/system/cache/flush`, `POST /api/v1/system/config/reload`.
- **Deps:** T030, T043
- **Est:** 5h
- **Acceptance:** Editing upstreams via API takes effect on next query; reload endpoint applies cleanly.

---

## M8 — WebUI (12 tasks)

### T053 — Vite + React 19 + Tailwind setup

- **Scope:**
  - `webui/` workspace.
  - Vite config, TypeScript strict, ESLint flat config.
  - Tailwind CSS 4 with the local Vite integration.
  - `dist/` output committed for embeds in release builds.
- **Deps:** T001
- **Est:** 4h
- **Acceptance:** `pnpm dev` runs. `pnpm build` produces `dist/`. Lighthouse perf ≥ 95 for empty shell.

### T054 — WebUI component foundation

- **Scope:**
  - Reusable form, table, panel, dialog, tab, select, switch, badge, tooltip, and scroll controls.
  - App shell or single-page operational panel layout, matching the current WebUI scope.
  - Tailored color tokens matching Sis branding.
- **Deps:** T053
- **Est:** 4h
- **Acceptance:** Shared controls render consistently in light and dark themes.

### T055 — Theme provider (dark/light/system)

- **Scope:**
  - Context + localStorage persistence.
  - Toggle in topbar.
  - Respects `prefers-color-scheme`.
  - All shared controls themed through the common stylesheet.
- **Deps:** T054
- **Est:** 3h
- **Acceptance:** Toggle persists across reloads; system mode follows OS in real time.

### T056 — API client + auth context

- **Scope:**
  - `webui/src/lib/api.ts` — typed wrapper around fetch.
  - 401 handling: redirect to `/login`.
  - 412 (first-run) handling: redirect to `/setup`.
  - Auth context provider with `me` query.
- **Deps:** T053
- **Est:** 4h
- **Acceptance:** Round-tripped against the backend (after T046). Type errors caught at build.

### T057 — Login + first-run setup screens

- **Scope:**
  - `/login` page with username/password form.
  - `/setup` page for first-run admin creation (with password requirements, confirmation field).
  - Both centered cards, branded.
- **Deps:** T055, T056
- **Est:** 4h
- **Acceptance:** Fresh install → setup → login → dashboard flow works end-to-end.

### T058 — App shell

- **Scope:**
  - Sidebar with nav items (Dashboard, Query Log, Clients, Groups, Blocklists, Allowlist, Upstreams, Settings).
  - Topbar with theme toggle, current user, logout.
  - Mobile: sidebar collapses into drawer.
  - Responsive breakpoints: 320px / 768px / 1024px / 1440px.
- **Deps:** T055, T057
- **Est:** 5h
- **Acceptance:** Layout works at all four breakpoints. Sidebar toggles on mobile.

### T059 — Dashboard page

- **Scope:**
  - Stats cards: total queries, blocked %, cache hit %, active clients (last hour).
  - Time-series chart (sparkline) for QPS over 24h.
  - Top 5 clients table.
  - Top 5 blocked domains table.
  - Auto-refresh every 30s.
- **Deps:** T056, T058, T047
- **Est:** 6h
- **Acceptance:** Acceptance scenario 1 from SPEC §19 visible on dashboard.

### T060 — Query log page

- **Scope:**
  - Live tail toggle (SSE) and paged history.
  - Filter bar: client, domain, blocked-only.
  - Table with virtualized rows (`@tanstack/react-virtual` or hand-rolled; v1 hand-rolled to keep deps lean).
  - Row click expands to show full entry JSON.
- **Deps:** T056, T058, T048
- **Est:** 7h
- **Acceptance:** Live mode shows entries within 1s of arrival. Filter combinations responsive.

### T061 — Clients page

- **Scope:**
  - Table: name, key, group, last_seen, last_ip, query count (24h), blocked count.
  - Inline rename (click name → input).
  - Group select dropdown.
  - Hide/forget actions.
  - Empty state for fresh install.
- **Deps:** T056, T058, T049
- **Est:** 5h
- **Acceptance:** Rename + move group reflected within 1s. Forget removes row.

### T062 — Groups page

- **Scope:**
  - Group list with member count.
  - Group detail panel: blocklists multiselect, allowlist text area, schedules editor.
  - Schedule editor: name, days (chips), from/to (time input), block lists multiselect.
  - Add/delete group.
- **Deps:** T056, T058, T050
- **Est:** 8h
- **Acceptance:** Adding bedtime schedule for `kids` group → query at 23:00 returns block (smoke test against backend).

### T063 — Blocklists / Allowlist page

- **Scope:**
  - Blocklists table with sync state, last sync, entry count, force-sync button.
  - Add blocklist dialog (URL or custom).
  - Search-within-list panel.
  - Global allowlist editor (chip list).
- **Deps:** T056, T058, T051
- **Est:** 6h
- **Acceptance:** Sync button updates count + timestamp. Search returns matches.

### T064 — Upstreams + Settings page

- **Scope:**
  - Upstreams table with health badges and latency p50/p95.
  - Add upstream dialog with bootstrap IPs.
  - Test button per upstream.
  - Settings tabs: Privacy, Cache, Logging, Block Response.
- **Deps:** T056, T058, T052
- **Est:** 6h
- **Acceptance:** Test button shows latency. Settings changes persist via API.

---

## M9 — Deferred TUI / Unix Socket (4 tasks)

These tasks are retained for historical planning only. They are not part of the current v1
release scope unless the product scope is explicitly reopened.

### T065 — Unix socket JSON-RPC

- **Scope:**
  - Deferred from v1.
  - `internal/api/sock.go` — listens on `<data_dir>/sis.sock` (mode 0660).
  - Method registry: `stats.summary`, `stats.timeseries`, `stats.topClients`, `stats.topDomains`, `log.subscribe` (streaming), `clients.list`, `clients.update`, `blocklists.list`, `blocklists.sync`, `upstreams.list`, `cache.flush`, `query.test`.
  - Newline-delimited JSON-RPC 2.0.
- **Deps:** T043
- **Est:** 6h
- **Acceptance:** `nc -U sis.sock` + manual JSON request returns expected response.

### T066 — Bubble tea app shell

- **Scope:**
  - Deferred from v1.
  - `internal/tui/app.go` — top-level `Model`, view router.
  - Hotkeys: `1-5` switch view, `q` quit, `?` help, `/` filter, `r` refresh.
  - Top bar with current view label and live timestamp.
  - Connects to Unix socket.
- **Deps:** T065
- **Est:** 5h
- **Acceptance:** `sis tui` opens, shows shell, hotkeys work, exits cleanly.

### T067 — Dashboard + live log views

- **Scope:**
  - Deferred from v1.
  - Dashboard: QPS, hit %, blocked % gauges; sparklines via `lipgloss` blocks.
  - Live log: streaming entries, `/text` filter, color-coded blocks.
- **Deps:** T066
- **Est:** 6h
- **Acceptance:** Both views update at 1Hz minimum without flicker.

### T068 — Clients + upstreams + blocklists views

- **Scope:**
  - Deferred from v1.
  - Clients: list with rename (`r`) and group-move (`g`) inline.
  - Upstreams: health table with latency.
  - Blocklists: list with sync (`s`) trigger.
- **Deps:** T066
- **Est:** 5h
- **Acceptance:** Rename a client via TUI; verify reflected in WebUI within 1s.

---

## M10 — CLI (3 tasks)

### T069 — CLI dispatch + offline commands

- **Scope:**
  - `internal/cli/root.go` — command tree.
  - `sis serve`, `sis version`, `sis config validate`, `sis config show`, `sis user add`, `sis user passwd`.
  - All implemented without server running.
- **Deps:** T003, T044
- **Est:** 5h
- **Acceptance:** `sis config validate examples/sis.yaml` prints OK or specific errors.

### T070 — CLI live commands

- **Scope:**
  - All commands that connect through the authenticated HTTP API: `client list/rename/move/forget`, `group list/add`, `blocklist sync/test`, `allowlist add/remove`, `cache flush/stats`, `query test`, `logs tail`, `stats`, `upstream test/health`, `system info/store-verify`.
  - Helpful error when server is not reachable or a session cookie is missing/expired.
- **Deps:** T046, T069
- **Est:** 6h
- **Acceptance:** Each command produces expected output against a live server.

### T071 — CLI output formats

- **Scope:**
  - Default human table (column-aligned, no extra deps).
  - `--json` for machine output.
  - `--no-color` flag and TTY detection.
- **Deps:** T070
- **Est:** 3h
- **Acceptance:** Pipe `sis client list --json` into `jq` works. Tables align.

---

## M11 — Hardening, Tests, Release (7 tasks)

### T072 — Unit test coverage

- **Scope:**
  - Cover `internal/dns`, `internal/policy`, `internal/upstream` to ≥ 70%.
  - Edge-case suites for cache, schedule, domain tree, ARP parser.
  - Race detector enabled in CI.
- **Deps:** T013, T026, T030, T034
- **Est:** 10h
- **Acceptance:** `make test` reports coverage ≥ 70% for the three packages.

### T073 — Integration tests

- **Scope:**
  - `tests/integration/` — fake DoH upstream, real DNS client.
  - End-to-end scenarios from SPEC §19 automated.
  - Store integration smoke (open, write, restart, read).
- **Deps:** all M2–M7
- **Est:** 8h
- **Acceptance:** All 10 acceptance scenarios pass via `make test-integration`.

### T074 — Conformance tests

- **Scope:**
  - DNS message validation: standard query types, truncation, opcodes, classes.
  - Privacy rules: ECS strip, private PTR, `use-application-dns.net`.
- **Deps:** T015, T014
- **Est:** 4h
- **Acceptance:** Conformance suite ≥ 95% pass.

### T075 — Benchmarks

- **Scope:**
  - `benchmarks/` package: pipeline cache hit, domain match, blocklist parse, DoH RTT (skipped without network).
  - Targets in SPEC §15 verified.
  - `make bench` target.
- **Deps:** T013, T018, T019
- **Est:** 4h
- **Acceptance:** All benchmarks run in CI; perf regression > 10% fails the build.

### T076 — Rate limiting

- **Scope:**
  - `internal/dns/ratelimit.go` — per-client-IP token bucket (DNS).
  - `internal/api/ratelimit.go` — per-IP buckets for `/auth/login` and other endpoints.
  - LRU eviction of inactive buckets.
- **Deps:** T012, T043
- **Est:** 4h
- **Acceptance:** Burst of 1000 queries from one client is throttled to configured QPS without dropping legitimate other clients.

### T077 — Documentation completion

- **Scope:**
  - `README.md` — install, quick start, screenshots.
  - `examples/sis.yaml` — fully commented reference config.
  - `examples/sis.service` — systemd unit.
  - Inline GoDoc on all exported symbols.
- **Deps:** all features complete
- **Est:** 6h
- **Acceptance:** A new user can install Sis from README in < 5 minutes.

### T078 — Release pipeline

- **Scope:**
  - GitHub Actions: build, test, release on tag.
  - Cross-compile + checksums.
  - Release notes template referencing closed PRs.
- **Deps:** T002, T072, T073
- **Est:** 4h
- **Acceptance:** Tagging `v1.0.0` produces a public release with binaries and checksums.

---

## Effort Summary

| Milestone           | Tasks | Est. Hours | Est. LOC (Go) | Est. LOC (TS/TSX) |
|---------------------|-------|------------|---------------|-------------------|
| M1 Foundation       | 8     | 38         | 900           | —                 |
| M2 DNS Core         | 9     | 35         | 1,000         | —                 |
| M3 Policy           | 10    | 43         | 1,200         | —                 |
| M4 Upstream         | 6     | 20         | 600           | —                 |
| M5 Client Identity  | 4     | 14         | 450           | —                 |
| M6 Stats            | 5     | 21         | 750           | —                 |
| M7 API              | 10    | 50         | 1,500         | —                 |
| M8 WebUI            | 12    | 62         | 200           | 3,800             |
| M9 Deferred TUI     | 4     | 22         | 800           | —                 |
| M10 CLI             | 3     | 14         | 500           | —                 |
| M11 Hardening       | 7     | 40         | 1,300         | —                 |
| **Total**           | **78**| **~360**   | **~9,200**    | **~3,800**        |

---

## Critical Path

```
T001 ─► T003 ─► T005 ─► T009 ─► T012 ─► T017
                                            │
                T018 ──► T026 ──────────────┤
                                            │
                T028 ──► T030 ──────────────┤
                                            │
                                            ▼
                                        T043 ──► T046 ──► T056 ──► T058 ──► T059
                                                                              │
                                                                              ▼
                                                                         T072 ─► T073 ─► T078
```

Backend can land independently of WebUI through T052. WebUI tasks T059–T064 can parallelize behind T056 + T058. The release gate is T072 → T073 → T078.

---

*End of TASKS.*
