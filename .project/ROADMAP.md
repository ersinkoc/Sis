# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-29
> This roadmap prioritizes work needed to bring Sis to production quality.

## Current State Assessment

Sis has a credible core: DNS UDP/TCP ingress, policy enforcement, DoH forwarding, auth, API, embedded WebUI, JSON/SQLite stores, release scripts, CI, backup/restore, and runbooks. The project is plausible for controlled home/lab/small-office deployments, especially if operators accept the documented limitations.

Production blockers are concentrated in live-host validation, broader acceptance coverage,
and remaining hardening rather than basic feature absence. Recent work closed the WebUI
schedule-erasure path, added schedule editing, made `/readyz` dependency-aware, documented
the PBKDF2-SHA256 password hashing contract, added Origin/Referer protection for
cookie-authenticated mutations, and reconciled the project scope documents with the current
WebUI plus HTTP-backed CLI posture.

What is working well: modular Go package layout, strong operational scripts, SQLite migration/export posture, WebUI build/lint, npm audit, DNS pipeline shape, and API coverage.

## Phase 1: Critical Fixes (Week 1-2)

### Must-fix items blocking basic functionality

- [x] Preserve group schedules in WebUI saves - `webui/src/lib/dashboard.ts`, `webui/src/App.tsx`; minimum fix is to send existing `group.schedules` rather than `[]`; estimate 4h.
- [x] Add actual schedule editing to WebUI - group panel should create/edit/delete schedule name, days, from/to, blocklists; estimate 12-20h.
- [x] Implement dependency-aware `/readyz` - verifies config availability, store readability, upstream pool health, DNS pipeline wiring, and DNS listener lifecycle state; estimate 6h.
- [x] Install/verify Go toolchain in local/dev environments - Go 1.25.9 is the project toolchain; full local gate passes, and race/fuzz run in CI where cgo/gcc is available; estimate 2h.
- [x] Decide password hashing contract - documented current PBKDF2-SHA256 compatibility contract and migration requirement in `SECURITY.md`; estimate 6h.
- [x] Add Origin/Referer protection for unsafe cookie-authenticated HTTP methods; cross-origin browser mutations now return 403 while same-origin WebUI and local CLI flows remain compatible; estimate 8h.

## Phase 2: Core Completion (Week 3-6)

### Complete missing core features from specification

- [x] Reconcile TUI scope - TUI/Unix-socket control plane is deferred from current v1 scope; supported management surfaces are WebUI and HTTP-backed CLI.
- [x] Align CLI transport decision - HTTP-backed CLI is documented as intentional for live operations.
- [x] Complete WebUI navigation model - project docs now describe the current single-page operational console instead of an unbuilt multi-route shell.
- [x] Add JSON error envelope for API failures - middleware converts text API errors into `{error, request_id}` responses while preserving streaming; estimate 6h.
- [x] Expand HTTP rate limiting beyond login - `server.http.rate_limit_per_minute` adds a configurable per-IP limiter for authenticated `/api/v1/*`; estimate 6h.
- [x] Add malformed DNS/error counters - `rate_limited_total` and `malformed_total` are counted in live stats, persisted rollups, and WebUI summary; estimate 4h.

## Phase 3: Hardening (Week 7-8)

### Security, error handling, edge cases

- [x] Add HSTS when TLS is enabled and document reverse-proxy expectations.
- [x] Add CSRF tests for unsafe methods.
- [x] Review every config mutation endpoint for partial update semantics and preservation of omitted fields.
- [x] Add request ID to access logs and JSON errors.
- [x] Ensure config save fsyncs temp file and parent directory like store writes.
- [x] Add optional secure cookie override for reverse proxy deployments.
- [x] Run `govulncheck` in CI once Go tooling is available.

## Phase 4: Testing (Week 9-10)

### Comprehensive test coverage

- [x] Map SPEC §19 acceptance scenarios to automated evidence in `.project/ACCEPTANCE_MATRIX.md`, including remaining host/browser/race/fuzz gaps.
- [x] Add integration tests for setup/login/session restart persistence, blocklist sync, and group schedule mutation through query/test.
- [x] Add frontend test coverage for group schedule preservation/editing.
- [x] Add mocked Playwright coverage for login, client edit, group edit, upstream CRUD, blocklist inspect, and allow/block list edits.
- [x] Add scheduled/manual CI quality job for `go test -race` and short fuzz campaigns.
- [x] Add fuzz tests for blocklist parser, domain normalization, and DNS message edge cases.

## Phase 5: Performance & Optimization (Week 11-12)

### Performance tuning and optimization

- [x] Benchmark DNS cache, policy evaluation, DNS pipeline packet handling, and DoH forwarding under representative in-process loads.
- [ ] Replace per-query policy list-map copy with immutable snapshot pointer if benchmarks show pressure.
- [ ] Add UDP buffer pooling if allocation profile warrants it.
- [ ] Evaluate cache sharding if lock contention appears.
- [x] Add SQLite write/read benchmarks for sessions, stats, clients, and config history.
- [ ] Consider exact-map top stats limits/eviction or implement specified count-min/min-heap.
- [x] Track frontend bundle budget; current JS is 71.33 kB gzip.

## Phase 6: Documentation & DX (Week 13-14)

### Documentation and developer experience

- [x] Update `.project/SPECIFICATION.md` to match current decisions: SQLite, HTTP CLI, PBKDF2-SHA256, WebUI scope, TUI status.
- [x] Update `.project/IMPLEMENTATION.md` store, auth, CLI, diagnostics, release, and WebUI/TUI sections for current architecture.
- [x] Add API documentation, preferably OpenAPI generated from route definitions or maintained alongside handlers.
- [x] Add config reference derived from `internal/config/types.go`.
- [x] Add troubleshooting guide for DNS bind failures, upstream DoH failures, first-run setup, and SQLite migration.
- [x] Document local prerequisites explicitly: Go version, Node 24, npm, Playwright browser install.

## Phase 7: Release Preparation (Week 15-16)

### Final production preparation

- [x] Run full CI locally and in GitHub Actions with Go available.
- [ ] Complete live host production validation record in `docs/PRODUCTION_VALIDATION.md`.
- [ ] Run strict production validation with real LAN DNS and real client observation.
- [x] Verify release artifacts and optional checksum signing through the release dry-run workflow.
- [x] Add rollback drill documentation from actual restore test.
- [x] Final security review of auth/session/cookie/config/backups.
- [x] Decide Docker posture: explicitly unsupported or add Dockerfile/compose.

## Beyond v1.0: Future Enhancements

### Features and improvements for future versions

- [x] Prometheus metrics endpoint.
- [x] Optional pprof endpoint protected by local/admin access.
- [ ] Role-based admin permissions (deferred beyond current v1 scope).
- [ ] OIDC or reverse-proxy auth integration.
- [ ] DoT/DoH ingress.
- [ ] DNSSEC validation.
- [ ] Pi-hole/AdGuard import.
- [ ] Multi-node HA or warm standby.
- [ ] Richer WebUI routing, tables, and accessibility audit.

## Effort Summary

| Phase | Estimated Hours | Priority | Dependencies |
|---|---:|---|---|
| Phase 1 | 38-54h | CRITICAL | None |
| Phase 2 | 64-104h | HIGH | Phase 1 |
| Phase 3 | 36-52h | HIGH | Phase 1 |
| Phase 4 | 56-88h | HIGH | Phase 1-2 |
| Phase 5 | 36-64h | MEDIUM | Test harness |
| Phase 6 | 28-44h | MEDIUM | Scope decisions |
| Phase 7 | 24-40h | HIGH | Prior phases |
| **Total** | **282-446h** | | |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| WebUI schedule regression returns | Medium | High | Keep schedule preservation tests and WebUI coverage in Phase 4 |
| DNS listener readiness regresses | Low | High | Keep `dns.Server.Ready` and `/readyz` tests in the Go gate |
| Race bugs hidden by missing cgo compiler | Medium | High | Install gcc/cgo support and run `go test -race ./...` in CI |
| Cookie-auth CSRF bypass through unusual proxy/header behavior | Low | High | Keep Origin/Referer tests, prefer localhost/TLS, consider token-based CSRF if exposed broadly |
| Spec/docs drift misleads operators or contributors | High | Medium | Update SPEC/IMPLEMENTATION/TASKS after scope decisions |
| JSON store write amplification under heavy churn | Medium | Medium | Prefer SQLite for production; monitor store verify counts and DB size |
| TUI remains expected by users but absent | Medium | Medium | Either implement or remove from v1 promise |
