# Sis — Specification

**Sis** is a privacy-first DNS gateway for home and small office networks. It listens on classic DNS port 53, applies per-client blocking and tracking policies, and forwards upstream over DNS-over-HTTPS.

- **Repository:** `github.com/ersinkoc/sis`
- **License:** MIT
- **Language:** Go (stdlib-first)
- **Tagline:** *Sorgular siste, cevaplar berrak.* / *DNS in the fog. Answers in the clear.*

---

## 1. Scope

### 1.1 What Sis Is

Sis sits between client devices and upstream DNS resolvers on a home or small-office LAN. It performs:

- Classic DNS (UDP/TCP :53) ingress for any device that speaks DNS
- DNS-over-HTTPS (DoH) egress to chosen upstream resolvers
- Per-client identification (MAC primary, IP fallback)
- Per-group policy enforcement (blocklists, allowlists, schedules)
- Query logging with per-client attribution
- Three management surfaces: CLI, TUI (bubble tea), WebUI (React 19)

### 1.2 What Sis Is Not

- **Not a recursive resolver.** It always forwards upstream. (See: Labyrinth.)
- **Not an authoritative DNS server.** (See: NothingDNS.)
- **Not a network-wide router or DHCP server.** It only handles DNS.
- **Not a DNS firewall for cloud workloads.** Home/SOHO LAN focus.

### 1.3 v1 Goals

1. Drop-in replacement for the LAN's default DNS resolver, configured at the router or per-device.
2. Single Go binary, no external runtime dependencies.
3. Privacy by default: ECS stripped, DoH upstream, optional log anonymization.
4. Per-client visibility: who asked for what, when, and what was blocked.
5. Per-group rules: different policies for kids, IoT, guests, work devices.
6. Time-based schedules: block list X for group Y between hours A and B.

### 1.4 v2 Reserved (Out of Scope for v1)

DHCP server integration, DoT/DoH/DoQ ingress, DNSSEC validation, ACME, MCP server, Raft clustering, OIDC, Pi-hole/AdGuard config import, CGNAT support, IPv6 RA integration, mobile push notifications.

---

## 2. Architecture Overview

### 2.1 Pipeline

Every incoming query flows through a fixed pipeline:

```
1.  Ingress         (UDP/TCP :53)
2.  Parse           (validate DNS message)
3.  Identify        (resolve client: MAC via ARP, fallback to IP)
4.  Resolve Group   (client → group → policy bundle)
5.  Cache Lookup    (return cached answer if fresh)
6.  Schedule Check  (is a schedule rule active right now?)
7.  Blocklist       (apply group's blocklists + active schedule lists)
8.  Allowlist       (override block decision if matched)
9.  Upstream Select (pick first healthy upstream)
10. Forward (DoH)   (HTTP/2 POST application/dns-message)
11. Validate        (response sanity)
12. Cache Store     (respect TTL, clamp to min/max)
13. Log             (structured JSON)
14. Respond         (write to ingress)
```

### 2.2 Component Map

```
                 ┌─────────────────────────────────────┐
                 │           Sis Binary (sis)          │
                 │                                     │
   LAN ──:53────►│  Ingress  ──►  Pipeline  ──►  DoH ──┼──► Upstream
                 │                    │                │
                 │            ┌───────┴───────┐        │
                 │            ▼               ▼        │
                 │        Storage          Logger      │
                 │     (sis.db.json)        (JSON)     │
                 │            ▲                        │
                 │            │                        │
                 │   ┌────────┴────────┐               │
                 │   │                 │               │
   :8080 ◄──HTTP─┼── WebUI       Admin REST API        │
                 │   │                 ▲               │
                 │   │                 │               │
   stdin ◄──TUI──┼── TUI ──────────────┤               │
                 │                     │               │
   shell ◄──CLI──┼── CLI ──────────────┘               │
                 └─────────────────────────────────────┘
```

### 2.3 Concurrency Model

- One UDP listener goroutine, one TCP listener goroutine.
- Worker pool for DNS query processing (configurable, default = NumCPU × 4).
- Single background goroutine for blocklist sync (interval-based).
- Single background goroutine for log rotation.
- Single background goroutine for upstream health checks.
- HTTP server for WebUI/API on its own goroutine pool (stdlib `net/http`).
- All shared state behind `sync.RWMutex` or atomic primitives; no global locks held during upstream I/O.

---

## 3. DNS Behavior

### 3.1 Supported Message Types

- **Question types:** A, AAAA, CNAME, MX, TXT, NS, SOA, PTR, SRV, CAA, ANY (forwarded as-is).
- **Classes:** IN (Internet) only. CH/HS rejected with REFUSED.
- **Opcode:** QUERY only. Other opcodes return NOTIMP.
- **Recursion:** Sis always forwards; RD bit honored, RA bit set in responses.

### 3.2 Special Names

- **`localhost.` / `*.localhost.`** → 127.0.0.1 / ::1, no upstream call.
- **PTR for RFC1918 ranges** without a local zone match → return NXDOMAIN immediately, never leak to upstream (privacy).
- **`use-application-dns.net`** → NXDOMAIN (Firefox canary domain; signals the network handles DNS, disabling Firefox's auto-DoH).

### 3.3 EDNS

- ECS (EDNS Client Subnet) is **stripped from outbound queries by default** to prevent client IP leakage.
- Other EDNS options (cookies, padding) are passed through.
- Maximum UDP response size: 1232 bytes (RFC 6891 recommendation), TC bit set when exceeded → client retries over TCP.

### 3.4 Cache

- In-memory LRU with TTL respect.
- Min TTL clamp (default 60s), max TTL clamp (default 86400s) — both configurable.
- Negative caching per RFC 2308 (NXDOMAIN, NODATA), default cap 3600s.
- Cache key: `(qname, qtype, qclass)` lowercased.
- **Cache is global, not per-client.** Per-client policy is applied *before* cache lookup for blocked domains, *after* cache lookup for allowed domains. (Implementation detail: blocklist/schedule decisions don't poison cache; they are evaluated on every query.)

---

## 4. Client Identification

### 4.1 Identity Resolution

For each incoming query, Sis resolves the client identity in this order:

1. **MAC address** via ARP/NDP table lookup for the source IP.
2. **IP address** if MAC cannot be resolved (cross-subnet, ARP miss).

The resolved identity is the **client key**. MAC keys are normalized to lowercase, colon-separated form (`aa:bb:cc:dd:ee:ff`). IP keys are normalized to canonical form.

### 4.2 Auto-Registration

The first time Sis sees a client key, it auto-creates a client record:

```yaml
key: "aa:bb:cc:dd:ee:ff"          # MAC or IP
type: "mac"                        # or "ip"
name: ""                           # empty until user names it
group: "default"                   # default group
first_seen: 2026-04-25T14:23:11Z
last_seen:  2026-04-25T14:23:11Z
last_ip:    192.168.1.42           # current IP, for display
```

### 4.3 Client Operations

Users can:

- **Rename** a client (assign friendly name).
- **Move** a client to a different group.
- **Hide** a client from dashboards (still logged, just not displayed).
- **Forget** a client (delete record + history). Will re-register on next query.

### 4.4 ARP Resolution

Sis reads ARP/NDP entries from the OS:

- **Linux:** `/proc/net/arp` for IPv4, `/proc/net/ipv6_route` + `ip -6 neigh` parsing for IPv6.
- **macOS/BSD:** `arp -an` and `ndp -an` parsing (development convenience; not the primary target).
- **Windows:** `GetIpNetTable2` Win32 API via syscall (not in v1; Windows reports as IP-only).

ARP table is cached in-memory and refreshed every 30 seconds. A miss falls back to IP-based identity.

---

## 5. Groups & Policies

### 5.1 Groups

A group bundles policy:

```yaml
groups:
  - name: "default"
    blocklists: ["ads", "trackers"]
    allowlist: []
    schedules: []

  - name: "kids"
    blocklists: ["ads", "trackers", "adult"]
    allowlist: ["khanacademy.org"]
    schedules:
      - name: "school-hours"
        days: ["mon", "tue", "wed", "thu", "fri"]
        from: "08:00"
        to:   "15:00"
        block: ["social", "games"]
      - name: "bedtime"
        days: ["all"]
        from: "22:00"
        to:   "07:00"
        block: ["video", "social", "games"]

  - name: "iot"
    blocklists: ["telemetry-strict"]
    allowlist: []
    schedules: []

  - name: "guests"
    blocklists: ["ads", "malware"]
    allowlist: []
    schedules: []
```

Required: `default` group must always exist. Cannot be deleted.

### 5.2 Policy Evaluation

For a query `(client, qname)`:

1. Resolve `client.group`.
2. Compute the **effective blocklist set** = `group.blocklists ∪ active_schedule_blocklists`.
3. Check if `qname` matches any list in the effective set → block candidate.
4. Check `group.allowlist` (and global allowlist) → if matched, **unblock**.
5. If still blocked, return synthetic response (see §5.4).
6. Otherwise, continue to upstream.

Allowlist always wins over blocklist within a group.

### 5.3 Schedules

A schedule is `(days, time-window, additional-blocklists)`. When **now** falls inside the window, those extra lists apply on top of the group's base lists. Schedules cannot *unblock* — only add blocks.

- `days`: array of `mon|tue|wed|thu|fri|sat|sun|all|weekday|weekend`.
- `from` / `to`: 24-hour `HH:MM` in the server's local timezone (configurable).
- Crossing midnight (`22:00` → `07:00`) is supported and treated as one continuous window.
- Timezone is server-wide; per-group TZ is v2.

### 5.4 Block Response

When a query is blocked, Sis returns a synthetic answer:

- **A queries:** `0.0.0.0`
- **AAAA queries:** `::`
- **HTTPS / SVCB queries:** synthetic empty answer (NODATA)
- **Other types:** NXDOMAIN

TTL: 60 seconds (configurable). Block reason and matched list are written to the log but not exposed in the DNS response.

NXDOMAIN-on-block is configurable as an alternative to `0.0.0.0`/`::` for users who prefer stricter signaling to clients.

---

## 6. Blocklists & Allowlists

### 6.1 Blocklist Format

Sis accepts two parser modes:

- **Hosts format** (Steven Black, OISD raw, etc.) — lines like `0.0.0.0 example.com` or `127.0.0.1 example.com`.
- **Domain-only format** — one domain per line.

Comments (`#`) and blank lines are ignored. Wildcards via `*.example.com` are supported and converted to suffix matches.

### 6.2 Blocklist Definition

```yaml
blocklists:
  - id: "ads"
    name: "StevenBlack Unified"
    url: "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
    enabled: true
    refresh_interval: "24h"

  - id: "trackers"
    name: "OISD Big"
    url: "https://big.oisd.nl/"
    enabled: true
    refresh_interval: "24h"

  - id: "adult"
    name: "StevenBlack + porn"
    url: "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/porn-only/hosts"
    enabled: true
    refresh_interval: "24h"
```

Local files are also supported via `file://` URLs.

### 6.3 Sync Behavior

- On startup: each enabled blocklist is fetched (if cache stale or missing).
- Background goroutine ticks at the smallest configured interval; each list refreshes when its own interval is due.
- Fetch uses `If-Modified-Since` and `ETag` when available.
- On fetch failure: keep existing cached version, log warning. Do not zero out an existing list.
- Parsed lists are stored as compiled domain trees (suffix-matchable) for O(labels) lookup.
- Hot-swap: after parsing, atomic pointer swap; in-flight queries are unaffected.

### 6.4 Allowlist

- **Global allowlist:** applies before any group rules; if matched, query is never blocked regardless of group.
- **Per-group allowlist:** applies only to clients in that group.
- Format: same domain-only / wildcard rules as blocklists.

### 6.5 Manual Entries

Users can add domains directly via WebUI/CLI:

- **Custom block list:** `id: "custom"` — managed via API, persisted to storage.
- **Custom allow list:** `id: "custom-allow"` — same.

These are first-class blocklists/allowlists, treated identically to URL-sourced lists.

---

## 7. Upstreams

### 7.1 Upstream Definition

```yaml
upstreams:
  - id: "cloudflare"
    name: "Cloudflare 1.1.1.1"
    url: "https://cloudflare-dns.com/dns-query"
    bootstrap: ["1.1.1.1", "1.0.0.1"]
    timeout: "3s"

  - id: "quad9"
    name: "Quad9"
    url: "https://dns.quad9.net/dns-query"
    bootstrap: ["9.9.9.9", "149.112.112.112"]
    timeout: "3s"

  - id: "google"
    name: "Google"
    url: "https://dns.google/dns-query"
    bootstrap: ["8.8.8.8", "8.8.4.4"]
    timeout: "3s"
```

### 7.2 Bootstrap

Each upstream lists IP addresses used to resolve the upstream's hostname **without** depending on Sis itself (chicken-and-egg). The HTTP client is configured to dial these IPs directly when connecting to the DoH endpoint.

### 7.3 Selection Strategy (v1)

**Sequential failover.** Upstreams are tried in declared order. First healthy upstream is used. If it fails (timeout, 5xx, network error), Sis tries the next.

Failure of the primary triggers a health check; the upstream is marked unhealthy for `unhealthy_cooldown` (default 30s) and skipped during that window.

Round-robin, weighted, latency-based: v2.

### 7.4 Health Check

- Active probe: `A example.com` query every 60s when idle.
- Passive: any query failure increments a failure counter; 3 consecutive failures → unhealthy.
- Recovery: probe must succeed once after cooldown to mark healthy again.

### 7.5 HTTP Client

- HTTP/2 enforced where possible (`Force HTTP/2`).
- Connection pooling with `MaxIdleConnsPerHost = 4`.
- TLS 1.2 minimum, 1.3 preferred.
- Custom `DialContext` honors the `bootstrap` IPs.
- Idle connection timeout: 90s.

---

## 8. Logging

### 8.1 Query Log

One JSON object per query, written to a rotating file (`sis-query.log`):

```json
{
  "ts": "2026-04-25T14:23:11.123Z",
  "client_key": "aa:bb:cc:dd:ee:ff",
  "client_name": "Ahmet's iPhone",
  "client_group": "kids",
  "client_ip": "192.168.1.42",
  "qname": "tiktok.com.",
  "qtype": "A",
  "qclass": "IN",
  "rcode": "NOERROR",
  "answers": ["0.0.0.0"],
  "blocked": true,
  "block_reason": "schedule:bedtime",
  "block_list": "social",
  "upstream": "",
  "cache_hit": false,
  "latency_us": 142,
  "proto": "udp"
}
```

For unblocked queries `blocked: false`, `block_reason: ""`, `upstream` is set, `answers` reflects the upstream response.

### 8.2 Audit Log

Configuration changes and admin actions are logged separately to `sis-audit.log`:

```json
{
  "ts": "2026-04-25T14:30:00Z",
  "actor": "admin",
  "actor_ip": "192.168.1.10",
  "action": "group.update",
  "target": "kids",
  "before": { ... },
  "after":  { ... }
}
```

### 8.3 Rotation & Retention

- Size-based rotation: default 100 MB per file.
- Retention: default 7 days (configurable). Older files are deleted.
- Compression: gzip on rotation (configurable).
- Storage location: `<data_dir>/logs/`.

### 8.4 Privacy Modes

Three log levels for client identifiers:

- **`full`** (default): client_key, client_name, client_ip recorded as-is.
- **`hashed`**: client_key/IP replaced with HMAC-SHA256 (server keeps the salt). Names still visible.
- **`anonymous`**: client identifiers omitted entirely from query log. Audit log unaffected.

Level applies only to query log; statistics dashboards continue to function with `hashed` (per-client aggregation works on the hash).

---

## 9. Configuration

### 9.1 File Layout

Single YAML file, default `/etc/sis/sis.yaml` (Linux) or `./sis.yaml` (working dir):

```yaml
server:
  dns:
    listen: ["0.0.0.0:53", "[::]:53"]
    udp_workers: 0          # 0 = NumCPU * 4
    tcp_workers: 0
  http:
    listen: "0.0.0.0:8080"
    tls: false
    cert_file: ""
    key_file: ""
  data_dir: "/var/lib/sis"

cache:
  max_entries: 100000
  min_ttl: "60s"
  max_ttl: "24h"
  negative_ttl: "1h"

privacy:
  strip_ecs: true
  block_local_ptr: true
  log_mode: "full"          # full | hashed | anonymous
  log_salt: ""              # auto-generated if empty

logging:
  query_log: true
  audit_log: true
  rotate_size_mb: 100
  retention_days: 7
  gzip: true

block:
  response_a: "0.0.0.0"
  response_aaaa: "::"
  response_ttl: "60s"
  use_nxdomain: false

upstreams: [ ... ]          # see §7
blocklists: [ ... ]          # see §6
allowlist:                   # global allowlist
  domains: []

groups: [ ... ]              # see §5
clients: []                  # auto-populated; user edits names/groups

auth:
  users:
    - username: "admin"
      password_hash: "$2a$12$..."
  session_ttl: "24h"
  cookie_name: "sis_session"
```

### 9.2 Hot Reload

`SIGHUP` triggers a reload. Atomic config swap: parse + validate first, swap pointer only if valid. Errors during reload keep the running config. WebUI changes go through the same path internally (REST → validate → persist → swap).

### 9.3 Config Sources

Precedence (highest first):

1. Command-line flags
2. Environment variables (`SIS_*`)
3. Config file
4. Built-in defaults

---

## 10. Storage

### 10.1 What Is Stored

| Data                | Location                     |
|---------------------|------------------------------|
| Config (current)    | YAML file (canonical)        |
| Config history      | `sis.db.json` (last N revisions) |
| Clients             | `sis.db.json`                |
| Custom block/allow  | `sis.db.json`                |
| Sessions (WebUI)    | `sis.db.json`                |
| Cached blocklists   | `<data_dir>/blocklists/`     |
| Query logs          | `<data_dir>/logs/`           |
| Audit logs          | `<data_dir>/logs/`           |
| Stats aggregates    | `sis.db.json` (1m/1h/1d buckets) |

### 10.2 Stats Aggregation

Real-time counters live in memory. Every minute, a tick flushes aggregates to the store:

- Total queries, cache hits, blocks (global)
- Per-client counters (queries, blocks, top domains)
- Per-domain counters (top N maintained, decayed)
- Per-upstream counters (queries, errors, avg latency)

Buckets: 1-minute (kept 24h), 1-hour (kept 30d), 1-day (kept 1y).

---

## 11. Management Surfaces

### 11.1 CLI

```
sis serve [--config FILE]
sis config validate [--config FILE]
sis config show [--config FILE]
sis client list
sis client rename <key> <name>
sis client move <key> <group>
sis client forget <key>
sis group list
sis group add <name>
sis blocklist sync [<id>]
sis blocklist test <domain>
sis allowlist add <domain>
sis allowlist remove <domain>
sis cache flush
sis cache stats
sis query test <domain> [--type A] [--client <ip|mac>]
sis logs tail [--follow] [--client <key>] [--blocked]
sis stats [--since 1h]
sis upstream test
sis upstream health
sis user add <username>
sis user passwd <username>
sis version
```

All commands respect `--config` (path) and `--json` (machine-readable output).

### 11.2 TUI

A single bubble-tea program (`sis tui`) with these views, switched by hotkey:

- **[1] Dashboard** — live QPS, cache hit %, blocked %, top 5 clients, top 5 blocked domains. Sparklines.
- **[2] Live Log** — streaming query log, filterable by `/text` (qname or client).
- **[3] Clients** — list with name, group, last_seen, query count; rename inline (`r`), move group (`g`).
- **[4] Upstreams** — health, latency p50/p95, error rate.
- **[5] Blocklists** — last sync, entry count, sync now (`s`).

Hotkeys: `1-5` switch view, `q` quit, `?` help, `/` filter, `r` refresh.

### 11.3 WebUI

See `WEBUI.md` for full screen-by-screen specification. Summary:

- Stack: React 19, Tailwind v4.1, shadcn/ui, lucide-react.
- Dark / light mode toggle, persisted in localStorage.
- Responsive: mobile (320px+) → tablet → desktop.
- Embedded in the Go binary via `embed`.
- Auth: cookie session after username/password (bcrypt).
- API: REST + JSON, served at `/api/v1/*`.

---

## 12. REST API

Base path: `/api/v1`. All requests after `/auth/login` require a valid session cookie.

### 12.1 Auth

- `POST /auth/login` — `{username, password}` → sets cookie
- `POST /auth/logout` — clears cookie
- `GET  /auth/me` — current user

### 12.2 Stats

- `GET /stats/summary?range=1h|24h|7d` — totals
- `GET /stats/timeseries?metric=qps|blocked&range=...&bucket=1m|1h`
- `GET /stats/top-clients?range=...&limit=10`
- `GET /stats/top-domains?range=...&limit=10&blocked=true|false`
- `GET /stats/upstreams`

### 12.3 Query Log

- `GET /logs/query?client=&qname=&blocked=&since=&until=&limit=&cursor=`
- `GET /logs/query/stream` — Server-Sent Events for live tail

### 12.4 Clients

- `GET    /clients`
- `GET    /clients/{key}`
- `PATCH  /clients/{key}` — `{name?, group?, hidden?}`
- `DELETE /clients/{key}` — forget

### 12.5 Groups

- `GET    /groups`
- `POST   /groups`
- `GET    /groups/{name}`
- `PATCH  /groups/{name}`
- `DELETE /groups/{name}` — except `default`

### 12.6 Blocklists

- `GET    /blocklists`
- `POST   /blocklists`
- `PATCH  /blocklists/{id}`
- `DELETE /blocklists/{id}`
- `POST   /blocklists/{id}/sync` — force refresh
- `GET    /blocklists/{id}/entries?q=&limit=` — search within list

### 12.7 Allowlist

- `GET    /allowlist`
- `POST   /allowlist` — `{domain}`
- `DELETE /allowlist/{domain}`

### 12.8 Upstreams

- `GET    /upstreams`
- `POST   /upstreams`
- `PATCH  /upstreams/{id}`
- `DELETE /upstreams/{id}`
- `POST   /upstreams/{id}/test`

### 12.9 Settings

- `GET   /settings`
- `PATCH /settings`

### 12.10 System

- `GET  /system/info` — version, uptime, build
- `POST /system/cache/flush`
- `POST /system/config/reload`
- `GET  /healthz`
- `GET  /readyz`

---

## 13. Security

### 13.1 Authentication

- Local user/password only in v1.
- Passwords hashed with bcrypt (cost 12).
- Sessions: random 32-byte tokens, stored server-side, HttpOnly + SameSite=Lax cookie.
- Session TTL configurable, default 24h, sliding expiration.
- Brute-force: per-IP login rate limit (5/min), exponential backoff on repeated failures.

### 13.2 Network Exposure

- DNS :53 — exposed to LAN by design.
- HTTP :8080 — bound to all interfaces by default but **not authenticated until first user is created** (first-run wizard requires creating an admin user before any other endpoint becomes reachable).
- HTTPS support via cert/key files in config (no ACME in v1).

### 13.3 Rate Limiting

- DNS: per-client-IP token bucket, default 200 qps, burst 400. Excess queries dropped (UDP) or REFUSED (TCP).
- HTTP: per-IP 100 req/min for `/auth/*`, 600 req/min for other endpoints.

### 13.4 Input Validation

- DNS messages parsed with `miekg/dns`; malformed messages dropped with a counter.
- All API request bodies validated against schema; unknown fields rejected.
- Domain names normalized (lowercase, IDN to A-label) before any comparison.

### 13.5 Secrets

- Bcrypt hashes never returned via API.
- Log salt auto-generated on first run, stored in config file with `0600` permissions.
- Session tokens never logged.

---

## 14. Observability

### 14.1 Health Endpoints

- `GET /healthz` — liveness, returns 200 OK if process is up.
- `GET /readyz` — readiness, 200 OK if DNS listener is bound, blocklists loaded, at least one upstream healthy.

### 14.2 Internal Metrics (Exposed via API)

Counters maintained in-memory and exposed via `/api/v1/stats/*`:

- `dns.queries.total{proto,rcode}`
- `dns.cache.hits.total`, `dns.cache.misses.total`, `dns.cache.size`
- `dns.blocks.total{group,list}`
- `dns.upstream.requests.total{upstream,result}`
- `dns.upstream.latency.histogram{upstream}`
- `process.goroutines`, `process.memory.heap`

Prometheus exporter is reserved for v2.

### 14.3 Build Info

`/api/v1/system/info` returns:

```json
{
  "version": "1.0.0",
  "commit": "abc1234",
  "built_at": "2026-04-25T12:00:00Z",
  "go_version": "go1.23.0",
  "uptime_seconds": 12345
}
```

---

## 15. Performance Targets

For a Raspberry Pi 4 (4-core ARM, 4 GB RAM) class device on a typical home LAN:

| Metric                              | Target       |
|-------------------------------------|--------------|
| Sustained QPS                       | ≥ 5,000      |
| p50 latency (cache hit)             | ≤ 0.5 ms     |
| p50 latency (upstream)              | ≤ 25 ms      |
| p99 latency (upstream)              | ≤ 100 ms     |
| Memory (idle, 100k entries cache)   | ≤ 150 MB     |
| Cold start                          | ≤ 2 s        |
| Blocklist parse (1M domains)        | ≤ 5 s        |
| Binary size (stripped)              | ≤ 25 MB      |

Targets verified via included benchmark harness (`sis bench`).

---

## 16. Dependencies

### 16.1 Runtime Dependencies

- **Go 1.23+**
- **`github.com/miekg/dns`** — DNS message parsing/serialization. Battle-tested, the de facto Go DNS library. Sis uses it for wire format only; all routing/policy logic is hand-written.
- **`github.com/charmbracelet/bubbletea`** — TUI framework.
- **`github.com/charmbracelet/lipgloss`** — TUI styling.
- **`golang.org/x/crypto/bcrypt`** — password hashing.
- **`internal/store` file backend** — embedded JSON persistence behind narrow interfaces.

### 16.2 No Other Runtime Dependencies

- No web framework (use `net/http` directly).
- No CLI library (use `flag` + dispatch).
- No YAML library (encoding/json + a small hand-written YAML subset parser, scoped to what sis.yaml needs). *Decision pending: if the parser scope creeps, fall back to `gopkg.in/yaml.v3`.*
- No HTTP/2 client library (stdlib `net/http` handles it).

### 16.3 Build-Time / Frontend

WebUI frontend dependencies are isolated to `webui/`:

- React 19, React DOM 19
- Tailwind CSS 4.1
- shadcn/ui components
- lucide-react icons
- Vite 5 (build)
- TypeScript 5 strict

The compiled WebUI is embedded into the Go binary via `//go:embed`.

---

## 17. Project Layout

```
sis/
├── cmd/
│   └── sis/                 # main entry point
│       └── main.go
├── internal/
│   ├── dns/                 # DNS pipeline
│   │   ├── server.go        # listeners
│   │   ├── pipeline.go      # query flow
│   │   ├── cache.go         # LRU cache
│   │   ├── client_id.go     # MAC/IP resolution
│   │   ├── arp_linux.go     # /proc/net/arp parser
│   │   └── arp_other.go
│   ├── policy/
│   │   ├── group.go
│   │   ├── schedule.go
│   │   ├── blocklist.go     # parser + tree
│   │   ├── allowlist.go
│   │   └── eval.go          # decision engine
│   ├── upstream/
│   │   ├── doh.go
│   │   ├── pool.go
│   │   ├── health.go
│   │   └── bootstrap.go
│   ├── store/
│   │   ├── store.go         # persistence interfaces
│   │   └── file.go          # JSON file backend
│   ├── log/
│   │   ├── query.go
│   │   ├── audit.go
│   │   └── rotate.go
│   ├── stats/
│   │   ├── counters.go
│   │   └── aggregator.go
│   ├── config/
│   │   ├── config.go
│   │   ├── load.go
│   │   ├── validate.go
│   │   └── reload.go
│   ├── api/
│   │   ├── router.go
│   │   ├── auth.go
│   │   ├── handlers_*.go
│   │   └── middleware.go
│   ├── webui/
│   │   └── embed.go         # //go:embed dist/*
│   ├── tui/
│   │   ├── app.go
│   │   ├── views/
│   │   └── model.go
│   └── cli/
│       ├── root.go
│       └── cmd_*.go
├── webui/                   # React 19 frontend
│   ├── src/
│   ├── index.html
│   ├── package.json
│   └── vite.config.ts
├── docs/
│   ├── SPECIFICATION.md
│   ├── IMPLEMENTATION.md
│   ├── TASKS.md
│   ├── BRANDING.md
│   ├── WEBUI.md
│   └── README.md
├── examples/
│   └── sis.yaml
├── scripts/
│   └── build.sh
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

---

## 18. Acceptance Criteria

A v1 release is considered ready when:

1. All acceptance scenarios in §19 pass on Linux amd64 and arm64.
2. Performance targets (§15) are met on the reference Raspberry Pi 4 hardware.
3. Documentation (SPEC, IMPL, README) is complete.
4. Test coverage ≥ 70% on `internal/dns`, `internal/policy`, `internal/upstream`.
5. WebUI ships dark/light mode, all v1 screens functional, accessible at WCAG AA contrast.
6. Single binary builds: `go build -o sis ./cmd/sis` produces a working artifact.

## 19. Acceptance Scenarios

1. **Fresh install, default config:** Sis serves DNS, blocks ads from StevenBlack, all clients in `default` group, queries resolved via Cloudflare DoH.
2. **Per-client rename:** A new device queries Sis. WebUI shows it auto-registered. Admin renames it "Living Room TV" and moves it to `iot` group. Subsequent queries log the new name and group.
3. **Schedule active:** A client in `kids` group queries `tiktok.com` at 23:00 (within bedtime schedule). Response is `0.0.0.0`, log shows `block_reason: schedule:bedtime, block_list: social`.
4. **Schedule inactive:** Same client queries `tiktok.com` at 14:00 (outside any blocking schedule, social not in base list). Query is forwarded upstream and resolved normally.
5. **Allowlist override:** A domain in `ads` blocklist is added to global allowlist. All clients can resolve it, log shows `blocked: false`.
6. **Upstream failover:** Cloudflare returns 5xx. Sis automatically tries Quad9. Latency reflects the retry. Health UI shows Cloudflare unhealthy with cooldown timer.
7. **Cache hit:** Two queries for `example.com` from different clients. Second query returns from cache, log shows `cache_hit: true`, latency < 1 ms.
8. **Hot reload:** Admin edits a group's schedule via WebUI. Pipeline picks up the change without dropping in-flight queries; audit log records the change with before/after diff.
9. **Privacy mode:** `log_mode: hashed` is set. Query log shows HMAC-SHA256 client keys; per-client stats still aggregate correctly because the hash is stable.
10. **Restart persistence:** Sis is stopped and restarted. Clients, groups, custom lists, and stats are recovered from `sis.db.json`. In-memory cache is cold (expected).

---

*End of SPECIFICATION.*
