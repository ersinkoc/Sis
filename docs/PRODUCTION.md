# Production Runbook

This runbook captures the current production posture for Sis. It is meant to be short,
operational, and honest about the remaining storage trade-offs.

## Current Readiness Decision

Sis is ready for a first small-site production release with the current JSON or SQLite
store backends, provided these constraints are acceptable:

- Target deployments are home, lab, or small office networks.
- The data directory is on a local filesystem, not NFS or object storage.
- Operators take a backup before upgrades or config-heavy changes.
- Very high write concurrency and very large query-history retention are out of scope for
  the first release.

The store is isolated behind `internal/store` interfaces. The active backend is configured
with `server.store_backend`; supported values are `json` and `sqlite`. New larger
deployments should prefer `sqlite`, while existing small-site JSON deployments can continue
using `json`. The active backend is exposed by `sis system info` and the WebUI System
panel.
For the full schema, defaults, validation rules, and environment overrides, see
[CONFIG_REFERENCE.md](CONFIG_REFERENCE.md).
For the authenticated management API surface, see [API_REFERENCE.md](API_REFERENCE.md).

## Files To Protect

The configured `server.data_dir` contains runtime state:

- `sis.db.json`: clients, sessions, custom lists, stats rows, and config history
- `sis.db`: SQLite store when `server.store_backend: sqlite` is enabled
- `logs/`: query and audit logs
- `blocklists/`: downloaded blocklist cache
- `dbg/`: optional SIGUSR2 profile dumps

The config file also contains sensitive operational data such as password hashes and the
privacy log salt. Treat backups as secrets.

`sis backup create` stores runtime state as a portable logical `sis.db.json` snapshot for
both JSON and SQLite deployments. `sis backup restore` recreates `sis.db.json` for JSON
configs and rebuilds `sis.db` for SQLite configs.
`sis store verify -config /etc/sis/sis.yaml` confirms the configured store can be read,
prints collection-level record counts, and for SQLite also runs `PRAGMA quick_check`.
The same check is available remotely to authenticated operators through
`sis system store-verify`, `/api/v1/system/store/verify`, or the WebUI System panel.

## HTTP/TLS Exposure

Keep the management HTTP listener on localhost or a trusted management network unless it is
protected by TLS and network policy. When Sis terminates TLS itself
(`server.http.tls: true`), HTTP responses include `Strict-Transport-Security`.

When Sis runs behind a reverse proxy, terminate TLS at the proxy, configure the Sis HTTP
listener on localhost, configure HSTS at the proxy edge, and set `auth.secure_cookie: true`
so session cookies are marked `Secure` even though Sis receives proxied HTTP. The Sis
process only knows about request TLS that reaches it directly or TLS enabled in its own
config; proxy-only TLS must therefore be enforced by the proxy configuration.

## Pre-Upgrade Checklist

```sh
sudo ./scripts/backup-linux-service.sh
sudo systemctl status sis
sudo ./scripts/verify-linux-service.sh
```

## Upgrade Checklist

Download and verify the release bundle before replacing the installed binary:

```sh
./scripts/download-release.sh v0.1.1 dist/v0.1.1
dist/v0.1.1/sis_linux_amd64 version
```

```sh
sudo systemctl stop sis
sudo install -m 0755 dist/v0.1.1/sis_linux_amd64 /usr/local/bin/sis
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

For a first install or a standard upgrade on the host architecture, the wrapper
performs the download, verification, install, systemd enable/start, and live checks:

```sh
sudo ./scripts/install-release-linux.sh v0.1.1
```

For upgrades of an existing host, prefer the backup-first wrapper:

```sh
sudo ./scripts/upgrade-release-linux.sh v0.1.1
```

If the service fails after an upgrade, restore the last verified backup:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup restore -in /var/backups/sis/sis-YYYYMMDDTHHMMSSZ.tar.gz -config /etc/sis/sis.yaml -data-dir /var/lib/sis -force
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

## LAN DNS Validation

After binding DNS to the LAN interface and updating DHCP/router DNS settings, validate the
client-visible path:

```sh
sudo SIS_LAN_VALIDATE_DNS_SERVER=192.168.1.2:53 ./scripts/validate-lan-dns.sh
```

For policy validation, pass a domain that should be blocked:

```sh
sudo SIS_LAN_VALIDATE_DNS_SERVER=192.168.1.2:53 \
  SIS_LAN_VALIDATE_BLOCKED_DOMAIN=blocked.example.com \
  ./scripts/validate-lan-dns.sh
```

The helper checks config validity, UDP DNS, TCP DNS, optional blocked-domain behavior, and
HTTP health/readiness. Set `SIS_LAN_VALIDATE_SKIP_HTTP=1` when HTTP is intentionally not
reachable from the validation environment.
For bind, firewall, router/DHCP, or policy failures, see
[TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## Production Validation Report

After host installation, SQLite dry-run validation, and LAN DNS configuration, generate a
single validation report:

```sh
sudo SIS_PROD_VALIDATE_LAN_DNS_SERVER=192.168.1.2:53 \
  SIS_PROD_VALIDATE_STRICT=1 \
  SIS_PROD_VALIDATE_BLOCKED_DOMAIN=blocked.example.com \
  SIS_PROD_VALIDATE_API_COOKIE='sis_session=...' \
  SIS_PROD_VALIDATE_REAL_CLIENT=1 \
  SIS_PROD_VALIDATE_REAL_CLIENT_ID=192.168.1.50 \
  SIS_PROD_VALIDATE_DIAGNOSTICS=1 \
  ./scripts/run-production-validation.sh
```

The report is written under `sis-validation/` by default and includes command outputs for
service verification, SQLite migration dry-run, LAN DNS validation, and optional diagnostics.
Set `SIS_PROD_VALIDATE_STRICT=1` for release-candidate evidence so incomplete validation
inputs fail before the live checks start.
Set `SIS_PROD_VALIDATE_API_COOKIE='sis_session=...'` to also verify the authenticated
system store-verification API; the generated report redacts the cookie value.
Set `SIS_PROD_VALIDATE_REAL_CLIENT=1` after a real LAN client has queried through Sis to
verify that the client appears in query logs or client inventory. Add
`SIS_PROD_VALIDATE_REAL_CLIENT_ID` to require a specific client IP/key.
Set `SIS_PROD_VALIDATE_DIAGNOSTICS=1` to attach a diagnostics bundle run.
Copy the generated summary into `docs/PRODUCTION_VALIDATION.md` so the live production
evidence is tracked in the repository. To avoid manual copy/paste drift, run
`scripts/update-production-validation-record.sh sis-validation/production-validation-*.md`
from the repository checkout after the report is generated, then fill in the host details
and real-client observation fields.

## Storage Limits

The JSON store writes the whole logical database through an atomic temp-file and rename flow.
This keeps the implementation simple and crash-resilient for small installs, but it is not a
replacement for SQLite on large installations.
The SQLite backend stores logical records in `sis.db`, keeps an indexed collection column
for each KV row, and uses normalized `clients`, `sessions`, `custom_lists`, `stats`, and
`config_history` tables for client/session/list/stats/config-history CRUD while preserving
portable KV payloads for backup/export. It runs `PRAGMA quick_check` during store
verification. Prefer it for deployments that expect more client, session, custom-list,
stats, or config-history churn.

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

Before touching the live service, run the same migration flow against a restored backup copy:

```sh
sudo ./scripts/validate-sqlite-migration.sh
```

To export SQLite state back to JSON manually for inspection:

```sh
sudo /usr/local/bin/sis store export-sqlite-json -data-dir /var/lib/sis -out /var/backups/sis/sis.db.json
```

After large imports, migration testing, or heavy churn, compact the active store while the
service is stopped:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis store compact -data-dir /var/lib/sis -backend sqlite
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

## Diagnostics

For support or incident triage, collect a small diagnostics bundle without including
config file contents, backups, or runtime databases:

```sh
sudo ./scripts/collect-linux-diagnostics.sh
```

Journal logs are skipped by default because they may contain domain or client data.
Set `SIS_DIAG_INCLUDE_JOURNAL=1` only after accepting that exposure.
The bundle includes `sis store verify` output, which reports backend, path, total and
collection-level record counts, and schema version without copying database contents.
Use [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common DNS bind, upstream DoH, first-run,
and SQLite migration recovery flows.

## Release Gate

Before pushing a release candidate tag, confirm that the live validation record is complete:

```sh
./scripts/release-candidate-check.sh vX.Y.Z-rc.1
```

Before pushing any public release tag, run:

```sh
./scripts/release-readiness.sh vX.Y.Z
```

That gate checks branch/tag state, runs the release-candidate evidence gate for prerelease
tags, runs the full test gate, builds release artifacts, signs checksums when signing keys
are configured, and runs release smoke.
