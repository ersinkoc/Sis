# API Reference

Sis serves the WebUI and management API from the configured HTTP listener. The API base path
is `/api/v1`. Responses are JSON unless an endpoint explicitly streams Server-Sent Events
or returns `204 No Content`.

This is a maintained route reference for the current API surface. It is not an OpenAPI
contract yet.

## Authentication And Middleware

- `POST /api/v1/auth/setup` and `POST /api/v1/auth/login` are public.
- `/healthz` and `/readyz` are public.
- Other `/api/v1/*` routes require the configured session cookie, default
  `sis_session`.
- The current v1 scope has one authenticated role: every authenticated user is a full
  administrator. See [AUTHORIZATION_SCOPE.md](AUTHORIZATION_SCOPE.md).
- JSON request bodies are limited to 1 MiB, reject unknown fields, and must contain a single
  JSON value.
- Unsafe cookie-authenticated methods (`POST`, `PATCH`, `DELETE`) require same-origin
  `Origin` or `Referer` when those headers are present.
- API errors are returned as JSON envelopes for `/api/v1/*` routes, except the query log
  stream.

Error envelope:

```json
{
  "error": "message",
  "request_id": "..."
}
```

## Health

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `GET` | `/healthz` | no | Liveness check. Returns `{"ok": true}` when the process is serving HTTP. |
| `GET` | `/readyz` | no | Readiness check for config, store, upstreams, DNS pipeline, and DNS listener state. Returns `503` when not ready. |

## Auth

| Method | Path | Auth | Body | Response |
| --- | --- | --- | --- | --- |
| `POST` | `/api/v1/auth/setup` | no | `{"username":"admin","password":"secret123"}` | Creates the first admin, sets a session cookie, returns `{"username":"admin"}`. Requires `auth.first_run` and no existing users. |
| `POST` | `/api/v1/auth/login` | no | `{"username":"admin","password":"secret123"}` | Sets a session cookie, returns `{"username":"admin"}`. Login is rate limited per client IP. |
| `POST` | `/api/v1/auth/logout` | yes | none | Deletes the current session and clears the cookie. Returns `204`. |
| `GET` | `/api/v1/auth/me` | yes | none | Returns `{"username":"admin","expires_at":"..."}`. |

## Stats

| Method | Path | Query | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/stats/summary` | none | Current counters snapshot. |
| `GET` | `/api/v1/stats/timeseries` | `bucket=1m|1h|1d`, `limit=1..1440` | Historical stats rows. Defaults: `bucket=1m`, `limit=60`. |
| `GET` | `/api/v1/stats/upstreams` | none | Per-upstream stats snapshot. |
| `GET` | `/api/v1/stats/top-domains` | `blocked=true|false`, `limit=1..100` | Top domains from the in-memory counters. Defaults: `blocked=false`, `limit=10`. |
| `GET` | `/api/v1/stats/top-clients` | `limit=1..100` | Top clients from the in-memory counters. Defaults to `10`. |

## Query Logs

| Method | Path | Query | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/logs/query` | `client`, `qname`, `blocked=true|false`, `limit=1..1000` | Recent query log entries. Default `limit=100`. |
| `GET` | `/api/v1/logs/query/stream` | none | Server-Sent Events stream. Replays recent entries, then follows live entries. |

SSE events are emitted as `data: <query-log-entry-json>`.

## Clients

| Method | Path | Body | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/clients` | none | List known clients. |
| `GET` | `/api/v1/clients/{key}` | none | Get one client by key. |
| `PATCH` | `/api/v1/clients/{key}` | `{"name":"TV","group":"iot","hidden":false}` | Updates provided fields. `group` must exist; empty group becomes `default`. |
| `DELETE` | `/api/v1/clients/{key}` | none | Forget a client. Returns `204`. |

`name`, `group`, and `hidden` are optional patch fields.

## Groups

Group objects use the config schema documented in [CONFIG_REFERENCE.md](CONFIG_REFERENCE.md).

| Method | Path | Body | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/groups` | none | List configured groups. |
| `POST` | `/api/v1/groups` | group object | Create a group. Returns `201`. |
| `GET` | `/api/v1/groups/{name}` | none | Get one group. |
| `PATCH` | `/api/v1/groups/{name}` | `{"name":"kids","blocklists":["ads"],"allowlist":[],"schedules":[]}` | Updates provided fields. The `default` group cannot be renamed. |
| `DELETE` | `/api/v1/groups/{name}` | none | Delete a group. `default` cannot be deleted. Returns `204`. |

Group patch fields are optional. Config validation runs before changes are persisted.

## Lists

### Global Allowlist

| Method | Path | Body | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/allowlist` | none | Returns `{"domains":[...]}` for the custom allowlist. |
| `POST` | `/api/v1/allowlist` | `{"domain":"example.com"}` | Adds a normalized domain. Returns `201`. |
| `DELETE` | `/api/v1/allowlist/{domain}` | none | Removes a normalized domain. Returns `204`. |

### Custom Blocklist

| Method | Path | Body | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/custom-blocklist` | none | Returns `{"domains":[...]}` for the custom blocklist. |
| `POST` | `/api/v1/custom-blocklist` | `{"domain":"example.com"}` | Adds a normalized domain. Returns `201`. |
| `DELETE` | `/api/v1/custom-blocklist/{domain}` | none | Removes a normalized domain. Returns `204`. |

### Managed Blocklists

Blocklist objects use the config schema documented in
[CONFIG_REFERENCE.md](CONFIG_REFERENCE.md).

| Method | Path | Query/Body | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/blocklists` | none | List configured managed blocklists. |
| `POST` | `/api/v1/blocklists` | blocklist object | Create a managed blocklist. Returns `201`. |
| `PATCH` | `/api/v1/blocklists/{id}` | `{"id":"ads","name":"Ads","url":"file:///tmp/ads.txt","enabled":true,"refresh_interval":"24h"}` | Updates provided fields. |
| `DELETE` | `/api/v1/blocklists/{id}` | none | Delete a managed blocklist and clear its in-memory policy list. Returns `204`. |
| `POST` | `/api/v1/blocklists/{id}/sync` | none | Force sync. Returns accepted count and cache/not-modified flags. |
| `GET` | `/api/v1/blocklists/{id}/entries` | `q`, `limit=1..1000` | Search compiled policy entries. Default `limit=100`. |

## Upstreams

Upstream objects use the config schema documented in [CONFIG_REFERENCE.md](CONFIG_REFERENCE.md).

| Method | Path | Body | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/upstreams` | none | List configured upstreams with `healthy` status. |
| `POST` | `/api/v1/upstreams` | upstream object | Create an upstream. Returns `201`. |
| `PATCH` | `/api/v1/upstreams/{id}` | `{"id":"quad9","name":"Quad9","url":"https://dns.quad9.net/dns-query","bootstrap":["9.9.9.9"],"timeout":"3s"}` | Updates provided fields. |
| `DELETE` | `/api/v1/upstreams/{id}` | none | Delete an upstream. Returns `204`. |
| `POST` | `/api/v1/upstreams/{id}/test` | none | Runs a DoH test query. Returns `rcode`, `latency_us`, and answer count. |

## Settings

| Method | Path | Body | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/settings` | none | Returns mutable cache, privacy, logging, and block-response settings. |
| `PATCH` | `/api/v1/settings` | `{"cache":{...},"privacy":{...},"logging":{...},"block":{...}}` | Replaces provided sections and validates the resulting config. |

The nested section schemas match [CONFIG_REFERENCE.md](CONFIG_REFERENCE.md). Omitted
sections are preserved.

## Query Test

| Method | Path | Body | Description |
| --- | --- | --- | --- |
| `POST` | `/api/v1/query/test` | `{"domain":"example.com","type":"A","client_ip":"192.168.1.50","proto":"api"}` | Executes a DNS query through the live pipeline and returns response metadata. |

`type` defaults to `A`. `proto` defaults to `api`. `client_ip` defaults to the request
remote address, then `127.0.0.1` if unavailable.

Response:

```json
{
  "domain": "example.com.",
  "type": "A",
  "rcode": "NOERROR",
  "source": "cache",
  "latency_us": 123,
  "answers": []
}
```

## System

| Method | Path | Query | Description |
| --- | --- | --- | --- |
| `GET` | `/api/v1/system/info` | none | Returns service/config summary including listeners, data dir, store backend, and first-run status. |
| `GET` | `/api/v1/system/store/verify` | none | Verifies the configured store backend and returns record counts/check results. |
| `POST` | `/api/v1/system/cache/flush` | none | Flushes DNS cache. Returns number of entries flushed. |
| `GET` | `/api/v1/system/config/history` | `limit=1..100` | Returns redacted config snapshots. Default `limit=20`. |
| `POST` | `/api/v1/system/config/reload` | none | Reloads the config file and applies it if validation passes. |

Config history redacts password hashes and `privacy.log_salt`.

## Status Codes

Common status codes:

- `200`: request succeeded.
- `201`: resource created.
- `204`: mutation succeeded with no response body.
- `400`: invalid body, query parameter, or config validation error.
- `401`: missing or invalid session.
- `403`: same-origin protection rejected an unsafe cookie-authenticated request.
- `404`: resource not found.
- `409`: resource conflict or setup already complete.
- `412`: setup required before authenticated API use.
- `429`: rate limited.
- `500`: internal service failure.
- `502`: upstream or blocklist sync failure.
- `503`: required runtime dependency unavailable.
