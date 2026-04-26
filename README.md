# Sis

Sis is a privacy-first DNS gateway for home and small office networks.

Tagline: "Sorgular siste, cevaplar berrak." / "DNS in the fog. Answers in the clear."

## Status

This repository is in early v1 implementation. The current tree includes:

- Config loading, validation, hot reload holder
- Config history snapshots for API-driven changes
- Config-seeded client metadata
- Store interfaces with a temporary file-backed backend
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

## Usage

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

Run in development mode:

```sh
sis version
sis config check -config examples/sis.yaml
sis config show -config examples/sis.yaml
sis user add -config examples/sis.yaml admin change-me-now
sis serve -config examples/sis.yaml
```

The example config listens on `127.0.0.1:5353` for DNS and `127.0.0.1:8080` for HTTP.
Set `server.http.tls: true` with `cert_file` and `key_file` to serve the API over HTTPS; session cookies become `Secure` automatically.
When `privacy.log_mode: hashed` is enabled with an empty `log_salt`, Sis generates and persists a salt on startup or config update.
Common deployment settings can be overridden with `SIS_*` environment variables, such as `SIS_DNS_LISTEN`, `SIS_HTTP_LISTEN`, `SIS_DATA_DIR`, `SIS_DNS_RATE_LIMIT_QPS`, and `SIS_AUTH_SESSION_TTL`.

Install as a Linux service:

```sh
sudo useradd --system --home /var/lib/sis --shell /usr/sbin/nologin sis
sudo install -d -o root -g root /etc/sis
sudo install -d -o sis -g sis /var/lib/sis
sudo install -m 0640 -o root -g sis examples/sis.yaml /etc/sis/sis.yaml
sudo install -m 0755 bin/sis /usr/local/bin/sis
sudo install -m 0644 examples/sis.service /etc/systemd/system/sis.service
sudo systemctl daemon-reload
sudo systemctl enable --now sis
```

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
sis system -cookie 'sis_session=...' history 10
sis query -server 127.0.0.1:5353 test example.com A
sis query -api http://127.0.0.1:8080 -cookie 'sis_session=...' test example.com A
```

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
```

`make coverage` runs `scripts/coverage.sh`, which fails unless total Go coverage is at least
`COVERAGE_THRESHOLD` (`100.0` by default). CI also runs WebUI install/build/lint and the same
100% Go coverage gate before building the binary.
`make bench` runs the Go benchmark suite with allocation reporting; set `BENCHTIME` or `BENCHCOUNT` for longer local runs.
`make godoc` checks that exported Go declarations have GoDoc comments.
`make preflight` verifies that required local tools such as Go, gofmt, and npm are installed.
`make check` runs the full CI-style gate: Go formatting drift check, WebUI build/lint,
100% Go coverage, and binary build.

The v1 design lives in:

- `ARCHITECTURE.md`
- `.project/SPECIFICATION.md`
- `.project/IMPLEMENTATION.md`
- `.project/TASKS.md`

Release process notes live in `.github/RELEASE.md`.
