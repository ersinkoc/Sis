# Troubleshooting

This guide covers the first checks to run when a Sis install does not behave as expected.
Prefer the scripts here over ad hoc commands because they exercise the same paths used by
the production validation gate.

## Quick Triage

Start with the live service verifier:

```sh
sudo ./scripts/verify-linux-service.sh
```

Override paths and endpoints when the install is non-default:

```sh
sudo SIS_VERIFY_BIN=/usr/local/bin/sis \
  SIS_VERIFY_CONFIG=/etc/sis/sis.yaml \
  SIS_VERIFY_HTTP_URL=http://127.0.0.1:8080 \
  SIS_VERIFY_DNS_SERVER=127.0.0.1:53 \
  ./scripts/verify-linux-service.sh
```

Useful direct checks:

```sh
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo /usr/local/bin/sis store verify -config /etc/sis/sis.yaml
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
sudo /usr/local/bin/sis query -server 127.0.0.1:53 test example.com A
```

## DNS Bind Failures

Symptoms:

- `systemctl status sis` shows the service exiting during startup.
- DNS queries fail locally or from LAN clients.
- Logs mention permission denied or address already in use for port 53.

Checks:

```sh
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo ss -lntup | grep ':53 '
sudo systemctl status sis
sudo journalctl -u sis -n 100 --no-pager
```

Common fixes:

- Port 53 needs root or the service capability configured by `examples/sis.service`.
- Another DNS service may already be bound, such as `systemd-resolved`, `dnsmasq`, or a
  router/vendor DNS helper.
- For LAN use, set `SIS_DNS_LISTEN=0.0.0.0:53,[::]:53` in `/etc/sis/sis.env`; for local
  development, keep `127.0.0.1:5353`.
- Confirm the router/DHCP server advertises the Sis host IP as DNS, then renew a client DHCP
  lease before expecting real client traffic.

Validate the client-visible path:

```sh
sudo SIS_LAN_VALIDATE_DNS_SERVER=192.168.1.2:53 ./scripts/validate-lan-dns.sh
```

Add a real blocked domain from the deployed policy when checking policy behavior:

```sh
sudo SIS_LAN_VALIDATE_DNS_SERVER=192.168.1.2:53 \
  SIS_LAN_VALIDATE_BLOCKED_DOMAIN=blocked.example.com \
  ./scripts/validate-lan-dns.sh
```

If the validation environment cannot reach the management HTTP listener, use
`SIS_LAN_VALIDATE_SKIP_HTTP=1` only after separately checking `/healthz` and `/readyz`.

## Upstream DoH Failures

Symptoms:

- `/readyz` fails even though `/healthz` passes.
- DNS queries return `SERVFAIL`.
- Upstream health shows unhealthy resolvers.

Checks:

```sh
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo /usr/local/bin/sis upstream -cookie 'sis_session=...' health
sudo /usr/local/bin/sis upstream -cookie 'sis_session=...' test cloudflare
curl -fsS http://127.0.0.1:8080/readyz
```

Common fixes:

- Each upstream URL must be HTTPS and each bootstrap value must be an IP address.
- The host must be able to reach upstream port 443 directly.
- Captive portals, outbound firewalls, or TLS-inspecting proxies can break DoH.
- Keep at least two upstreams configured so sequential failover has somewhere to go.

When debugging from a service host, compare a direct DNS path with a Sis path:

```sh
sudo /usr/local/bin/sis query -server 127.0.0.1:53 test example.com A
curl -fsS https://cloudflare-dns.com/dns-query >/dev/null
```

## First-Run And Login

Symptoms:

- WebUI keeps returning to setup.
- Login succeeds once and later expires unexpectedly.
- CLI live commands return authentication errors.

Checks:

```sh
curl -i http://127.0.0.1:8080/api/v1/auth/me
sudo /usr/local/bin/sis system -cookie 'sis_session=...' info
sudo /usr/local/bin/sis store verify -config /etc/sis/sis.yaml
```

Common fixes:

- Complete first-run setup at `http://127.0.0.1:8080` before using authenticated APIs.
- Use a fresh cookie for CLI live commands: `sis auth login USER PASSWORD`.
- If TLS terminates at a reverse proxy, set `auth.secure_cookie: true` or
  `SIS_AUTH_SECURE_COOKIE=true` so browsers keep sending secure cookies correctly.
- Check `auth.session_ttl`; very short values are useful for tests but confusing in normal
  operation.
- Treat config files and backups as sensitive because they include password hashes, sessions,
  client metadata, and privacy salt material.

## SQLite Migration

Symptoms:

- Store verification fails after changing `server.store_backend`.
- The service starts with empty clients/sessions/stats after a manual migration attempt.
- Backup restore works for JSON but SQLite does not open.

Checks:

```sh
sudo /usr/local/bin/sis store verify -config /etc/sis/sis.yaml
sudo ./scripts/backup-linux-service.sh
sudo ./scripts/validate-sqlite-migration.sh
```

Safe migration path:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup create -config /etc/sis/sis.yaml -out /var/backups/sis/pre-sqlite.tar.gz
sudo /usr/local/bin/sis store migrate-json-to-sqlite -data-dir /var/lib/sis
sudo sed -i 's/store_backend: "json"/store_backend: "sqlite"/' /etc/sis/sis.yaml
sudo /usr/local/bin/sis config check -config /etc/sis/sis.yaml
sudo systemctl start sis
sudo ./scripts/verify-linux-service.sh
```

Use `scripts/validate-sqlite-migration.sh` before touching the live service. It creates a
backup, restores into a temporary directory, migrates that copy, exports SQLite back to JSON,
and validates config loading with `SIS_STORE_BACKEND=sqlite`.

If a manual migration fails, restore the verified backup instead of editing database files:

```sh
sudo systemctl stop sis
sudo /usr/local/bin/sis backup restore -in /var/backups/sis/pre-sqlite.tar.gz \
  -config /etc/sis/sis.yaml \
  -data-dir /var/lib/sis \
  -force
sudo systemctl start sis
```

## Diagnostics Bundle

For support or incident triage:

```sh
sudo ./scripts/collect-linux-diagnostics.sh
```

Journal logs are skipped by default because they may contain domain or client data. Set
`SIS_DIAG_INCLUDE_JOURNAL=1` only after accepting that exposure.
