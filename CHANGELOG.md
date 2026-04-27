# Changelog

All notable changes to Sis are documented here.

This project follows semantic versioning once public version tags are cut. Until the
first stable release, minor versions may still include operational or storage changes
that require careful upgrade notes.

## Unreleased

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

### Known Limitations

- The current file-backed JSON store is intended for home and small-office use.
  Larger multi-tenant or high-write deployments should wait for a durable database
  backend such as SQLite before treating Sis as a broad production platform.
- Public release checksums are unsigned until repository signing secrets are
  configured.

## v0.1.0 - Planned

Initial public release candidate for home and small-office DNS gateway deployments.
The release target is the current `Unreleased` scope after final signing setup,
tag creation, GitHub release artifact verification, and live host validation.
