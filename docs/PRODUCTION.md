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

The store is isolated behind `internal/store` interfaces. The active backend is configured
with `server.store_backend`; supported values are `json` and `sqlite`. New larger
deployments should prefer `sqlite`, while existing small-site JSON deployments can continue
using `json`.

## Files To Protect

The configured `server.data_dir` contains runtime state:

- `sis.db.json`: clients, sessions, custom lists, stats rows, and config history
- `sis.db`: SQLite store when `server.store_backend: sqlite` is enabled
- `logs/`: query and audit logs
- `blocklists/`: downloaded blocklist cache
- `dbg/`: optional SIGUSR2 profile dumps

The config file also contains sensitive operational data such as password hashes and the
privacy log salt. Treat backups as secrets.

## Pre-Upgrade Checklist

```sh
sudo ./scripts/backup-linux-service.sh
sudo systemctl status sis
sudo ./scripts/verify-linux-service.sh
```

## Upgrade Checklist

Download and verify the release bundle before replacing the installed binary:

```sh
./scripts/download-release.sh v0.1.0 dist/v0.1.0
dist/v0.1.0/sis_linux_amd64 version
```

```sh
sudo systemctl stop sis
sudo install -m 0755 dist/v0.1.0/sis_linux_amd64 /usr/local/bin/sis
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

For a first install or a standard upgrade on the host architecture, the wrapper
performs the download, verification, install, systemd enable/start, and live checks:

```sh
sudo ./scripts/install-release-linux.sh v0.1.0
```

For upgrades of an existing host, prefer the backup-first wrapper:

```sh
sudo ./scripts/upgrade-release-linux.sh v0.1.0
```

If the service fails after an upgrade, restore the last verified backup:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup restore -in /var/backups/sis/sis-YYYYMMDDTHHMMSSZ.tar.gz -config /etc/sis/sis.yaml -data-dir /var/lib/sis -force
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

When these appear, move the deployment to SQLite before expanding scope:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup create -config /etc/sis/sis.yaml -out /var/backups/sis/pre-sqlite.tar.gz
sudo /usr/local/bin/sis store migrate-json-to-sqlite -data-dir /var/lib/sis
sudo sed -i 's/store_backend: "json"/store_backend: "sqlite"/' /etc/sis/sis.yaml
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

To export SQLite state back to the JSON backup format for inspection or rollback planning:

```sh
sudo /usr/local/bin/sis store export-sqlite-json -data-dir /var/lib/sis -out /var/backups/sis/sis.db.json
```

## Diagnostics

For support or incident triage, collect a small diagnostics bundle without including
config file contents, backups, or runtime databases:

```sh
sudo ./scripts/collect-linux-diagnostics.sh
```

Journal logs are skipped by default because they may contain domain or client data.
Set `SIS_DIAG_INCLUDE_JOURNAL=1` only after accepting that exposure.

## Release Gate

Before pushing a public tag, run:

```sh
./scripts/release-readiness.sh v0.1.0
```

That gate checks branch/tag state, runs the full test gate, builds release artifacts, signs
checksums when signing keys are configured, and runs release smoke.
