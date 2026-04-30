# SPEC 19 Acceptance Matrix

This matrix maps the SPECIFICATION.md section 19 scenarios to automated evidence.
It separates covered source-level acceptance from remaining host/browser validation.
The source-level integration subset is runnable with `make test-integration`.

| # | Scenario | Automated evidence | Status | Remaining gap |
|---|---|---|---|---|
| 1 | Fresh install, default config | `scripts/smoke.sh` covers auth setup, DNS query, blocklist policy, API summary; `internal/dns/acceptance_test.go` covers default forwarding/blocking with fake DoH and real DNS clients. | Covered | Live target-host validation still required before broad production claims. |
| 2 | Per-client rename | `TestSpec19DNSAcceptanceClientRenameAndGroupMove` auto-discovers a client, renames/moves it in store, verifies subsequent DNS policy and query log metadata. | Covered | Browser UI flow is not covered on this host. |
| 3 | Schedule active | `TestSpec19DNSAcceptanceScheduleActiveInactive` verifies active schedule block response and log metadata. | Covered | Browser UI flow is not covered on this host. |
| 4 | Schedule inactive | `TestSpec19DNSAcceptanceScheduleActiveInactive` verifies inactive schedule allows upstream resolution. | Covered | Browser UI flow is not covered on this host. |
| 5 | Allowlist override | `TestSpec19DNSAcceptanceCorePath` verifies global allowlist override on a blocked domain; smoke also covers allowlist override. | Covered | None beyond live-host validation. |
| 6 | Upstream failover | `TestSpec19DNSAcceptanceFailoverAndCacheHit` verifies primary 5xx failover to backup and unhealthy primary state. | Covered | Health UI cooldown display is not browser-tested here. |
| 7 | Cache hit | `TestSpec19DNSAcceptanceFailoverAndCacheHit` verifies cache-hit logging and no extra upstream hit. | Covered | Latency target is not benchmarked as a hard assertion. |
| 8 | Hot reload | `TestSpec19DNSAcceptanceHotReloadWithoutRestart` verifies upstream/policy reload without listener restart; `TestGroupSchedulePatchAffectsQueryTest` verifies HTTP group schedule mutation affects query/test; smoke covers config reload/history. | Covered | In-flight query continuity and WebUI edit path are not browser-tested here. |
| 9 | Privacy mode | `TestSpec19DNSAcceptancePrivacyModeHashesClientIdentity` verifies hashed client key/IP logging. | Covered | Per-client stats hash aggregation is not isolated in this scenario. |
| 10 | Restart persistence | `TestSpec19DNSAcceptanceRestartPersistence` verifies stored client group/name after restart; smoke verifies runtime restart persistence; `TestSetupPersistsConfigAndSessionAcrossRestart` verifies setup/session persistence. | Covered | Custom lists and stats recovery are covered by store/smoke tests, not a single named SPEC scenario test. |
| 11 | Store verification | `scripts/smoke.sh` covers CLI/API store verification; `TestSystemStoreVerify` covers API JSON store verification. | Covered | WebUI store verification is only covered by Playwright smoke where browser execution is available. |
| 12 | Production validation | `scripts/run-production-validation.sh` preflight mode, `scripts/update-production-validation-record.sh`, docs runbooks, and script smoke tests exist. | Partial | Requires real target host/router/LAN/client evidence and cannot be completed in local unit tests. |

## Cross-Cutting Gaps

- Playwright browser execution is blocked on this host by unsupported Chromium package availability, but CI browser smoke passes on a supported runner.
- Race testing is blocked locally because `go test -race` requires cgo and no C compiler is installed; CI scheduled/manual race testing passes.
- Seeded fuzz targets exist for blocklist parsing, domain normalization, and DNS edge cases; CI scheduled/manual short fuzz campaigns pass.
- Short benchmark baselines now cover DNS cache, policy evaluation, DNS pipeline, DoH forwarding, and SQLite store paths; broad release claims still need sustained production-like load evidence.
