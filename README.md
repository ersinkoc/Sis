# Sis

Sis is a privacy-first DNS gateway for home and small office networks.

Tagline: "Sorgular siste, cevaplar berrak." / "DNS in the fog. Answers in the clear."

## Status

This repository is in early v1 implementation. The current tree includes:

- Config loading, validation, hot reload holder
- Config history snapshots for API-driven changes
- Config-seeded client metadata
- Store interfaces with JSON and SQLite backends
- Query/audit logging with rotation and live fanout
- Runtime logging/privacy reconfiguration
- Policy engine, schedules, allowlists, blocklist parser/fetch/sync
- DNS UDP/TCP server scaffold, cache, synthetic responses, ECS stripping
- DNS per-client token-bucket rate limiting with bounded bucket retention
- DoH upstream client with bootstrap dialing and failover pool
- Upstream health probing and per-upstream stats
- Stats persistence with 1m/1h/1d buckets
- HTTP health, stats summary, live query-log stream, clients, allowlist, custom blocklist, blocklist sync, upstream test, and cache flush endpoints
- Cookie auth with server-side sliding sessions
- Config-backed settings, groups, blocklists, and upstream CRUD endpoints
- Vite/React/Tailwind WebUI shell with persisted light/dark/system theme, setup/login flow, stats summary, query trend, top domain/client analytics, system cache/config operations, query test tool, config history preview, filtered query log, upstream create/edit/test/delete with reset controls, expanded settings edits with reset/dirty-state controls, group create/edit/delete with reset controls, blocklist create/edit/sync/delete/inspect with reset controls, allow/block list edits, searchable client list, and client rename/group/hide/forget actions with reset controls embedded through the Go API server
- WebUI store backend visibility and authenticated store verification from the System panel

See [CHANGELOG.md](CHANGELOG.md) for release scope, upgrade notes, and known limitations.

## Usage

Install the latest Linux release:

```sh
sudo ./scripts/install-release-linux.sh v0.1.1
```

Quick local start:

```sh
make build
./bin/sis config validate -config examples/sis.yaml
./bin/sis serve -config examples/sis.yaml
```

Then open `http://127.0.0.1:8080` and complete first-run setup. The development DNS listener is `127.0.0.1:5353`, so local DNS checks can target it without binding privileged port 53.

Build a local binary:

```sh
make build
./bin/sis version
```

Without `make`, use the scripts directly:

```sh
./scripts/check.sh
./scripts/build.sh
./scripts/smoke.sh
```

Run in development mode:

```sh
sis version
sis config check -config examples/sis.yaml
sis config show -config examples/sis.yaml
sis user add -config examples/sis.yaml admin change-me-now
sis backup create -config examples/sis.yaml -out sis-backup.tar.gz
sis backup verify -in sis-backup.tar.gz
sis serve -config examples/sis.yaml
```

The example config listens on `127.0.0.1:5353` for DNS and `127.0.0.1:8080` for HTTP.
Set `server.http.tls: true` with `cert_file` and `key_file` to serve the API over HTTPS; session cookies become `Secure` automatically.
When `privacy.log_mode: hashed` is enabled with an empty `log_salt`, Sis generates and persists a salt on startup or config update.
Common deployment settings can be overridden with `SIS_*` environment variables, such as `SIS_DNS_LISTEN`, `SIS_HTTP_LISTEN`, `SIS_DATA_DIR`, `SIS_DNS_RATE_LIMIT_QPS`, `SIS_HTTP_RATE_LIMIT_PER_MINUTE`, and `SIS_AUTH_SESSION_TTL`.
The durable store backend is configured with `server.store_backend`; supported values are `json` and `sqlite`.

Install as a Linux service:

```sh
sudo useradd --system --home /var/lib/sis --shell /usr/sbin/nologin sis
sudo install -d -o root -g root /etc/sis
sudo install -d -o sis -g sis /var/lib/sis
sudo install -m 0640 -o root -g sis examples/sis.yaml /etc/sis/sis.yaml
sudo install -m 0640 -o root -g sis examples/sis.env /etc/sis/sis.env
sudo install -m 0755 bin/sis /usr/local/bin/sis
sudo install -m 0644 examples/sis.service /etc/systemd/system/sis.service
sudoedit /etc/sis/sis.env
sudo -u sis /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo systemctl daemon-reload
sudo systemctl enable --now sis
systemctl status sis
```

Or use the installer script after building or unpacking a Linux release binary:

```sh
sudo SIS_INSTALL_BIN=./sis_linux_amd64 ./scripts/install-linux-service.sh
sudo systemctl enable --now sis
sudo ./scripts/verify-linux-service.sh
```

The installer keeps existing `/etc/sis/sis.yaml` and `/etc/sis/sis.env` files,
writing refreshed examples beside them as `.example` files during upgrades.
`scripts/verify-linux-service.sh` checks the installed binary, config, systemd service state,
HTTP health/readiness, and a DNS query. Override `SIS_VERIFY_*` variables for non-default
ports, paths, or staged checks.

Create an operational backup before upgrades or config-heavy changes:

```sh
sudo ./scripts/backup-linux-service.sh
```

Backups include `sis.yaml`, a portable logical `sis.db.json` store snapshot for JSON or
SQLite deployments, and a small manifest.
Treat them as sensitive because they can include password hashes, sessions, client metadata,
custom lists, and the privacy log salt.
Restore refuses to overwrite existing files unless `-force` is passed:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup restore -in sis-backup.tar.gz -config /etc/sis/sis.yaml -data-dir /var/lib/sis -force
sudo systemctl start sis
```
`scripts/validate-sqlite-migration.sh` runs a non-destructive SQLite migration dry-run
against a restored backup copy before changing the live service.

For a direct LAN DNS deployment, uncomment `SIS_DNS_LISTEN=0.0.0.0:53,[::]:53`
and `SIS_DATA_DIR=/var/lib/sis` in `/etc/sis/sis.env`. Keep the HTTP listener
on localhost unless a trusted management network, firewall, or reverse proxy protects it.
After updating router/DHCP DNS settings, validate the LAN path from a client-reachable
address:

```sh
sudo SIS_LAN_VALIDATE_DNS_SERVER=192.168.1.2:53 ./scripts/validate-lan-dns.sh
```
Use `scripts/run-production-validation.sh` to write a timestamped Markdown report that
combines service verification, SQLite migration dry-run, and LAN DNS validation.
Set `SIS_PROD_VALIDATE_STRICT=1` for release-candidate evidence so missing authenticated
API, real-client, diagnostics, LAN DNS, or blocked-domain checks fail before the run starts.
Set `SIS_PROD_VALIDATE_REAL_CLIENT=1` with an authenticated API cookie to include a real
client observation check against query logs or client inventory.
Record the real host results in `docs/PRODUCTION_VALIDATION.md`; after the report is written,
`scripts/update-production-validation-record.sh sis-validation/production-validation-*.md`
can import the summary and results table without overwriting host notes.

Useful early API checks:

```sh
curl http://127.0.0.1:8080/healthz
curl -X POST http://127.0.0.1:8080/api/v1/auth/setup \
  -H 'content-type: application/json' \
  -d '{"username":"admin","password":"change-me-now"}' \
  -c cookies.txt
curl -b cookies.txt http://127.0.0.1:8080/api/v1/stats/summary
curl -b cookies.txt http://127.0.0.1:8080/api/v1/stats/timeseries
curl -b cookies.txt http://127.0.0.1:8080/api/v1/stats/top-domains
curl -b cookies.txt http://127.0.0.1:8080/api/v1/clients
curl -b cookies.txt http://127.0.0.1:8080/api/v1/groups
curl -b cookies.txt http://127.0.0.1:8080/api/v1/custom-blocklist
curl -b cookies.txt http://127.0.0.1:8080/api/v1/settings
curl -b cookies.txt http://127.0.0.1:8080/api/v1/upstreams
curl -b cookies.txt http://127.0.0.1:8080/api/v1/system/config/history
curl -b cookies.txt http://127.0.0.1:8080/api/v1/system/store/verify
curl -b cookies.txt 'http://127.0.0.1:8080/api/v1/blocklists/ads/entries?q=example&limit=50'
curl -b cookies.txt -X POST http://127.0.0.1:8080/api/v1/query/test -d '{"domain":"example.com","type":"A"}'
curl -b cookies.txt 'http://127.0.0.1:8080/api/v1/logs/query?limit=50'
curl -b cookies.txt -N http://127.0.0.1:8080/api/v1/logs/query/stream
```

Runtime signals:

- `SIGHUP`: reload config
- `SIGUSR1`: rotate query/audit logs
- `SIGUSR2`: write goroutine and heap profiles under `<data_dir>/dbg/`

`SIGHUP` reloads policy, upstreams, cache settings, DNS rate limits, query/audit logging settings, and writes a config history snapshot.
`sis system info` and `/api/v1/system/info` include the active `store_backend` so
operators can confirm whether the running service is using JSON or SQLite.

CLI examples:

```sh
sis auth login admin change-me-now
sis user passwd -config examples/sis.yaml admin newer-password
sis client -cookie 'sis_session=...' list
sis client -cookie 'sis_session=...' get 192.168.1.10
sis client -cookie 'sis_session=...' rename 192.168.1.10 "Living Room TV"
sis group -cookie 'sis_session=...' list
sis upstream -cookie 'sis_session=...' health
sis blocklist -cookie 'sis_session=...' add blocked.example.com
sis blocklist -cookie 'sis_session=...' custom
sis blocklist -cookie 'sis_session=...' entries ads example
sis cache -cookie 'sis_session=...' flush
sis stats -cookie 'sis_session=...' top-domains
sis logs -cookie 'sis_session=...' list 50 example.com
sis logs -cookie 'sis_session=...' tail
sis system -cookie 'sis_session=...' info
sis system -cookie 'sis_session=...' store-verify
sis system -cookie 'sis_session=...' history 10
sis query -server 127.0.0.1:5353 test example.com A
sis query -api http://127.0.0.1:8080 -cookie 'sis_session=...' test example.com A
sis backup create -config examples/sis.yaml -out sis-backup.tar.gz
sis backup verify -in sis-backup.tar.gz
sis backup restore -in sis-backup.tar.gz -config restored/sis.yaml -data-dir restored/data
sis store migrate-json-to-sqlite -data-dir ./data
sis store export-sqlite-json -data-dir ./data -out sis.db.json
sis store compact -data-dir ./data -backend sqlite
sis store verify -config examples/sis.yaml
```

`sis store verify` prints total and collection-level record counts so operators can spot
client, session, stats, or config-history growth before choosing maintenance or migration work.
SQLite stores those collections in an indexed metadata column while preserving the portable
logical JSON payload used by backup and restore.
Client, session, custom allow/block list, stats, and config history records are also mirrored
into normalized SQLite tables and served from those tables when the SQLite backend is active.

## Development

```sh
make preflight
make check
make fmt
make test
make coverage
make bench
make godoc
make build
make release
make release-smoke
```

`scripts/check.sh` runs the same main gate without requiring `make`: Go format drift check,
GoDoc, WebUI install/build/lint, Go coverage, Go vet, binary build, and a local serve smoke test.
`scripts/build.sh` creates the release binaries and `dist/SHA256SUMS`.
`scripts/verify-release-artifacts.sh` validates release checksums, optional GPG signatures,
and the SPDX SBOM for a downloaded or locally-built release bundle.
`scripts/download-release.sh vX.Y.Z` downloads all release artifacts into `dist/`,
marks Linux binaries executable, and runs `scripts/verify-release-artifacts.sh`.
`scripts/install-release-linux.sh vX.Y.Z` downloads a release, selects the host Linux
binary, installs the systemd service, enables it, and runs live verification.
`scripts/upgrade-release-linux.sh vX.Y.Z` takes and verifies a pre-upgrade backup,
stops the service, installs the selected release, and runs live verification.
`scripts/release-smoke.sh` verifies release checksums, the Linux artifact, config validation,
backup restore, service hardening directives, and a staged Linux service install without touching the host system.
`scripts/release-readiness.sh vX.Y.Z` checks branch/tag cleanliness, runs the full gate,
builds release artifacts with that version, signs checksums when signing env is configured,
and runs release smoke before a tag is pushed. For prerelease tags such as `vX.Y.Z-rc.N`,
it also runs `scripts/release-candidate-check.sh` before starting the heavy release gate.
`scripts/release-candidate-check.sh vX.Y.Z-rc.N` verifies that
`docs/PRODUCTION_VALIDATION.md` has recorded live host validation evidence before cutting a
release candidate tag. `scripts/check.sh` also runs a fixture smoke test for that gate.
`scripts/verify-linux-service.sh` verifies a live Linux installation; use `SIS_VERIFY_SKIP_*`
variables for staged or partial checks.
`scripts/backup-linux-service.sh` writes a timestamped verified backup for the installed
Linux service; override `SIS_BACKUP_*` variables for non-default paths.
`scripts/collect-linux-diagnostics.sh` writes a support bundle with version, config-check,
store verification, service, and host diagnostics without including config, database, or
backup contents.
`scripts/smoke.sh` starts `bin/sis` with a temporary local config and verifies health/readiness,
DNS queries, blocklist enforcement, auth setup, CLI API access, inventory APIs, custom blocklist
mutation, query logs, stats, cache flush, and config reload/history.
`make coverage` runs `scripts/coverage.sh`, which fails unless total Go coverage is at least
`COVERAGE_THRESHOLD` (`60.0` by default). CI also runs WebUI install/build/lint, Go vet,
the same coverage gate, binary build, and smoke test.
`make bench` runs the Go benchmark suite with allocation reporting; set `BENCHTIME` or `BENCHCOUNT` for longer local runs.
`make godoc` checks that exported Go declarations have GoDoc comments.
`make preflight` verifies that required local tools such as Go, gofmt, and npm are installed.
`make check` runs the full CI-style gate: Go formatting drift check, WebUI build/lint,
Go coverage, Go vet, binary build, and smoke test.

The v1 design lives in:

- `ARCHITECTURE.md`
- `.project/SPECIFICATION.md`
- `.project/IMPLEMENTATION.md`
- `.project/TASKS.md`

Release process notes live in `.github/RELEASE.md`.
Production operation notes live in `docs/PRODUCTION.md`.
Security reporting and release verification notes live in `SECURITY.md`.
