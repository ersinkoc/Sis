# Changelog

All notable changes to Sis are documented here.

This project follows semantic versioning once public version tags are cut. Until the
first stable release, minor versions may still include operational or storage changes
that require careful upgrade notes.

## Unreleased

### Added

- Added an interactive root `install.sh` that resolves the latest GitHub release, verifies
  release artifacts, writes managed service environment overrides, and installs the Linux
  systemd service.

## v0.1.2 - 2026-04-30

### Changed

- Updated operator-facing install, upgrade, release-gate, and issue-template examples
  to reference the current `v0.1.2` release and the JSON/SQLite storage posture.
- Added store verification for JSON/SQLite backends and included it in Linux service
  verification.
- Added store verification output to Linux diagnostics bundles.
- Exposed the active store backend in system info responses and the WebUI System panel.
- Added authenticated API and CLI access to store verification through system operations.
- Added WebUI System panel action for authenticated store verification.
- Added optional authenticated API store verification to production validation reports,
  with report-time cookie redaction.
- Added a helper to import production validation report summaries into the durable
  production validation record.
- Added optional real-client observation validation for production reports.
- Added collection-level store verification counts for JSON and SQLite backends.
- Added a SQLite store migration that records each KV row's collection in an indexed
  SQL column, preparing safer collection-level inspection and future normalization.
- Added a normalized SQLite `clients` table that is kept in sync with the portable KV
  payload and used by SQLite client CRUD operations.
- Added a normalized SQLite `sessions` table that is kept in sync with the portable KV
  payload and used by SQLite session CRUD and expiration cleanup.
- Added a normalized SQLite `custom_lists` table that is kept in sync with the portable KV
  payload and used by SQLite custom allow/block list CRUD operations.
- Added a normalized SQLite `stats` table that is kept in sync with the portable KV
  payload and used by SQLite stats CRUD operations.
- Added a normalized SQLite `config_history` table that is kept in sync with the portable
  KV payload and used by SQLite config history listing.
- Added SQLite schema upgrade-path regression coverage across schema versions 1 through 7.
- Added a release-candidate validation check that blocks RC tagging until the live
  production validation record is complete.
- Added release-candidate validation gate smoke coverage to the main check script.
- Added strict production-validation preflight checks for release-candidate evidence.
- Added a production-validation record update smoke test that proves generated reports can satisfy the release-candidate gate.
- Tightened release-candidate validation so every required production result row must be present and passing.
- Tightened release-candidate validation metadata and summary requirements.
- Added release-readiness prerelease enforcement for production validation evidence.

## v0.1.1 - 2026-04-27

### Added

- Release binary installation and verification instructions in README and the
  production runbook.
- Release download helper script for fetching and verifying all published
  artifacts with one command.
- Linux release install helper that downloads, verifies, installs, enables, and
  checks the service on the target host.
- Linux service backup helper for timestamped, verified operational backups.
- Linux diagnostics helper for support bundles without config, database, or backup contents.
- Linux release upgrade helper that performs a verified pre-upgrade backup before
  installing and verifying a release.
- SQLite store backend support behind `server.store_backend: sqlite`.
- Store migration commands for JSON-to-SQLite import and SQLite-to-JSON export.
- Backup and restore support for SQLite deployments via portable logical store snapshots.
- Non-destructive SQLite migration validation helper for restored backup dry-runs.
- LAN DNS validation helper for UDP/TCP DNS, optional block policy, and health checks.
- Production validation report helper combining service, SQLite, LAN DNS, and diagnostics checks.
- Production validation record template for live host/network evidence.
- Store compaction command for JSON rewrite and SQLite checkpoint/VACUUM maintenance.

## v0.1.0 - 2026-04-27

Initial public release candidate for home and small-office DNS gateway deployments.

### Added

- Release readiness gate covering local checks, release builds, optional checksum
  signing, artifact verification, backup restore smoke tests, and staged Linux
  service verification.
- Cross-platform release artifacts for Linux and macOS on amd64 and arm64.
- SPDX SBOM generation for release bundles.
- Optional GPG signing for `SHA256SUMS`, with release public-key export.
- Release signing key generation helper for preparing GitHub signing secrets.
- Release readiness smoke coverage for the release signing key helper.
- Release artifact verifier for checksums, optional signatures, and SBOM presence.
- Manual GitHub Actions release dry run workflow.
- Security policy with supported versions and private vulnerability reporting flow.
- Dependabot automation for Go modules, WebUI npm packages, and GitHub Actions.
- GitHub issue and pull request templates for release, security, and operations hygiene.
- Linux systemd installer and live service verification script.
- Hardened example Linux service sandbox.
- Local backup create, verify, and restore commands.
- Production storage runbook documenting the current JSON store operating envelope.
- Architecture guide with system diagrams and operational flows.

### Changed

- CI and release workflows run on current GitHub Actions runtime versions.
- Coverage checks run with uncached Go tests for deterministic enforcement.
- Release smoke tests now verify packaged binaries rather than only source-tree
  builds.

### Security

- Runtime deployment documentation now calls out artifact verification, release
  signing, backup sensitivity, and systemd hardening expectations.
- GitHub release dry run verifies signed checksums with the repository release
  signing secret configured.

### Known Limitations

- The current file-backed JSON store is intended for home and small-office use.
  Larger multi-tenant or high-write deployments should wait for a durable database
  backend such as SQLite before treating Sis as a broad production platform.
