# Project Analysis Report

> Auto-generated comprehensive analysis of Sis
> Generated: 2026-04-29
> Analyzer: Codex - Full Codebase Audit
> Post-audit implementation update: this branch now preserves and edits group schedules in
> the WebUI, makes `/readyz` dependency-aware, documents the PBKDF2-SHA256 password hashing
> contract, and adds Origin/Referer checks for unsafe cookie-authenticated API methods.
> Follow-up update: `/readyz` now also consumes DNS listener lifecycle state, Go 1.24.0
> `gofmt`/`test`/`vet` were run successfully with a temporary toolchain, and a mocked
> Playwright group schedule regression spec was added. Race testing is still blocked by
> missing `gcc`; browser execution is blocked by unsupported Playwright Chromium install
> on this host.
> Hardening update: API text errors are now returned as JSON envelopes with request IDs,
> access logs include request IDs, and HSTS is emitted when TLS is active or configured.
> Rate-limit update: authenticated API routes now have a configurable per-IP limiter via
> `server.http.rate_limit_per_minute` / `SIS_HTTP_RATE_LIMIT_PER_MINUTE`.
> Visibility update: DNS/API rate-limit rejections increment `rate_limited_total`, which is
> exposed in live stats, persisted rollups, and the WebUI summary.
> DNS error visibility update: malformed DNS packets increment `malformed_total`, which is
> exposed in live stats, persisted rollups, and the WebUI summary.

## 1. Executive Summary

Sis is a single-binary, privacy-first DNS gateway for home and small-office networks. It accepts classic DNS over UDP/TCP, applies per-client/group allow/block policy, forwards allowed queries to DNS-over-HTTPS upstreams, records query/audit/stat state, exposes a cookie-authenticated HTTP API, and embeds a Vite/React WebUI.

Key metrics from discovery:

| Metric | Value |
|---|---:|
| Raw files including `webui/node_modules`, `webui/dist`, `dist`, and `bin` | 5,641 |
| Source/ops files excluding generated/vendor artifacts | 180 |
| Go files excluding `webui/node_modules` | 103 |
| Go LOC excluding `webui/node_modules` and `vendor` | 17,440 |
| Go test files | 33 |
| Go package directories | 12 |
| Frontend source/test TS/TSX files | 8 |
| Frontend source/test LOC | 3,184 |
| Runtime Go deps in `go.mod` | 3 direct, 13 indirect |
| Frontend deps | 6 runtime, 9 dev, 172 lockfile packages |
| API routes registered | 47 |
| TODO/FIXME/HACK markers | 0 in source/docs excluding generated artifacts |

Overall health: **7/10 for small-site pre-v1 deployment, 5/10 against the full v1 specification**. The core DNS/API/storage architecture is coherent and surprisingly broad, with SQLite support, release scripts, CI, WebUI build/lint, and operational runbooks. The score is held down by missing original v1 surfaces (TUI and Unix-socket JSON-RPC), inability to verify Go build/tests in this environment, security/spec deviations around password hashing and CSRF, shallow readiness checks, incomplete WebUI schedule support, and lack of full integration/conformance/load testing evidence.

Top strengths:

- Solid modular-monolith architecture: `cmd/sis` composes small `internal/*` packages, and domain packages avoid web-framework sprawl.
- Operational posture is stronger than typical early projects: release scripts, systemd examples, backup/restore, SQLite migration/export, SBOM generation, artifact verification, and production runbooks exist.
- Good source-level defensive patterns: bounded HTTP body reads, structured middleware, request IDs, panic recovery, DNS rate limiting, atomic config holder, graceful shutdown, and no open TODO/FIXME debt.

Top concerns:

- The original TUI/Unix-socket management surface is completely absent (`internal/tui` and `internal/api/sock.go` do not exist).
- WebUI group saves discard schedules: `webui/src/lib/dashboard.ts:480-486` sends `schedules: []`, while `webui/src/App.tsx:947-1048` exposes only name/blocklists/allowlist editing.
- Security does not match the spec: passwords use custom PBKDF2-SHA256 (`internal/api/password.go:93-160`) while SPEC requires bcrypt; cookie-authenticated state-changing endpoints have no CSRF token/origin protection; only login is HTTP-rate-limited.

## 2. Architecture Analysis

### 2.1 High-Level Architecture

Sis is a **single-process modular monolith**. The executable in `cmd/sis/main.go` owns CLI dispatch and runtime composition. The runtime graph is:

```text
Config YAML + env
  -> config.Loader / config.Holder / config.Reloader
  -> store.OpenBackend(json|sqlite)
  -> log.Query + log.Audit
  -> policy.Engine + policy.Syncer
  -> upstream.Pool
  -> dns.Cache + dns.ClientID + dns.Pipeline + dns.Server
  -> stats.Counters + stats.Aggregator
  -> api.Server + embedded webui.Handler
```

DNS data flow:

```text
UDP/TCP listener
  -> miekg/dns unpack
  -> per-IP DNS token bucket
  -> client identity: ARP/NDP MAC, IP fallback
  -> opcode/class validation
  -> special names and private PTR handling
  -> policy evaluation: global/group/custom allow, blocklists, schedules
  -> cache lookup
  -> ECS stripping
  -> sequential DoH upstream pool
  -> cache store
  -> query log + stats
  -> DNS response
```

HTTP/API data flow:

```text
Browser/CLI/curl
  -> net/http ServeMux
  -> recover -> security headers -> request id -> access log -> auth middleware
  -> route handler
  -> config/store/policy/upstream/cache/stats/log subsystem
  -> JSON/SSE response
```

Concurrency model:

- `cmd/sis/main.go:1155-1163` starts background goroutines for SIGHUP, ARP refresh, blocklist sync, upstream health probing, stats aggregation, session cleanup, and operational signal handling.
- `internal/dns/server.go:34-88` starts UDP and TCP listeners for every configured DNS address.
- UDP packets use a bounded worker pool (`internal/dns/workers.go:292-360`); overflow drops work without response.
- TCP accepts one goroutine per connection but gates concurrency with `tcpSlots` (`internal/dns/server.go:203-277`).
- HTTP uses the standard `net/http` server with timeouts (`internal/api/server.go:175-183`).
- Shared runtime state uses mutexes, atomics, or atomic pointers: config holder, DNS cache, policy engine, SQLite/file stores, stats counters, log fanout.

### 2.2 Package Structure Assessment

Go packages and responsibilities:

| Package | Files | Responsibility | Cohesion |
|---|---:|---|---|
| `cmd/sis` | 4 | Main command dispatcher, offline/live CLI, backup/store ops, runtime composition | Broad but acceptable as composition root; `main.go` is very large at 1,322 LOC |
| `internal/api` | 24 | HTTP server, auth, middleware, route handlers, config mutation | Cohesive integration layer, but route surface is wide |
| `internal/config` | 11 | YAML types, load/save, defaults, env overrides, validation, reload, secret salt | Cohesive |
| `internal/dns` | 19 | DNS listeners, worker pool, cache, pipeline, identity, ECS, special/synthetic responses, rate limiting | Cohesive |
| `internal/log` | 6 | Query/audit logs, rotation, privacy, SSE fanout | Cohesive |
| `internal/policy` | 18 | Domain tree, parser/fetch/sync, groups/schedules, engine, store resolver | Cohesive |
| `internal/stats` | 5 | Atomic counters, histograms, persisted rollups | Cohesive but simplified vs spec Top-K |
| `internal/store` | 8 | Store interfaces, JSON backend, SQLite backend, migrations, transfer, verify | Cohesive but `sqlite.go` is large at 1,498 LOC |
| `internal/tools/sbom` | 2 | SPDX SBOM generator | Cohesive tool package |
| `internal/upstream` | 4 | DoH client and sequential health/failover pool | Cohesive |
| `internal/webui` | 1 | Embedded WebUI file server | Cohesive |
| `pkg/version` | 1 | Build-time version vars | Cohesive |

Internal vs public package split is appropriate. Only `pkg/version` is public, which matches the project’s single-binary intent. No Go import cycles can compile by definition; the intended directional flow is respected by file layout. The main risks are large integration files (`cmd/sis/main.go`, `internal/store/sqlite.go`, `internal/api/server_test.go`) and route-handler coupling to many runtime dependencies.

### 2.3 Dependency Analysis

Go dependencies from `go.mod`:

| Dependency | Version | Direct | Purpose | Stdlib replacement? | Notes |
|---|---:|---:|---|---|---|
| `gopkg.in/yaml.v3` | `v3.0.1` | Yes | YAML config parsing/writing | No practical stdlib YAML | Spec originally wanted no YAML dependency pending parser scope; implementation chose sane library path |
| `github.com/miekg/dns` | `v1.1.72` | Yes | DNS wire parsing/serialization | No | Appropriate de facto Go DNS library |
| `modernc.org/sqlite` | `v1.44.3` | Yes | CGO-free SQLite backend | No | Important because Makefile builds with `CGO_ENABLED=0` |
| `github.com/dustin/go-humanize` | `v1.0.1` | Indirect | SQLite transitive | N/A | Indirect |
| `github.com/google/uuid` | `v1.6.0` | Indirect | SQLite transitive | N/A | Indirect |
| `github.com/mattn/go-isatty` | `v0.0.20` | Indirect | Tooling/transitive | N/A | Indirect |
| `github.com/ncruces/go-strftime` | `v1.0.0` | Indirect | SQLite transitive | N/A | Indirect |
| `github.com/remyoudompheng/bigfft` | pseudo-version | Indirect | SQLite transitive | N/A | Indirect |
| `golang.org/x/exp` | pseudo-version | Indirect | Transitive | N/A | Version is future-dated relative to normal Go module history; verify provenance in CI |
| `golang.org/x/mod` | `v0.31.0` | Indirect | Tooling/transitive | N/A | Indirect |
| `golang.org/x/net` | `v0.48.0` | Indirect | HTTP/2 and transitive networking | N/A | Indirect |
| `golang.org/x/sync` | `v0.19.0` | Indirect | Transitive concurrency | N/A | Indirect |
| `golang.org/x/sys` | `v0.39.0` | Indirect | OS/syscalls | N/A | Indirect |
| `golang.org/x/tools` | `v0.40.0` | Indirect | Tooling/transitive | N/A | Indirect |
| `modernc.org/libc` | `v1.67.6` | Indirect | SQLite transitive | N/A | Indirect |
| `modernc.org/mathutil` | `v1.7.1` | Indirect | SQLite transitive | N/A | Indirect |
| `modernc.org/memory` | `v1.11.0` | Indirect | SQLite transitive | N/A | Indirect |

Dependency hygiene:

- `go` is unavailable in this environment, so `go mod tidy`, `go list -m`, `govulncheck`, `go test`, `go vet`, and `staticcheck` could not be run.
- `staticcheck` is unavailable.
- No evidence of unused Go deps could be confirmed without the Go toolchain.
- Spec says bcrypt via `golang.org/x/crypto/bcrypt`, but `x/crypto` is absent and the code implements PBKDF2 manually.

Frontend dependencies:

| Dependency | Version | Purpose | Notes |
|---|---:|---|---|
| `react` | `^19.2.5` | UI | Current stack target is React 19 |
| `react-dom` | `^19.2.5` | DOM renderer | Good |
| `vite` | `^8.0.10` | Build/dev server | Spec mentioned Vite 5; implementation has newer version |
| `tailwindcss` | `^4.2.4` | CSS utilities | Spec mentioned 4.1; implementation has newer version |
| `@tailwindcss/vite` | `^4.2.4` | Tailwind/Vite integration | Good |
| `@vitejs/plugin-react` | `^6.0.1` | React transform | Good |

Dev deps include TypeScript `^6.0.3`, ESLint `^10.2.1`, Playwright `^1.59.1`, React types, Node types, globals, and typescript-eslint.

Verification:

- `npm run build` passed. Bundle: JS `258.24 kB` raw / `70.73 kB` gzip; CSS `15.87 kB` raw / `3.96 kB` gzip.
- `npm run lint` passed.
- `npm audit --omit=dev` and `npm audit` both reported `found 0 vulnerabilities`.

### 2.4 API & Interface Design

Endpoint inventory from `internal/api/server.go:83-129`:

| Method | Path | Handler |
|---|---|---|
| GET | `/healthz` | `healthz` |
| GET | `/readyz` | `readyz` |
| POST | `/api/v1/auth/setup` | `setup` |
| POST | `/api/v1/auth/login` | `login` |
| POST | `/api/v1/auth/logout` | `logout` |
| GET | `/api/v1/auth/me` | `me` |
| GET | `/api/v1/stats/summary` | `statsSummary` |
| GET | `/api/v1/stats/timeseries` | `statsTimeseries` |
| GET | `/api/v1/stats/upstreams` | `statsUpstreams` |
| GET | `/api/v1/stats/top-domains` | `statsTopDomains` |
| GET | `/api/v1/stats/top-clients` | `statsTopClients` |
| GET | `/api/v1/logs/query` | `queryLogList` |
| GET | `/api/v1/logs/query/stream` | `queryLogStream` |
| GET | `/api/v1/clients` | `clientsList` |
| GET | `/api/v1/clients/{key}` | `clientGet` |
| PATCH | `/api/v1/clients/{key}` | `clientPatch` |
| DELETE | `/api/v1/clients/{key}` | `clientDelete` |
| GET | `/api/v1/allowlist` | `allowlistGet` |
| POST | `/api/v1/allowlist` | `allowlistAdd` |
| DELETE | `/api/v1/allowlist/{domain}` | `allowlistDelete` |
| GET | `/api/v1/custom-blocklist` | `customBlocklistGet` |
| POST | `/api/v1/custom-blocklist` | `customBlocklistAdd` |
| DELETE | `/api/v1/custom-blocklist/{domain}` | `customBlocklistDelete` |
| GET | `/api/v1/blocklists` | `blocklistsList` |
| POST | `/api/v1/blocklists` | `blocklistCreate` |
| PATCH | `/api/v1/blocklists/{id}` | `blocklistPatch` |
| DELETE | `/api/v1/blocklists/{id}` | `blocklistDelete` |
| POST | `/api/v1/blocklists/{id}/sync` | `blocklistSync` |
| GET | `/api/v1/blocklists/{id}/entries` | `blocklistEntries` |
| GET | `/api/v1/upstreams` | `upstreamsList` |
| POST | `/api/v1/upstreams` | `upstreamCreate` |
| PATCH | `/api/v1/upstreams/{id}` | `upstreamPatch` |
| DELETE | `/api/v1/upstreams/{id}` | `upstreamDelete` |
| POST | `/api/v1/upstreams/{id}/test` | `upstreamTest` |
| GET | `/api/v1/groups` | `groupsList` |
| POST | `/api/v1/groups` | `groupCreate` |
| GET | `/api/v1/groups/{name}` | `groupGet` |
| PATCH | `/api/v1/groups/{name}` | `groupPatch` |
| DELETE | `/api/v1/groups/{name}` | `groupDelete` |
| GET | `/api/v1/settings` | `settingsGet` |
| PATCH | `/api/v1/settings` | `settingsPatch` |
| POST | `/api/v1/query/test` | `queryTest` |
| GET | `/api/v1/system/info` | `systemInfo` |
| GET | `/api/v1/system/store/verify` | `storeVerify` |
| POST | `/api/v1/system/cache/flush` | `cacheFlush` |
| GET | `/api/v1/system/config/history` | `configHistory` |
| POST | `/api/v1/system/config/reload` | `configReload` |

API consistency:

- Route organization is consistent around `/api/v1`.
- Responses are mostly JSON on success and `http.Error` plaintext on failure. There is no uniform JSON error envelope.
- `decodeJSON` rejects oversized bodies, unknown fields, and trailing data (`internal/api/server.go:339+`, verified by tests in `internal/api/server_test.go:654-678`).
- Auth model is local user/password with server-side sessions and sliding cookie renewal (`internal/api/auth.go:117-170`).
- `authRequired` gates API routes except setup/login (`internal/api/server.go:254-286`).
- No CORS support is present, which is acceptable if same-origin WebUI is the intended access model.
- No CSRF defense is present for cookie-authenticated mutation routes.
- Only login has HTTP rate limiting (`internal/api/server.go:80`, `internal/api/auth.go:71-75`); SPEC calls for wider HTTP rate limiting.

## 3. Code Quality Assessment

### 3.1 Go Code Quality

Style and organization:

- Code appears gofmt-formatted by inspection; actual `gofmt -l` could not run because `go` is missing.
- Naming is idiomatic and small packages have clear boundaries.
- Exported symbols generally have comments in public-ish packages, supporting the `godoc` script.

Error handling:

- Config validation accumulates multi-errors with field paths (`internal/config/validate.go:17-265`).
- API distinguishes internal/gateway errors in helper methods (`internal/api/server.go:325-337`) but returns plaintext errors.
- DNS pipeline drops malformed UDP/TCP messages silently (`internal/dns/server.go:162-166`, `233-237`), with no explicit malformed counter despite spec mention.
- CLI HTTP client caps response size and streams SSE safely (`cmd/sis/httpcli.go:46-105`).
- Config save now follows the store durability pattern by fsyncing the temporary YAML file before rename and fsyncing the parent directory after rename (`internal/config/load.go`).

Context usage:

- Main runtime uses `signal.NotifyContext` for SIGINT/SIGTERM (`cmd/sis/main.go:1155`).
- API and DNS shutdown use 10-second contexts (`cmd/sis/main.go:1179-1184`, `internal/api/server.go:158-163`).
- DoH requests use per-request timeouts (`internal/upstream/doh.go:110-112`).
- Background loops generally respect context.

Logging:

- Runtime uses `log/slog` for server events and structured query/audit log writers.
- Query log privacy modes are centralized in `internal/log/query.go`.
- Access logs include method, path, status, duration (`internal/api/middleware.go:26-32`) but not request ID in the log line.

Configuration:

- Loader applies YAML, defaults, env overrides, then validation (`internal/config/load.go:18-33`).
- Save uses a temporary file, restricted permissions, temp-file fsync, atomic rename, and parent-directory fsync (`internal/config/load.go`).
- Config mutation PATCH handlers preserve omitted fields for settings, groups, upstreams, and blocklists; explicit empty arrays and false booleans remain valid updates.
- Validation creates data/log directories as a side effect (`internal/config/validate.go:251-259`), which is operationally convenient but surprising for a validator.

Magic numbers and hardcoded values:

- HTTP timeouts: 5s/15s/30s/120s in `internal/api/server.go:175-183`.
- Session TTL default: 24h in `internal/api/auth.go:156-160` and `internal/config/load.go:121-123`.
- Password PBKDF2 iterations: 210,000 in `internal/api/password.go:93`.
- DNS default UDP size: 1232 in `internal/dns/server.go:145-148` and config defaults.
- DNS default rate limit: 200/400 in `internal/config/load.go:73-78`.
- DoH timeout default: 3s in `internal/upstream/doh.go:32-35`.

TODO/FIXME/HACK:

- `rg` found no `TODO`, `FIXME`, `HACK`, or `XXX` markers outside generated/vendor artifacts.

### 3.2 Frontend Code Quality

Frontend implementation:

- Vite/React 19/TypeScript strict is configured (`webui/tsconfig.json`).
- `npm run build` and `npm run lint` pass.
- Main UI is a single large `App.tsx` file with 2,378 LOC. It is functional but not modular enough for long-term maintainability.
- API access is typed manually in `webui/src/lib/dashboard.ts`; no generated OpenAPI client exists.
- State management uses React hooks and `Promise.all` loaders; no query cache or stale-while-revalidate mechanism.
- CSS uses Tailwind utility classes directly.

Accessibility:

- Forms generally use labels and native controls.
- There is no obvious route-level navigation, sidebar, drawer, tabs, or keyboard shortcut model despite the original WebUI specification.
- No automated accessibility tests are present.
- E2E smoke uses role/label selectors (`webui/e2e/smoke.spec.ts`), which is a positive sign.

Critical frontend correctness issue:

- `webui/src/lib/dashboard.ts:480-486` always sends `schedules: []` on group update.
- `webui/src/App.tsx:947-1048` lets users edit group name, blocklists, and allowlist only.
- Result: saving any group through the WebUI can erase all schedules for that group. This directly violates SPEC schedule goals and is production-impacting for parental/work-hour policies.

Bundle:

- Production build succeeded with JS `258.24 kB` raw / `70.73 kB` gzip and CSS `15.87 kB` raw / `3.96 kB` gzip.

### 3.3 Concurrency & Safety

Strengths:

- DNS cache uses a mutex and stores packed wire responses; cache hit rewrites ID/question and remaining TTL (`internal/dns/cache.go:63-99`).
- Policy engine snapshots list map references under RLock (`internal/policy/engine.go:73-95`).
- File store serializes writes by prefix and whole-store lock (`internal/store/file.go:191-227`).
- SQLite store uses a store-wide mutex around schema/backing writes.
- Stats counters use atomics and maps protected by mutexes.

Risks:

- `internal/dns/server.go:122-134` launches a goroutine on every `Shutdown` call. Tests cover idempotence, but concurrent repeated shutdowns could race on fields without a top-level mutex.
- UDP serve loop allocates a new buffer and copies packet per read (`internal/dns/server.go:149-158`); SPEC wanted `sync.Pool`.
- Policy `For` copies the list map per query (`internal/policy/engine.go:87-90`), which is simple but adds hot-path allocation under load.
- `cmd/sis/main.go:1198-1199` assumes query/audit log pointers are non-nil; current composition creates them, but the function signature accepts nil and would panic if reused incorrectly.

Resource management:

- DoH response bodies are closed and capped at 65,535 bytes (`internal/upstream/doh.go:118-135`).
- CLI HTTP responses are capped at 8 MiB (`cmd/sis/httpcli.go:15`, `84-90`).
- File store and config save both fsync data before rename and fsync the parent directory after rename.

### 3.4 Security Assessment

Positive:

- Server-side random 32-byte session tokens (`internal/api/auth.go:199-205`).
- HttpOnly, SameSite=Lax cookies; Secure set when request TLS, TLS config, or `auth.secure_cookie` is active (`internal/api/auth.go`).
- Bounded JSON bodies and unknown-field rejection.
- Security headers include `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and CSP (`internal/api/server.go:302-310`).
- No hardcoded secrets found by inspection.
- `npm audit` reports zero frontend vulnerabilities.

Concerns:

- Password hashing deviates from SPEC: custom PBKDF2-SHA256 instead of bcrypt (`internal/api/password.go:93-160`; SPEC §13.1 and §16.1 require bcrypt). PBKDF2 can be acceptable when tuned, but this is a spec and interoperability deviation.
- No CSRF protection for cookie-authenticated `POST/PATCH/DELETE` endpoints.
- No HSTS header when TLS is enabled.
- No HTTP request rate limit outside login, despite SPEC calling for `/auth/*` and other endpoint limits.
- No authorization roles; any authenticated user is full admin.
- `/readyz` always returns ready (`internal/api/server.go:199-202`) and does not verify DNS listener, blocklists, or upstream health as specified.
- No explicit session token redaction in access logs is needed because cookies are not logged, but query/audit data remains sensitive and must be protected operationally.

## 4. Testing Assessment

### 4.1 Test Coverage

Test files:

- 33 Go `_test.go` files across CLI, API, config, DNS, log, policy, stats, store, SBOM, upstream.
- Frontend has one Playwright smoke spec: `webui/e2e/smoke.spec.ts`.

Could not run:

- `go test ./... -count=1`: failed with `/bin/bash: go: command not found`.
- `go vet ./...`: failed with `/bin/bash: go: command not found`.
- `staticcheck ./...`: failed with `/bin/bash: staticcheck: command not found`.
- `go test -race`: impossible without Go.

Ran successfully:

- `npm run build`.
- `npm run lint`.
- `npm audit` and `npm audit --omit=dev`.

Coverage reality:

- Unit tests exist for most core packages.
- No `tests/integration/` directory exists.
- SPEC §19-style DNS acceptance coverage now exists in `internal/dns/acceptance_test.go`, using fake DoH upstreams and real UDP/TCP DNS clients for default forwarding/blocking, allowlist override, active/inactive schedules, upstream failover, cache-hit logging, hashed privacy logging, per-client rename/group move, hot reload, and restart persistence. API integration coverage includes file-backed blocklist sync, setup/session restart persistence, and group schedule mutation through query/test HTTP endpoints. `.project/ACCEPTANCE_MATRIX.md` maps all SPEC §19 scenarios to evidence and remaining gaps.
- No benchmark harness package exists; only package-level benchmarks for DNS cache and domain matching.
- Seeded Go fuzz targets exist for blocklist parsing, policy domain matching, API domain normalization, and DNS message edge cases.
- No frontend unit/component tests found.

Packages with weak/no direct test coverage:

- `internal/webui`: no direct Go tests.
- `internal/log/audit.go` and `rotate.go`: partial coverage through query tests, not comprehensive.
- `internal/store/sqlite.go`: covered through `internal/store/file_test.go` migration/CRUD tests but not isolated as a separate `sqlite_test.go`.
- `cmd/sis` has tests, but command surface is broad and not every live command path appears exhaustively covered.

### 4.2 Test Infrastructure

CI:

- `.github/workflows/ci.yml` sets up Go from `go.mod`, Node 24, runs `WEBUI_PM=npm WEBUI_INSTALL=ci ./scripts/check.sh`, installs Playwright Chromium, runs WebUI smoke, runs benchmarks, and publishes tag releases.

Scripts:

- `scripts/check.sh` gates gofmt, clean git diff, godoc, release/production validation smoke scripts, WebUI install/build/lint, embed sync, coverage, `go vet`, build, and smoke.
- `scripts/coverage.sh` exists but could not be executed without Go.

Frontend:

- Playwright smoke validates first-run setup, store verification, and blocked query test.
- No Storybook/shadcn setup despite TASKS T054.

## 5. Specification vs Implementation Gap Analysis

### 5.1 Feature Completion Matrix

| Planned Feature | Spec Section | Implementation Status | Files/Packages | Notes |
|---|---|---|---|---|
| Single Go binary | SPEC §1.3, §17 | Complete | `Makefile`, `cmd/sis`, `internal/webui` | Embedded WebUI via `//go:embed`; release scripts build 4 targets |
| UDP/TCP DNS ingress | SPEC §3, IMPL §5 | Complete | `internal/dns/server.go` | Binds both protocols; TCP connection cap |
| Worker pool | IMPL §2.2, T011 | Complete | `internal/dns/workers.go` | Bounded pool; UDP drops on full |
| DNS pipeline | SPEC §2.1 | Mostly complete | `internal/dns/pipeline.go` | Covers rate limit, identity, special, policy, cache, upstream, logs, stats |
| ECS stripping | SPEC §3.3 | Complete | `internal/dns/edns.go`, `pipeline.go:138-140` | Configurable |
| Special names/private PTR | SPEC §3.2 | Complete/partial | `internal/dns/special.go` | Local zones not implemented; spec mentions `policy.HasLocalZoneFor` concept |
| Cache LRU/TTL | SPEC §3.4 | Complete | `internal/dns/cache.go` | Mutex/list/map; no sharding |
| Per-client identity MAC/IP | SPEC §4 | Partial | `internal/dns/arp.go`, `client_id.go` | Linux ARP/NDP implemented; macOS/BSD/Windows convenience paths absent |
| Auto-registration | SPEC §4.2 | Complete | `internal/dns/client_id.go` | Store-backed Touch with debounce |
| Groups/policy/schedules | SPEC §5 | Complete backend | `internal/policy/*` | Backend supports schedules; WebUI can erase them |
| Custom block/allow | SPEC §6.5 | Complete | `internal/api/allowlist.go`, `custom_blocklist.go`, `store` | Runtime + persisted |
| Blocklist parser/fetch/sync | SPEC §6 | Complete | `internal/policy/parser.go`, `fetcher.go`, `sync.go` | Syncer exists |
| DoH upstream forwarding | SPEC §7 | Complete | `internal/upstream/doh.go`, `pool.go` | Bootstrap dialer and sequential failover present |
| Upstream cooldown | SPEC §7.3 | Partial | `internal/upstream/pool.go` | Three failures mark unhealthy; no explicit cooldown timestamp |
| Query/audit logging | SPEC §8 | Complete | `internal/log/*` | Privacy modes present |
| Stats aggregation | SPEC §10.2 | Partial | `internal/stats/*`, `internal/store` | Counters/rollups exist; Top-K is simple sorted map, not count-min/min-heap |
| CLI | SPEC §11.1 | Mostly complete | `cmd/sis/main.go`, `httpcli.go` | HTTP client based, not Unix socket; broad command set |
| TUI | SPEC §11.2 | Missing | No `internal/tui` | Entire milestone absent |
| React WebUI | SPEC §11.3 | Partial | `webui/src/*` | Broad dashboard/forms exist; not shadcn/lucide, no real routing/sidebar, schedules missing |
| REST API | SPEC §12 | Mostly complete | `internal/api/*` | Extra endpoints for setup/store/history; errors not uniform JSON |
| bcrypt auth | SPEC §13.1 | Deviates | `internal/api/password.go` | PBKDF2-SHA256 instead |
| HTTP rate limiting | SPEC §13.3 | Partial | `internal/api/ratelimit.go`, `auth.go` | Login only |
| Readiness | SPEC §14.1 | Incomplete | `internal/api/server.go:199-202` | Always true |
| Prometheus exporter | SPEC §14.2 | Reserved v2 | N/A | Correctly absent |
| Bench harness | SPEC §15 | Partial | `*_bench_test.go` | No `sis bench`; no perf target gate except CI benchmarks |
| SQLite backend | Docs/CHANGELOG post-spec | Complete addition | `internal/store/sqlite.go`, `transfer.go` | Scope creep but valuable |
| Release automation | T078 | Complete | `.github/workflows/ci.yml`, scripts | Strong |

### 5.2 Architectural Deviations

- **SQLite implemented though original IMPLEMENTATION said current backend was JSON and future SQLite.** This is a valuable improvement, backed by migration/export/verify scripts.
- **TUI/Unix socket skipped.** This is a regression against SPEC/TASKS, not merely a simplification, because SPEC lists three management surfaces.
- **CLI uses HTTP API rather than Unix socket JSON-RPC.** This simplifies implementation and aligns CLI/WebUI, but deviates from M9/M10 task plan.
- **Password hashing changed from bcrypt to custom PBKDF2.** This is a security/spec deviation that should be deliberate and documented or corrected.
- **No shadcn/ui or lucide-react.** WebUI uses native controls and Tailwind classes. This reduces dependency load but deviates from specified visual/component system.
- **No custom YAML subset parser.** Implementation uses `gopkg.in/yaml.v3`; this is a pragmatic improvement over a hand-rolled parser.
- **Readiness check simplified to always-ready.** This is a production-readiness regression.

### 5.3 Task Completion Assessment

Estimated task completion:

- Complete or substantially complete: T001-T039, T041-T052, T055-T064 partial/complete, T069-T071 mostly via HTTP, T076-T078 mostly complete.
- Partial: T040, T053, T054, T060-T064, T072-T075.
- Missing: T065-T068 TUI/Unix socket milestone.

Approximate completion:

- **Backend core/API/DNS/storage:** 85-90%.
- **Original full v1 including TUI and hardening:** 70-75%.
- **Task count rough completion:** about **60 of 78** fully/substantially complete, **10 partial**, **8 missing**, or roughly **77% task completion**.

Blocked/abandoned tasks:

- T065 Unix socket JSON-RPC: missing.
- T066-T068 TUI: missing.
- T054 shadcn/lucide integration: missing.
- T073 integration tests: missing as a dedicated suite.
- T074 conformance tests: partial only.
- T075 benchmark harness/perf gate: partial.

### 5.4 Scope Creep Detection

Valuable additions not in original v1 spec:

- SQLite backend with normalized tables and portable JSON export/import.
- Store verification API/CLI/WebUI.
- Backup/restore and Linux service operational scripts.
- Production validation report tooling and release-candidate evidence gate.
- SPDX SBOM generation and optional release signing.
- Dependabot and issue templates.

These additions are valuable for production operations, not gratuitous complexity. They do, however, pull attention away from TUI/schedule WebUI completion and need matching tests/docs discipline.

### 5.5 Missing Critical Components

Priority missing items:

1. **WebUI schedule editor and preservation.** Current WebUI can erase schedules, undermining one of v1’s headline features.
2. **Real readiness check.** `/readyz` must reflect DNS bind, blocklist state, and upstream health.
3. **Go test/build verification in the audited environment.** CI likely covers this, but local audit could not.
4. **Integration acceptance suite for SPEC §19.**
5. **CSRF protection or equivalent origin enforcement for cookie-authenticated mutations.**
6. **TUI/Unix socket if still considered v1 scope.**

## 6. Performance & Scalability

### 6.1 Performance Patterns

Hot paths:

- DNS UDP read/dispatch (`internal/dns/server.go:143-180`).
- Policy evaluation (`internal/policy/engine.go:182-208`).
- Cache get/put (`internal/dns/cache.go:63-133`).
- DoH forwarding (`internal/upstream/doh.go:90-144`).
- Query log write/fanout.

Potential bottlenecks:

- Policy map copy per query in `Engine.For` (`internal/policy/engine.go:87-90`).
- Single mutex DNS cache; no sharding.
- UDP allocates/copies per packet instead of sync.Pool (`internal/dns/server.go:149-156`).
- JSON store writes the whole database for each mutation (`internal/store/file.go:213-227`), acceptable for small sites but poor at high write churn.
- Stats top-domain/client implementation uses exact maps and sorting, not the specified count-min sketch/min-heap.

Good patterns:

- DoH body capped at DNS max size.
- Static assets get immutable cache headers (`internal/webui/webui.go:54-60`).
- API server has read/write/header/idle timeouts.

### 6.2 Scalability Assessment

Horizontal scaling:

- The service is stateful and single-node by design. It cannot horizontally scale without shared store, shared cache/policy state, and leader/failover decisions.
- DNS cache is process-local.
- Sessions are store-backed, so a shared DB could allow API session sharing, but no clustering is implemented.

Back-pressure:

- UDP worker queue drops on saturation.
- TCP has a slot semaphore.
- DoH forwarding is sequential failover, not load balancing.
- No global API rate limit for non-auth endpoints.

## 7. Developer Experience

### 7.1 Onboarding Assessment

README is extensive and practical. A developer can understand `make check`, release scripts, local start, service install, backup/restore, and API examples.

Local environment issue from this audit:

- `go` is not installed/on PATH in the current workspace, so Go onboarding is blocked here.
- Node/npm is available and WebUI build/lint works.

### 7.2 Documentation Quality

Strong:

- `README.md`, `ARCHITECTURE.md`, `docs/PRODUCTION.md`, `docs/PRODUCTION_VALIDATION.md`, `.github/RELEASE.md`, and `SECURITY.md` are useful and operator-focused.
- Architecture diagrams document current runtime composition.

Weak/drift:

- `.project/SPECIFICATION.md` still promises TUI, bcrypt, shadcn/lucide, and specific dependency choices that implementation does not match.
- `.project/IMPLEMENTATION.md` still describes JSON as current backend in early sections, while README/CHANGELOG document SQLite as implemented.
- No OpenAPI/API reference document exists beyond README examples and route code.

### 7.3 Build & Deploy

Build:

- `Makefile` and scripts support build, test, coverage, bench, godoc, WebUI embed, release, release smoke.
- `scripts/build.sh` cross-compiles Linux/macOS amd64/arm64 with `CGO_ENABLED=0`.

Deploy:

- systemd example and install/upgrade/verify scripts exist.
- No Dockerfile or docker-compose file found.
- No `.goreleaser.yml` found; custom scripts and GitHub Actions handle releases.

CI/CD:

- CI is mature for a small project: check gate, Playwright smoke, benchmarks, release job, SBOM, optional signing, GitHub Release publication.

## 8. Technical Debt Inventory

### Critical (blocks production readiness)

| Location | Debt | Suggested fix | Effort |
|---|---|---:|---:|
| `webui/src/lib/dashboard.ts:480-486`, `webui/src/App.tsx:947-1048` | Group edits drop schedules by sending `schedules: []`; no schedule editor exists | Preserve existing schedules at minimum; then implement schedule CRUD UI | 8-16h |
| `internal/api/server.go:199-202` | `/readyz` always returns ready | Check DNS server state, store readability, blocklist load state, and at least one healthy upstream | 4-8h |
| Environment | Go toolchain unavailable, so build/test/vet/race could not be verified locally | Install Go matching `go.mod`; run full gate | 1-2h |
| `internal/api/password.go:93-160` vs SPEC | Password hashing deviates from bcrypt requirement | Switch to `x/crypto/bcrypt` or update spec/security docs and add migration support | 4-8h |
| API auth boundary | No CSRF protection for cookie-auth mutations | Add CSRF token or Origin/Referer enforcement for unsafe methods | 6-12h |

### Important (should fix before v1.0)

| Location | Debt | Suggested fix | Effort |
|---|---|---:|---:|
| No `internal/tui`, no socket API | TUI/Unix socket tasks missing | Decide if removed from v1; update spec/tasks or implement | 24-40h |
| `internal/api` errors | Plaintext error bodies inconsistent with JSON API | Standardize JSON error envelope | 4-8h |
| `internal/dns/server.go:149-156` | UDP hot path allocates/copies per packet | Introduce buffer pool or benchmark current cost and document | 4-8h |
| `internal/policy/engine.go:87-90` | Per-query map copy | Replace with immutable snapshot pointer or benchmark and document | 4-8h |
| Tests | Browser/race/fuzz-campaign/performance evidence remains partial | Run browser e2e where Playwright Chromium is supported; add race, long-running fuzz, and perf gates | 8-16h |
| Observability | No Prometheus/pprof endpoint, request ID not in access logs | Add request ID to logs and optional metrics/profiling endpoints | 8-16h |
| API rate limits | Only login is limited | Add configurable limiter for other API endpoints | 4-8h |

### Minor (nice to fix, not urgent)

| Location | Debt | Suggested fix | Effort |
|---|---|---:|---:|
| `cmd/sis/main.go` | 1,322 LOC composition/CLI file | Split backup/store/serve/live command files | 8-16h |
| `internal/store/sqlite.go` | 1,498 LOC monolith | Split schema/migrations/stores by concern | 8-16h |
| `webui/src/App.tsx` | 2,378 LOC single component file | Split panels/forms/components | 8-16h |
| Docs | SPEC/IMPLEMENTATION drift from current code | Update source-of-truth docs | 4-8h |
| Frontend UX | No sidebar/routes despite spec | Add navigation shell or update spec | 8-16h |

## 9. Metrics Summary Table

| Metric | Value |
|---|---:|
| Total Go Files | 103 |
| Total Go LOC | 17,440 |
| Total Frontend Files | 8 |
| Total Frontend LOC | 3,184 |
| Test Files | 34 Go + 1 Playwright |
| Test Coverage | Not measured in this audit |
| External Go Dependencies | 3 direct, 13 indirect |
| External Frontend Dependencies | 6 runtime, 9 dev, 172 lockfile packages |
| Open TODOs/FIXMEs | 0 |
| API Endpoints | 47 |
| Spec Feature Completion | ~75% against full original v1; ~85% against current README scope |
| Task Completion | ~77% fully/substantially complete |
| Overall Health Score | 7/10 small-site; 5/10 full v1 spec |
