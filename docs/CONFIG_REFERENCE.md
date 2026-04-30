# Configuration Reference

Sis reads a YAML config, applies built-in defaults, applies supported `SIS_*`
environment overrides, then validates the final result. Durations use Go duration syntax
such as `60s`, `24h`, or `168h`.

Start from [examples/sis.yaml](../examples/sis.yaml) for a complete working file.

## Load Order

Precedence, highest first:

1. Supported environment variables.
2. YAML config file.
3. Built-in defaults.

Validate a file before starting the service:

```sh
sis config check -config /etc/sis/sis.yaml
```

Show the resolved config after defaults and environment overrides:

```sh
sis config show -config /etc/sis/sis.yaml
```

By default, `sis config show` redacts password hashes and `privacy.log_salt`. Use
`-secrets` only when intentionally inspecting the raw sensitive values.

## Server

| Field | Type | Default | Environment | Notes |
| --- | --- | --- | --- | --- |
| `server.dns.listen` | list of addresses | `["0.0.0.0:53", "[::]:53"]` | `SIS_DNS_LISTEN` | Comma-separated when set through env. Use `127.0.0.1:5353` for local development. |
| `server.dns.udp_workers` | integer | `0` | `SIS_DNS_UDP_WORKERS` | `0` means runtime default worker sizing. Must be `>= 0`. |
| `server.dns.tcp_workers` | integer | `0` | `SIS_DNS_TCP_WORKERS` | Caps concurrent TCP handling. Must be `>= 0`. |
| `server.dns.udp_size` | integer | `1232` | `SIS_DNS_UDP_SIZE` | Maximum UDP response size. Must be between `0` and `65535`. |
| `server.dns.rate_limit_qps` | integer | `200` | `SIS_DNS_RATE_LIMIT_QPS` | Per-client DNS query rate. `0` disables rate limiting. |
| `server.dns.rate_limit_burst` | integer | `400` | `SIS_DNS_RATE_LIMIT_BURST` | Token bucket burst. Must be `>= 0`. |
| `server.http.listen` | address | `127.0.0.1:8080` | `SIS_HTTP_LISTEN`, `SIS_SERVER_HTTP_LISTEN` | Keep on localhost unless a trusted network, firewall, TLS, or reverse proxy protects it. |
| `server.http.tls` | boolean | `false` | `SIS_HTTP_TLS` | Enables built-in HTTPS. |
| `server.http.cert_file` | path | empty | `SIS_HTTP_CERT_FILE` | Required when `server.http.tls` is true. |
| `server.http.key_file` | path | empty | `SIS_HTTP_KEY_FILE` | Required when `server.http.tls` is true. |
| `server.http.rate_limit_per_minute` | integer | `600` | `SIS_HTTP_RATE_LIMIT_PER_MINUTE`, `SIS_SERVER_HTTP_RATE_LIMIT_PER_MINUTE` | Authenticated API limiter. `0` disables it. |
| `server.data_dir` | path | `./data` | `SIS_DATA_DIR`, `SIS_SERVER_DATA_DIR` | Stores runtime DB, logs, blocklist cache, and debug dumps. Created if missing. |
| `server.store_backend` | enum | `json` | `SIS_STORE_BACKEND`, `SIS_SERVER_STORE_BACKEND` | Supported values: `json`, `sqlite`. |
| `server.tz` | string | `Local` | `SIS_TZ` | IANA timezone for schedules, or `Local`. |

## Cache

| Field | Type | Default | Environment | Notes |
| --- | --- | --- | --- | --- |
| `cache.max_entries` | integer | `100000` | `SIS_CACHE_MAX_ENTRIES` | Maximum DNS cache entries. Must be `>= 0`. |
| `cache.min_ttl` | duration | `60s` | `SIS_CACHE_MIN_TTL` | Minimum positive cache TTL. Must be `>= 0`. |
| `cache.max_ttl` | duration | `24h` | `SIS_CACHE_MAX_TTL` | Maximum positive cache TTL. Must be `>= 0` and `>= cache.min_ttl`. |
| `cache.negative_ttl` | duration | `1h` | `SIS_CACHE_NEGATIVE_TTL` | NXDOMAIN/NODATA cap. Must be `>= 0`. |

## Privacy

| Field | Type | Default | Environment | Notes |
| --- | --- | --- | --- | --- |
| `privacy.strip_ecs` | boolean | `false` | `SIS_PRIVACY_STRIP_ECS` | Example config enables this. Strips EDNS Client Subnet before upstream forwarding. |
| `privacy.block_local_ptr` | boolean | `false` | `SIS_PRIVACY_BLOCK_LOCAL_PTR` | Example config enables this. Prevents leaking local reverse lookups upstream. |
| `privacy.log_mode` | enum | `full` | `SIS_PRIVACY_LOG_MODE` | Supported values: `full`, `hashed`, `anonymous`. |
| `privacy.log_salt` | string | empty | `SIS_PRIVACY_LOG_SALT` | Generated and persisted when `log_mode: hashed` uses an empty salt. Treat as sensitive. |

## Logging

| Field | Type | Default | Environment | Notes |
| --- | --- | --- | --- | --- |
| `logging.query_log` | boolean | `false` | `SIS_LOGGING_QUERY_LOG` | Example config enables query logging. |
| `logging.audit_log` | boolean | `false` | `SIS_LOGGING_AUDIT_LOG` | Example config enables audit logging. |
| `logging.rotate_size_mb` | integer | `100` | `SIS_LOGGING_ROTATE_SIZE_MB` | Must be `>= 0`. |
| `logging.retention_days` | integer | `7` | `SIS_LOGGING_RETENTION_DAYS` | Must be `>= 0`. |
| `logging.gzip` | boolean | `false` | `SIS_LOGGING_GZIP` | Example config enables gzip rotation. |

## Block Responses

| Field | Type | Default | Environment | Notes |
| --- | --- | --- | --- | --- |
| `block.response_a` | IPv4 address | `0.0.0.0` | `SIS_BLOCK_RESPONSE_A` | Synthetic answer for blocked A queries. |
| `block.response_aaaa` | IPv6 address | `::` | `SIS_BLOCK_RESPONSE_AAAA` | Synthetic answer for blocked AAAA queries. |
| `block.response_ttl` | duration | `60s` | `SIS_BLOCK_RESPONSE_TTL` | Must be `>= 0`. |
| `block.use_nxdomain` | boolean | `false` | `SIS_BLOCK_USE_NXDOMAIN` | Return NXDOMAIN instead of address-based synthetic block answers. |

## Upstreams

`upstreams` is a list of DNS-over-HTTPS resolvers:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `id` | string | yes | Unique resolver ID. |
| `name` | string | no | Display name. |
| `url` | HTTPS URL | yes | Must be an `https://` DoH endpoint. |
| `bootstrap` | list of IPs | yes | Used to reach the resolver hostname without depending on Sis. |
| `timeout` | duration | no | Must be `>= 0` when set. |

At least one upstream is required.

## Blocklists

`blocklists` is a list of managed list sources:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `id` | string | yes | Unique blocklist ID. Referenced by groups and schedules. |
| `name` | string | no | Display name. |
| `url` | URL | when enabled | Supported schemes: `http`, `https`, `file`. |
| `enabled` | boolean | no | Enabled lists require `url`. |
| `refresh_interval` | duration | no | Must be `>= 0` when set. |

## Allowlist

| Field | Type | Default | Notes |
| --- | --- | --- | --- |
| `allowlist.domains` | list of domain patterns | `[]` | Global allowlist. Supports exact domains and `*.example.com` wildcards. |

## Groups And Schedules

`groups` controls per-client policy. Exactly one group named `default` is required.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `name` | string | yes | Unique group name. |
| `blocklists` | list of IDs | no | Every ID must exist in `blocklists`. |
| `allowlist` | list of domain patterns | no | Supports exact domains and `*.example.com` wildcards. |
| `schedules` | list | no | Time-windowed extra blocklists. |

Schedule fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `name` | string | yes | Schedule name. |
| `days` | list | yes | Tokens: `mon`, `tue`, `wed`, `thu`, `fri`, `sat`, `sun`, `all`, `weekday`, `weekend`. |
| `from` | `HH:MM` | yes | Start time in `server.tz`. |
| `to` | `HH:MM` | yes | End time in `server.tz`; crossing midnight is supported by policy evaluation. |
| `block` | list of IDs | no | Extra blocklists active during the schedule. Every ID must exist. |

## Clients

`clients` can seed static metadata. Sis also auto-discovers clients at query time.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `key` | string | yes | IP address when `type: ip`; MAC address when `type: mac`. |
| `type` | enum | no | Supported values: `ip`, `mac`. |
| `name` | string | no | Friendly display name. |
| `group` | string | no | Must reference an existing group when set. |
| `hidden` | boolean | no | Hide from dashboards while retaining behavior/logging. |

## Auth

| Field | Type | Default | Environment | Notes |
| --- | --- | --- | --- | --- |
| `auth.users` | list | `[]` | none | Required unless `auth.first_run` is true. |
| `auth.first_run` | boolean | `false` | `SIS_AUTH_FIRST_RUN` | Allows initial admin creation through setup. |
| `auth.session_ttl` | duration | `24h` | `SIS_AUTH_SESSION_TTL` | Must be `>= 0`. |
| `auth.cookie_name` | string | `sis_session` | `SIS_AUTH_COOKIE_NAME` | Must be a valid HTTP cookie name. |
| `auth.secure_cookie` | boolean | `false` | `SIS_AUTH_SECURE_COOKIE` | Force `Secure` cookies when TLS terminates at a reverse proxy. |

User fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `username` | string | yes | Unique local user name. |
| `password_hash` | string | yes | Managed by `sis user add`, `sis user passwd`, or first-run setup. |

## Environment Overrides

Only the variables listed above are supported. Invalid integer, boolean, or duration values
are ignored by the override parser, so prefer `sis config show` after setting environment
variables to confirm the resolved runtime config.

Useful production overrides in `/etc/sis/sis.env`:

```sh
SIS_DNS_LISTEN=0.0.0.0:53,[::]:53
SIS_HTTP_LISTEN=127.0.0.1:8080
SIS_DATA_DIR=/var/lib/sis
SIS_STORE_BACKEND=sqlite
SIS_PRIVACY_LOG_MODE=hashed
SIS_DNS_RATE_LIMIT_QPS=200
SIS_DNS_RATE_LIMIT_BURST=400
SIS_HTTP_RATE_LIMIT_PER_MINUTE=600
SIS_AUTH_SESSION_TTL=24h
SIS_AUTH_SECURE_COOKIE=true
```
