# Production Readiness Assessment

> Comprehensive evaluation of whether Sis is ready for production deployment.
> Assessment Date: 2026-04-29
> Verdict: Yellow CONDITIONALLY READY

## Overall Verdict & Score

**Production Readiness Score: 78/100**

| Category | Score | Weight | Weighted Score |
|---|---:|---:|---:|
| Core Functionality | 7/10 | 20% | 14.0 |
| Reliability & Error Handling | 8/10 | 15% | 12.0 |
| Security | 8/10 | 20% | 16.0 |
| Performance | 6/10 | 10% | 6.0 |
| Testing | 5/10 | 15% | 7.5 |
| Observability | 6/10 | 10% | 6.0 |
| Documentation | 8/10 | 5% | 4.0 |
| Deployment Readiness | 9/10 | 5% | 4.5 |
| **TOTAL** | | **100%** | **78/100** |

## 1. Core Functionality Assessment

### 1.1 Feature Completeness

Estimated specified feature completion:

- Current README/release scope: **~86% implemented**.
- Current `.project/SPECIFICATION.md` v1 scope: **~86% implemented**.

Core feature status:

- Complete: DNS UDP/TCP listener, pipeline, DoH forwarding, cache, policy engine, schedules backend, block/allow lists, custom lists, logging, stats counters/rollups, HTTP API, cookie sessions, config reload, JSON/SQLite stores, embedded WebUI, backup/restore, release scripts.
- Partial: acceptance testing, sustained live production load targets, conformance tests, frontend accessibility, upstream cooldown semantics.
- Missing or deferred: OpenAPI docs and full live SPEC §19 production validation. TUI/Unix-socket JSON-RPC is explicitly deferred from current v1 scope.
- Recently fixed: WebUI group saves now preserve schedules and expose schedule editing.

### 1.2 Critical Path Analysis

A user can likely complete the primary happy path: install, start service, run setup/login, query DNS, block a configured domain, view stats/logs, manage clients, manage allow/block lists, and test upstreams. This is backed by source inspection and a Playwright smoke spec.

Critical broken/unfinished flows:

- Group schedule management has WebUI create/edit/delete support and browser-regression coverage in CI.
- `/readyz` now checks config, store readability, upstream health, DNS pipeline wiring, and DNS listener lifecycle state.
- First-run/auth works; PBKDF2-SHA256 is documented as the current pre-v1 compatibility contract.
- TUI workflow is absent by current scope decision and tracked as v2/deferred work.

### 1.3 Data Integrity

- JSON store writes through in-memory map and atomic persistence; SQLite backend adds normalized tables and `PRAGMA quick_check` verification.
- Migration/export commands exist for JSON-to-SQLite and SQLite-to-JSON.
- Backup/restore commands and scripts exist.
- Config mutation flow validates, stores history, and preserves omitted fields on PATCH handlers.
- WebUI group schedule behavior has mocked Playwright regression coverage; live production behavior still depends on target-host validation.
- Config saves use temp-file fsync, atomic rename, and parent-directory fsync.

## 2. Reliability & Error Handling

### 2.1 Error Handling Coverage

- Many errors are checked and propagated.
- API panics are recovered with stack logging.
- API text errors are wrapped into JSON envelopes with `error` and `request_id` for `/api/v1/*`.
- DNS malformed packets are silently dropped without visible counters.
- `/readyz` is now dependency-aware, including DNS listener lifecycle state.

### 2.2 Graceful Degradation

- Upstream failures trigger sequential failover and health state changes.
- Blocklist fetch errors keep previous lists by design.
- Store unavailability returns service errors in handlers.
- No circuit breaker beyond upstream unhealthy marking.
- No retry/backoff system outside health probing/fetch behavior.

### 2.3 Graceful Shutdown

- SIGINT/SIGTERM use a root context.
- DNS and HTTP servers shut down with a 10-second timeout.
- Background goroutines observe context.
- Store/log close paths exist.
- This is generally good, though repeated/concurrent shutdown field mutation deserves race verification.

### 2.4 Recovery

- Store state persists across restart.
- Cache is intentionally in-memory/cold after restart.
- SQLite verification and backup restore help recovery; local rollback drill evidence is
  recorded in `docs/ROLLBACK_DRILL.md`.
- Config save crash safety now matches the file-store atomic write pattern for temp-file fsync, rename, and parent-directory fsync.

## 3. Security Assessment

### 3.1 Authentication & Authorization

- [x] Authentication mechanism is implemented.
- [x] Server-side random sessions are implemented.
- [x] Session expiry and sliding renewal are implemented.
- [x] Authorization checks guard `/api/v1/*` after setup/login.
- [x] Password hashing contract is documented. It uses PBKDF2-SHA256 with 210,000 iterations, not bcrypt/argon2.
- [x] Origin/Referer protection is implemented for unsafe cookie-authenticated API methods.
- [x] Login rate limiting exists.
- [x] Authenticated API rate limiting exists through `server.http.rate_limit_per_minute`.
- [ ] Role-based authorization exists.
- [x] Single-role admin authorization scope is documented in `docs/AUTHORIZATION_SCOPE.md`.

### 3.2 Input Validation & Injection

- [x] JSON decoder rejects unknown fields, trailing data, and oversized bodies.
- [x] SQL access uses parameterized queries by inspection.
- [x] React output escaping protects normal text rendering.
- [x] Command injection risk is low; runtime does not shell out for DNS paths.
- [x] Path traversal in WebUI static serving is mitigated through `path.Clean` and `fs`.
- [x] IDN/A-label normalization is implemented for policy domain sets and API-managed allow/block entries.

### 3.3 Network Security

- [x] TLS support via cert/key config.
- [x] Secure cookie when TLS is active, or when `auth.secure_cookie` is enabled for reverse-proxy TLS termination.
- [x] Security headers exist.
- [x] HSTS header exists when TLS is active or configured.
- [x] CORS is not wildcard; no CORS headers are set.
- [x] Origin/Referer CSRF mitigation exists for unsafe cookie-authenticated API methods.
- [x] HTTP listener defaults to localhost unless explicitly configured otherwise.

### 3.4 Secrets & Configuration

- [x] No hardcoded secrets found.
- [x] `.env` examples are separate.
- [x] Config/backups are documented as sensitive.
- [x] Privacy salt generation exists.
- [x] Git history secret scan evidence is recorded in `docs/SECRET_SCAN.md`.
- [x] Config history and default `sis config show` output redact password hashes and `privacy.log_salt`.
- [ ] Sensitive config masking in all logs was not exhaustively verified.

### 3.5 Security Vulnerabilities Found

| Severity | Finding | Evidence | Impact |
|---|---|---|---|
| Medium | Password hashing differs from original spec | `internal/api/password.go`; `SECURITY.md` documents PBKDF2-SHA256 contract | Operators must preserve compatibility or migrate credentials deliberately |
| Low | `/readyz` can only report listener lifecycle known to the current process | `internal/dns/server.go` exposes `Ready`; API consumes it | Keep startup error monitoring and Go regression tests |
| Low | API rate limiting is coarse per-IP only | `server.http.rate_limit_per_minute` protects authenticated API routes | NAT/shared-admin clients can contend for the same bucket |
| Low | HSTS relies on TLS detection/config | `securityHeaders` sets HSTS when request TLS or configured TLS is active | Reverse-proxy deployments still need correct TLS forwarding/operator docs |

NPM audit: `found 0 vulnerabilities`.

Go vulnerability status: not checked because `govulncheck` is unavailable.

## 4. Performance Assessment

### 4.1 Known Performance Issues

- UDP allocates and copies per packet.
- Policy snapshot copies list map per query.
- DNS cache is single-mutex.
- JSON backend rewrites whole store per mutation.
- Exact top-domain/client maps can grow without the specified approximate Top-K design.

### 4.2 Resource Management

- DoH clients use standard HTTP connection pooling and request timeouts.
- TCP connections are capped.
- UDP queue has back-pressure by dropping.
- Store and log files are closed.
- No explicit process memory limits are configured outside systemd/docker environment.

### 4.3 Frontend Performance

- JS bundle is modest at 71.33 kB gzip.
- No lazy route splitting because the app is a single screen/panel composition.
- No image/media optimization concerns; UI is text/control heavy.
- Core Web Vitals were not measured.

## 5. Testing Assessment

### 5.1 Test Coverage Reality Check

Actually tested by local audit:

- WebUI TypeScript build.
- WebUI ESLint.
- npm audit.

Not locally testable on this host:

- Race detector. `go test -race ./...` requires cgo and failed because `gcc` is not installed.
- Playwright browser execution. Chromium install failed because this Playwright build does not support the host `ubuntu26.04-x64` browser package.

Verified in GitHub Actions:

- Scheduled/manual quality job passes race detector and short fuzz campaigns.
- Browser smoke runs on a supported CI host and covers the first-run and mocked management flows.

Source tests present:

- Broad unit tests for config, DNS, policy, API, store, stats, upstream, CLI helpers.
- Playwright specs for first-run, dashboard, store verify, blocked query, mocked group schedule preservation/editing, login, client edit, upstream CRUD, blocklist inspect, and allow/block list edit flows.

Critical paths without enough visible coverage:

- SPEC §19 scenarios are mapped in `.project/ACCEPTANCE_MATRIX.md`; remaining gaps are live production validation and sustained live production load evidence.
- WebUI management flows now have mocked Playwright specs and CI browser execution.
- Real production install validation on target host.
- Live target-host rollback drill.
- Final security review of auth/session/cookie/config/backup behavior is recorded in
  `docs/SECURITY_REVIEW.md`; external security testing is still pending.
- Readiness dependency checks now have Go tests.

### 5.2 Test Categories Present

- [x] Unit tests - 41 Go test files.
- [ ] Integration tests - no dedicated integration suite found.
- [x] API/endpoint tests - concentrated in `internal/api/server_test.go`.
- [ ] Frontend component tests - absent.
- [x] E2E tests - 3 Playwright specs.
- [x] Benchmark tests - DNS cache, policy evaluation, DNS pipeline, DoH forwarding, and SQLite store benchmarks; longer local baseline is recorded in `docs/PERFORMANCE_BASELINE.md`.
- [x] Fuzz tests - blocklist parsing, domain normalization, policy domain matching, and DNS message edge cases.
- [ ] Load tests - absent.

### 5.3 Test Infrastructure

- [x] Tests can run locally with Go 1.25.9 tooling.
- [x] CI is configured to run Go/Node checks.
- [x] WebUI build/lint runs locally.
- [x] Test data/fixtures are mostly inline/tempdir based.
- [x] Race detector is enforced in CI for scheduled/manual quality runs.

## 6. Observability

### 6.1 Logging

- [x] Query logs are structured JSON.
- [x] Audit logs are separate.
- [x] HTTP access logging exists.
- [x] Request IDs are generated and returned.
- [x] Request IDs are included in access log lines.
- [x] Query log privacy modes exist.
- [x] Log rotation exists.
- [x] Panic logs include stack traces.

### 6.2 Monitoring & Metrics

- [x] `/healthz` exists.
- [x] `/readyz` checks core runtime dependencies and DNS listener lifecycle state.
- [x] Prometheus endpoint exists.
- [x] In-memory counters and API stats exist, including `rate_limited_total` and `malformed_total`.
- [x] Store verification exists.
- [x] Alert-worthy conditions are formalized in `docs/ALERTING.md`.

### 6.3 Tracing

- [ ] Distributed tracing exists.
- [ ] Correlation IDs propagate across service boundaries.
- [x] SIGUSR2 writes goroutine and heap profiles.
- [ ] pprof HTTP endpoint exists.

## 7. Deployment Readiness

### 7.1 Build & Package

- [x] Reproducible-ish scripted builds with `-trimpath`.
- [x] Multi-platform binary compilation.
- [x] Docker posture is documented as unsupported for current v1 scope.
- [x] Version information embedded via ldflags.
- [x] SBOM generation exists.
- [x] Checksums and optional signing exist.

### 7.2 Configuration

- [x] YAML config with env overrides.
- [x] Sensible defaults.
- [x] Startup validation.
- [x] Example config and env files.
- [ ] Feature flags system exists.

### 7.3 Database & State

- [x] JSON and SQLite backends.
- [x] SQLite schema migration path.
- [x] Backup/restore.
- [x] Export/import.
- [ ] Rollback migrations beyond backup/restore.

### 7.4 Infrastructure

- [x] CI/CD pipeline configured.
- [x] Automated release publication.
- [x] Linux systemd install/verify scripts.
- [x] Release smoke and production validation scripts.
- [ ] Zero-downtime deployment support.
- [x] Container/Kubernetes posture is documented as unsupported for current v1 scope.

## 8. Documentation Readiness

- [x] README is extensive.
- [x] Production runbook exists.
- [x] Security policy exists.
- [x] Architecture overview exists.
- [ ] SPEC/IMPLEMENTATION are fully accurate to current code.
- [ ] API reference/OpenAPI exists.
- [ ] Troubleshooting guide exists as a dedicated doc.

## 9. Final Verdict

### Production Blockers

1. SPEC §19 local acceptance evidence is mapped, but production validation still needs real target host/router/LAN/client evidence.
2. Broad-production posture still needs sustained live production load evidence and final scope alignment.
3. Deferred TUI/Unix-socket expectations should stay clearly out of v1 release messaging.

### High Priority

1. Complete strict live-host production validation with real LAN DNS, authenticated API, diagnostics, and real-client observation.
2. Add sustained live-host DNS/API load evidence beyond package benchmark baselines.
3. Calibrate alert thresholds from the target site baseline after live validation.
4. Keep SPEC/IMPLEMENTATION/TASKS aligned if deferred TUI/socket scope changes.

### Recommendations

1. Treat current releases as small-site conditional deployments only.
2. Prefer SQLite for new production installs.
3. Keep HTTP bound to localhost or protected management networks; Origin/Referer checks reduce browser CSRF risk but are not a substitute for network isolation.
4. Complete live production validation evidence before public stable release.
5. Split oversized files after the safety fixes are done.

### Estimated Time to Production Ready

- From current state: **4-6 weeks** for conditional small-site production hardening.
- Minimum viable production fixes only: **5-8 development days**.
- Full production readiness for broad managed-service claims: **8-12 weeks**, mostly due to live validation, sustained load testing, external security testing, and operational alerting.

### Go/No-Go Recommendation

**CONDITIONAL GO** for a tightly controlled home/lab/small-office deployment where HTTP is localhost/trusted-network only, SQLite is preferred, operators take backups, and operators accept that live target-host validation is still pending.

**NO-GO** for broad production, managed-service, untrusted-network, or stable v1 claims. The project still has too many verification gaps for that posture today: incomplete live production validation, limited sustained live-load evidence, no external security review, and no role-based authorization.

The honest read: Sis is not a toy, and the operational scaffolding is unusually serious for this stage. Recent work removed several production blockers: schedule data loss, shallow dependency readiness, undocumented auth hashing, browser-origin mutation gaps, local Go test/vet gaps, CI race/fuzz gaps, CI browser-smoke gaps, and stale v1 TUI/socket expectations. It is still not safe to present as fully production-ready until live production validation is recorded and sustained load/security evidence is stronger.
