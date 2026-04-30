# Release Security Review

Review date: 2026-04-30

Reviewed scope: local management authentication, sessions, browser cookie protections,
config persistence, backup/restore handling, diagnostics, and release documentation.

ASSUMPTION: This review is a source, test, and documentation review for the current
release-hardening branch. It is not an external penetration test and it does not include
live target-host validation.

## Outcome

Sis is acceptable for a tightly controlled home, lab, or small-office deployment when the
management listener is kept on localhost or a trusted management network, TLS is used for
remote management, and backups/config files are treated as secrets.

This review does not clear Sis for broad managed-service, untrusted-network, or stable v1
claims. Live production validation, sustained load evidence, and broader security testing
remain required before making those claims.

## Auth And Sessions

- First-run setup and login create local management users and server-side sessions.
- Password hashes use the documented `pbkdf2-sha256` compatibility contract with random
  salt and 210,000 iterations.
- Session tokens are generated server-side, expire according to `auth.session_ttl`, and are
  renewed on authenticated use before expiry.
- Logout deletes the server-side session and clears the browser cookie.
- Login attempts are rate limited per client IP, and authenticated API routes can be rate
  limited through `server.http.rate_limit_per_minute`.

Open risk: all authenticated users currently have full administrative access. Role-based
authorization and external identity integration are tracked as future work for broader
production use.

## Cookie And Browser Protections

- Session cookies are `HttpOnly` and `SameSite=Lax`.
- Cookies are marked `Secure` when Sis serves TLS directly or when `auth.secure_cookie` is
  enabled for reverse-proxy TLS termination.
- Unsafe cookie-authenticated API methods require a same-origin `Origin` or `Referer` when
  those headers are present.
- The API does not emit wildcard CORS headers.
- Security headers are set, and HSTS is enabled when TLS is active or configured.

Open risk: browser same-origin checks reduce CSRF exposure, but they are not a substitute
for localhost binding, trusted management networks, firewall policy, VPNs, or TLS at the
edge.

## Config And Secrets

- Config validation rejects invalid auth users, password hash fields, cookie names, session
  TTL values, listeners, groups, schedules, upstreams, and other typed settings before use.
- Config saves use temp-file write, fsync, atomic rename, and parent-directory fsync.
- `privacy.log_salt` is generated and persisted when hashed logging is enabled without an
  existing salt.
- Config history redacts password hashes and `privacy.log_salt` before exposing snapshots
  through the API.
- Config files are documented as sensitive because they can contain password hashes,
  privacy salts, upstream settings, and operational policy.

Open risk: sensitive masking in every possible log line was not exhaustively proven by this
review. Operators should continue treating diagnostics and logs as potentially sensitive
when sharing incident evidence.

## Backups, Restore, And Diagnostics

- `sis backup create` writes portable logical snapshots for both JSON and SQLite stores.
- `sis backup verify` validates backup archives before restore or release upgrade use.
- `sis backup restore` refuses to overwrite existing target state unless `-force` is used.
- Linux upgrade wrappers take and verify pre-upgrade backups before replacing the binary.
- The diagnostics collector records version, config check, store verification, service, and
  host information without including config files, runtime databases, or backup contents.
- Journal collection is opt-in because logs can contain domain and client metadata.

Open risk: backups contain sessions, client metadata, custom lists, stats, config history,
password hashes, and privacy salts. Backups must be stored and transferred as secrets.

## Evidence

- Local release gate passed with `./scripts/check.sh` using Go 1.25.9 before this review.
- GitHub Actions run `25148590600` passed unit, lint, vulnerability audit, Playwright smoke,
  benchmarks, race detector, fuzz campaigns, and release dry-run jobs before this review.
- Existing focused tests cover setup/login/logout, session renewal, secure cookie behavior,
  same-origin rejection for unsafe cookie-authenticated requests, config history redaction,
  config validation, backup/restore flows, and store verification.

## Remaining Security Work

1. Complete strict live-host production validation with real LAN DNS, authenticated API, and
   real-client observation.
2. Add sustained DNS/API load evidence for the supported small-site production envelope.
3. Decide whether v1 needs role-based admin permissions or can explicitly defer them.
4. Run an external security review before making broad production or managed-service claims.
5. Revisit password hashing after v1 compatibility requirements are settled; any migration
   must continue accepting existing PBKDF2 hashes during rotation or transparent upgrade.
