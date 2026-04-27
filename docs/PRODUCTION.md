# Production Runbook

This runbook captures the current production posture for Sis. It is meant to be short,
operational, and honest about the remaining storage trade-offs.

## Current Readiness Decision

Sis is ready for a first small-site production release with the current file-backed JSON
store, provided these constraints are acceptable:

- Target deployments are home, lab, or small office networks.
- The data directory is on a local filesystem, not NFS or object storage.
- Operators take a backup before upgrades or config-heavy changes.
- Very high write concurrency and very large query-history retention are out of scope for
  the first release.

The store is isolated behind `internal/store` interfaces, so a future SQLite backend can be
added without changing DNS, API, policy, or WebUI callers.

## Files To Protect

The configured `server.data_dir` contains runtime state:

- `sis.db.json`: clients, sessions, custom lists, stats rows, and config history
- `logs/`: query and audit logs
- `blocklists/`: downloaded blocklist cache
- `dbg/`: optional SIGUSR2 profile dumps

The config file also contains sensitive operational data such as password hashes and the
privacy log salt. Treat backups as secrets.

## Pre-Upgrade Checklist

```sh
sudo /usr/local/bin/sis backup create -config /etc/sis/sis.yaml -out sis-backup.tar.gz
sudo /usr/local/bin/sis backup verify -in sis-backup.tar.gz
sudo systemctl status sis
sudo ./scripts/verify-linux-service.sh
```

## Upgrade Checklist

Download and verify the release bundle before replacing the installed binary:

```sh
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/sis_linux_amd64
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/sis_linux_arm64
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/sis_darwin_amd64
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/sis_darwin_arm64
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/SHA256SUMS
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/SHA256SUMS.asc
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/release-signing-public-key.asc
curl -LO https://github.com/ersinkoc/Sis/releases/download/v0.1.0/sis.spdx.json
SIS_RELEASE_DIST=. ./scripts/verify-release-artifacts.sh
chmod +x sis_linux_amd64
./sis_linux_amd64 version
```

```sh
sudo systemctl stop sis
sudo install -m 0755 sis_linux_amd64 /usr/local/bin/sis
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

If the service fails after an upgrade, restore the last verified backup:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup restore -in sis-backup.tar.gz -config /etc/sis/sis.yaml -data-dir /var/lib/sis -force
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

## Storage Limits

The JSON store writes the whole logical database through an atomic temp-file and rename flow.
This keeps the implementation simple and crash-resilient for small installs, but it is not a
replacement for SQLite on large installations.

Watch for these signals:

- `sis.db.json` grows into tens of MB.
- Config/history or stats writes become visibly slow.
- The host serves a large network with heavy churn in client/session/stats records.

When these appear, move the deployment to the planned SQLite backend before expanding scope.

## Release Gate

Before pushing a public tag, run:

```sh
./scripts/release-readiness.sh v0.1.0
```

That gate checks branch/tag state, runs the full test gate, builds release artifacts, signs
checksums when signing keys are configured, and runs release smoke.
