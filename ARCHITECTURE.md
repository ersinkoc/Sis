# Sis Architecture

Sis is a privacy-first DNS gateway for home and small office networks. It runs as a single Go binary that serves DNS over UDP/TCP, exposes an authenticated HTTP API, and embeds a React WebUI for day-to-day operation.

The project is currently in early v1 implementation, but the core runtime is already wired end to end: configuration loading and hot reload, DNS pipeline, policy engine, DoH upstream forwarding, query/audit logging, stats aggregation, file-backed persistence, API handlers, CLI commands, and the embedded WebUI shell are present.

## Current Status

```mermaid
mindmap
  root((Sis v1))
    Implemented
      Config load and validation
      Hot reload holder
      DNS UDP/TCP server
      DNS pipeline
      Cache
      Policy engine
      Allowlist and blocklists
      DoH upstream pool
      Upstream health probing
      Query and audit logs
      Stats counters and rollups
      Cookie auth sessions
      HTTP API
      React WebUI
      CLI wrappers
    Temporary
      File-backed JSON store
      Embedded built WebUI assets
    Operational
      SIGHUP reload
      SIGUSR1 log rotation
      SIGUSR2 debug profiles
      systemd example
```

## System Context

Sis sits between LAN clients and upstream DNS-over-HTTPS providers. Users and administrators interact with the same running process through the WebUI, HTTP API, or CLI commands.

```mermaid
flowchart LR
  subgraph LAN["Home / Small Office LAN"]
    Device1["Laptop / Phone / TV"]
    Device2["Router / DHCP clients"]
    Admin["Administrator"]
  end

  subgraph Sis["sis process"]
    DNS["DNS listener<br/>UDP + TCP"]
    API["HTTP API<br/>/api/v1"]
    UI["Embedded WebUI"]
    Runtime["Runtime services<br/>policy, cache, logs, stats"]
  end

  Upstreams["DoH upstreams<br/>Cloudflare, Quad9, etc."]
  Disk["Data directory<br/>config, store, logs, blocklists"]

  Device1 -->|"DNS query"| DNS
  Device2 -->|"DNS query"| DNS
  Admin -->|"Browser"| UI
  Admin -->|"CLI / curl"| API
  UI --> API
  DNS --> Runtime
  API --> Runtime
  Runtime -->|"DoH POST application/dns-message"| Upstreams
  Runtime <--> Disk
```

## Runtime Composition

`cmd/sis/main.go` is the composition root. The `serve` command loads configuration, initializes shared dependencies, starts background workers, then starts the DNS and HTTP servers.

```mermaid
flowchart TB
  Main["cmd/sis<br/>runServe"]
  Loader["config.Loader"]
  Holder["config.Holder"]
  Reloader["config.Reloader"]
  Store["store.Open<br/>sis.db.json"]
  QueryLog["log.Query"]
  AuditLog["log.Audit"]
  Policy["policy.Engine"]
  Upstream["upstream.Pool"]
  Cache["dns.Cache"]
  ClientID["dns.ClientID<br/>ARP + store"]
  Counters["stats.Counters"]
  Pipeline["dns.Pipeline"]
  DNSServer["dns.Server"]
  APIServer["api.Server"]
  Syncer["policy.Syncer"]
  Aggregator["stats.Aggregator"]

  Main --> Loader --> Holder
  Main --> Reloader
  Main --> Store
  Main --> QueryLog
  Main --> AuditLog
  Main --> Counters
  Main --> Policy
  Main --> Upstream
  Main --> Cache
  Main --> ClientID
  Policy --> Store
  ClientID --> Store
  Cache --> Pipeline
  Policy --> Pipeline
  Upstream --> Pipeline
  QueryLog --> Pipeline
  Counters --> Pipeline
  Pipeline --> DNSServer
  Pipeline --> APIServer
  Store --> APIServer
  Syncer --> Policy
  Syncer --> AuditLog
  Aggregator --> Store
  Counters --> Aggregator
```

## Package Map

```mermaid
flowchart LR
  CMD["cmd/sis<br/>binary + CLI"]
  Config["internal/config<br/>YAML, env overrides,<br/>validation, reload"]
  DNS["internal/dns<br/>server, pipeline,<br/>cache, synthetic replies"]
  API["internal/api<br/>HTTP routes, auth,<br/>settings, system ops"]
  Policy["internal/policy<br/>groups, schedules,<br/>lists, sync"]
  Upstream["internal/upstream<br/>DoH client + pool"]
  Store["internal/store<br/>interfaces + file backend"]
  Logs["internal/log<br/>query/audit logs,<br/>fanout, rotation"]
  Stats["internal/stats<br/>counters, histograms,<br/>rollups"]
  WebUI["internal/webui<br/>embedded dist"]
  Version["pkg/version"]
  React["webui<br/>React + Vite + Tailwind"]

  CMD --> Config
  CMD --> DNS
  CMD --> API
  CMD --> Policy
  CMD --> Upstream
  CMD --> Store
  CMD --> Logs
  CMD --> Stats
  CMD --> Version
  DNS --> Config
  DNS --> Policy
  DNS --> Upstream
  DNS --> Logs
  DNS --> Stats
  API --> Config
  API --> DNS
  API --> Policy
  API --> Upstream
  API --> Store
  API --> Logs
  API --> Stats
  API --> WebUI
  Policy --> Config
  Policy --> Store
  Upstream --> Config
  Logs --> Config
  React -->|"built by make webui"| WebUI
```

## DNS Query Flow

The DNS server accepts UDP and TCP on each configured listen address. UDP packets are dispatched through a bounded worker pool; TCP connections are capped with a slot semaphore. Both protocols converge on `dns.Pipeline.Handle`.

```mermaid
sequenceDiagram
  autonumber
  participant Client as LAN Client
  participant Server as dns.Server
  participant Pipeline as dns.Pipeline
  participant Rate as RateLimiter
  participant CID as ClientID
  participant Policy as policy.Engine
  participant Cache as dns.Cache
  participant Pool as upstream.Pool
  participant Log as query log
  participant Stats as stats.Counters

  Client->>Server: UDP/TCP DNS message
  Server->>Pipeline: Request{Msg, SrcIP, Proto, StartedAt}
  Pipeline->>Rate: Allow(SrcIP)
  alt over limit
    Pipeline-->>Server: REFUSED for TCP or dropped UDP
  else allowed
    Pipeline->>Stats: IncQuery()
    Pipeline->>CID: Resolve/Touch client identity
    Pipeline->>Pipeline: handle special names
    alt special response
      Pipeline->>Log: Write local/synthetic entry
      Pipeline-->>Server: local/synthetic response
    else normal domain
      Pipeline->>Policy: Evaluate(qname, qtype, now)
      alt blocked
        Pipeline->>Stats: IncBlocked()
        Pipeline->>Log: Write blocked entry
        Pipeline-->>Server: synthetic block response
      else allowed
        Pipeline->>Cache: Get(cache key)
        alt cache hit
          Pipeline->>Stats: IncCacheHit()
          Pipeline->>Log: Write cache entry
          Pipeline-->>Server: cached response
        else cache miss
          Pipeline->>Stats: IncCacheMiss()
          Pipeline->>Pool: Forward via healthy DoH upstreams
          Pool-->>Pipeline: response or error + attempts
          Pipeline->>Stats: record upstream attempt stats
          Pipeline->>Cache: Put(response)
          Pipeline->>Log: Write upstream/synthetic entry
          Pipeline-->>Server: DNS response
        end
      end
    end
  end
  Server-->>Client: DNS wire response
```

## Pipeline Stages

```mermaid
flowchart LR
  In["Incoming DNS message"] --> Parse["Parse miekg/dns Msg"]
  Parse --> Limit{"Rate limit ok?"}
  Limit -- "no" --> Drop["Drop UDP<br/>or REFUSED TCP"]
  Limit -- "yes" --> Identity["Resolve client identity"]
  Identity --> Special{"Special name?"}
  Special -- "yes" --> Local["Local / synthetic response"]
  Special -- "no" --> Decision{"Policy blocks?"}
  Decision -- "yes" --> Block["Block response<br/>0.0.0.0, ::, or NXDOMAIN"]
  Decision -- "no" --> CacheLookup{"Cache hit?"}
  CacheLookup -- "yes" --> Cached["Serve cached response"]
  CacheLookup -- "no" --> ECS["Strip ECS if enabled"]
  ECS --> Forward["Forward to DoH pool"]
  Forward --> StoreCache["Store response in cache"]
  StoreCache --> Finish["Record stats + query log"]
  Cached --> Finish
  Block --> Finish
  Local --> Finish
  Finish --> Out["DNS response"]
```

## Policy Model

Policy evaluation is group-oriented. Clients resolve to groups through the store-backed client resolver. A query can be allowed globally, allowed by custom/group allowlist, blocked by a group blocklist, blocked by an active schedule, or blocked by the custom blocklist.

```mermaid
flowchart TB
  Client["Client identity<br/>IP / key"] --> Resolver["Client resolver"]
  Resolver --> Group["Group<br/>default fallback"]
  ConfigAllow["Config allowlist"] --> Engine["policy.Engine"]
  CustomAllow["Custom allowlist"] --> Engine
  GroupAllow["Group allowlist"] --> Group
  Blocklists["Compiled blocklists"] --> Engine
  Schedules["Group schedules"] --> Group
  CustomBlock["Custom blocklist"] --> Engine
  Group --> Eval["Policy.Evaluate"]
  Engine --> Eval
  Query["qname + qtype + time"] --> Eval
  Eval --> Decision["Decision<br/>allow or block(reason, list)"]
```

## HTTP API And WebUI

The HTTP server uses Go's `http.ServeMux`. `/healthz`, `/readyz`, setup, and login are public. Most `/api/v1/*` routes require a valid server-side session cookie. The embedded WebUI is served as a fallback route and calls the same API.

```mermaid
flowchart TB
  Browser["Browser"] --> WebUI["Embedded React WebUI"]
  CLI["sis CLI"] --> API
  Curl["curl / integrations"] --> API
  WebUI --> API["api.Server"]

  API --> Middleware["recover -> security headers -> request id -> access log -> auth"]
  Middleware --> Routes["HTTP routes"]

  Routes --> Auth["auth/setup/login/me/logout"]
  Routes --> Stats["stats summary/timeseries/top"]
  Routes --> Logs["query logs + SSE stream"]
  Routes --> Clients["clients"]
  Routes --> Lists["allowlist/custom/blocklists"]
  Routes --> Upstreams["upstreams + test"]
  Routes --> Groups["groups"]
  Routes --> Settings["settings"]
  Routes --> System["cache/config/system"]
  Routes --> QueryTest["query/test"]

  Routes --> Runtime["shared runtime dependencies"]
  Runtime --> Store["store"]
  Runtime --> Pipeline["dns.Pipeline"]
  Runtime --> Policy["policy.Engine"]
  Runtime --> Cache["dns.Cache"]
  Runtime --> Pool["upstream.Pool"]
  Runtime --> Counters["stats.Counters"]
  Runtime --> LogsRuntime["query/audit logs"]
```

## Persistence Layout

The current store implementation is intentionally simple: a JSON file under the configured data directory. Logs and downloaded blocklists live beside it. The `internal/store` package already exposes interfaces, so the backend can be replaced without changing API, policy, or stats callers.

```mermaid
flowchart LR
  DataDir["server.data_dir"] --> DB["sis.db.json"]
  DataDir --> LogsDir["logs/"]
  DataDir --> BlocklistsDir["blocklists/"]
  DataDir --> DebugDir["dbg/"]

  DB --> Clients["clients:*"]
  DB --> Sessions["sessions:*"]
  DB --> CustomLists["customlist:*"]
  DB --> StatsRows["stats:1m/1h/1d:*"]
  DB --> History["config_history:*"]

  LogsDir --> QueryLog["sis-query.log<br/>rotated + optional gzip"]
  LogsDir --> AuditLog["sis-audit.log<br/>rotated + optional gzip"]
  BlocklistsDir --> Downloaded["fetched blocklist data"]
  DebugDir --> Profiles["goroutine + heap profiles"]
```

## Configuration And Reload

Configuration is loaded from YAML, enriched with defaults and `SIS_*` environment overrides, validated, and stored in an atomic holder. Runtime mutation endpoints update the config file and append config history snapshots.

```mermaid
sequenceDiagram
  autonumber
  participant OS as Operator / OS
  participant Loader as config.Loader
  participant Reloader as config.Reloader
  participant Holder as config.Holder
  participant Policy as policy.Engine
  participant Logs as query/audit logs
  participant Pool as upstream.Pool
  participant Cache as dns.Cache
  participant Pipe as dns.Pipeline
  participant Store as ConfigHistoryStore

  OS->>Reloader: SIGHUP or API reload
  Reloader->>Loader: Load YAML + env overrides
  Loader-->>Reloader: validated Config
  Reloader->>Holder: Replace(new config)
  Reloader->>Policy: ReloadConfig
  Reloader->>Logs: Reconfigure logging/privacy
  Reloader->>Pool: Replace upstreams
  Reloader->>Cache: Reconfigure TTL/size
  Reloader->>Pipe: Reconfigure rate limiter
  Reloader->>Store: Append config snapshot
```

## Background Workers

```mermaid
flowchart TB
  Context["process context<br/>SIGINT/SIGTERM"] --> DNS["DNS server"]
  Context --> API["HTTP server"]
  Context --> SIGHUP["SIGHUP watcher"]
  Context --> ARP["ARP table refresher"]
  Context --> Syncer["blocklist syncer"]
  Context --> Health["upstream health prober"]
  Context --> Aggregator["stats aggregator"]
  Context --> SessionGC["expired session cleanup"]
  Context --> OpsSignals["SIGUSR1/SIGUSR2 watcher"]

  SIGHUP --> Reload["config reload"]
  Syncer --> Policy["policy list replacement"]
  Health --> Pool["upstream health state"]
  Aggregator --> StatsStore["1m/1h/1d rollups"]
  SessionGC --> Store["session store"]
  OpsSignals --> Rotate["log rotation"]
  OpsSignals --> Debug["debug profiles"]
```

## Build And Delivery

```mermaid
flowchart LR
  Go["Go sources"] --> Build["make build"]
  WebSrc["webui React/Vite"] --> WebBuild["make webui"]
  WebBuild --> Embedded["internal/webui/dist"]
  Embedded --> Build
  Build --> Binary["bin/sis"]
  Binary --> Serve["sis serve"]
  Binary --> CLI["sis auth/client/group/query/..."]
  Binary --> Release["make release<br/>multi-platform binaries"]
```

## Main Runtime Dependencies

| Area | Implementation |
| --- | --- |
| DNS protocol | `github.com/miekg/dns` |
| Configuration | YAML via `gopkg.in/yaml.v3`, environment overrides through `SIS_*` |
| HTTP server | Go standard library `net/http` |
| Upstream transport | DNS-over-HTTPS using `application/dns-message` |
| WebUI | React, Vite, Tailwind |
| Persistence | `internal/store` interfaces with current JSON file backend |
| Auth | Server-side sessions stored in `store.SessionStore` |
| Observability | Query/audit logs, live fanout, counters, histograms, persisted rollups |

## API Surface

The API is grouped by runtime concern. CLI commands are thin HTTP clients for many of the same routes, which keeps local automation and the WebUI aligned.

```mermaid
flowchart TB
  API["/api/v1"] --> Auth["auth<br/>setup, login, logout, me"]
  API --> Stats["stats<br/>summary, timeseries,<br/>upstreams, top domains, top clients"]
  API --> Logs["logs/query<br/>list + SSE stream"]
  API --> Clients["clients<br/>list, get, patch, delete"]
  API --> Lists["lists<br/>allowlist, custom blocklist,<br/>managed blocklists"]
  API --> Upstreams["upstreams<br/>list, create, patch,<br/>delete, test"]
  API --> Groups["groups<br/>list, get, create,<br/>patch, delete"]
  API --> Settings["settings<br/>get, patch"]
  API --> Query["query/test"]
  API --> System["system<br/>info, cache flush,<br/>config history, reload"]

  Auth --> Store["SessionStore"]
  Stats --> Counters["stats.Counters"]
  Stats --> StatsStore["StatsStore"]
  Logs --> QueryLog["log.Query fanout"]
  Clients --> ClientStore["ClientStore"]
  Lists --> Policy["policy.Engine"]
  Lists --> ConfigFile["config file mutation"]
  Upstreams --> Pool["upstream.Pool"]
  Upstreams --> ConfigFile
  Groups --> ConfigFile
  Settings --> ConfigFile
  Query --> Pipeline["dns.Pipeline"]
  System --> Cache["dns.Cache"]
  System --> Reloader["config.Reloader"]
```

## Authentication Boundary

The public surface is intentionally small. Setup and login create server-side sessions; authenticated requests refresh session expiry with a sliding TTL.

```mermaid
sequenceDiagram
  autonumber
  participant User
  participant API as api.Server
  participant Limiter as login rate limiter
  participant Store as SessionStore
  participant Config as config.Holder

  User->>API: POST /api/v1/auth/login
  API->>Limiter: check username/IP attempts
  API->>Config: read configured users + cookie settings
  API->>Store: Upsert(session token, username, expires_at)
  API-->>User: Set-Cookie: sis_session=...

  User->>API: Authenticated /api/v1/* request
  API->>Store: Get(session token)
  alt valid session
    API->>Store: Upsert(extended expires_at)
    API-->>User: response + refreshed cookie
  else missing or expired
    API-->>User: 401 unauthorized
  end
```

## Data Model

```mermaid
erDiagram
  CLIENT {
    string key
    string type
    string name
    string group
    bool hidden
  }

  SESSION {
    string token
    string username
    time expires_at
  }

  STATS_ROW {
    string bucket
    map counters
  }

  CONFIG_SNAPSHOT {
    time created_at
    string source
    string checksum
    object config
  }

  QUERY_ENTRY {
    time ts
    string client_key
    string client_ip
    string qname
    string qtype
    string rcode
    bool blocked
    string upstream
    bool cache_hit
    int latency_us
  }

  AUDIT_ENTRY {
    time ts
    string actor
    string action
    string target
  }

  CLIENT ||--o{ QUERY_ENTRY : emits
  SESSION }o--|| CLIENT : authenticates_admin_actions
  STATS_ROW ||--o{ QUERY_ENTRY : aggregates
  CONFIG_SNAPSHOT ||--o{ AUDIT_ENTRY : explains_changes
```

## Configuration Mutation Flow

API endpoints that mutate settings, groups, blocklists, or upstreams edit the YAML configuration rather than only changing memory. This makes the config file the durable source of truth, while reload callbacks update live components.

```mermaid
flowchart LR
  Request["Authenticated mutation request"] --> Decode["Decode + validate payload"]
  Decode --> Load["Load current config file"]
  Load --> Edit["Apply focused config edit"]
  Edit --> Validate["config.Validate"]
  Validate --> Save["Save YAML"]
  Save --> History["Append config history"]
  Save --> Holder["Replace config holder"]
  Holder --> Live["Reconfigure live components"]
  Live --> Audit["Write audit entry"]
```

## Observability Flow

```mermaid
flowchart TB
  Query["DNS query completion"] --> Entry["log.Entry"]
  Entry --> Privacy{"privacy.log_mode"}
  Privacy -- full --> Full["keep client fields"]
  Privacy -- hashed --> Hashed["HMAC client key/IP"]
  Privacy -- anonymous --> Anonymous["clear client identity fields"]
  Full --> Fanout["in-memory fanout"]
  Hashed --> Fanout
  Anonymous --> Fanout
  Fanout --> SSE["/api/v1/logs/query/stream"]
  Fanout --> Recent["/api/v1/logs/query"]
  Fanout --> Disk["sis-query.log if enabled"]

  Query --> Counters["stats.Counters"]
  Counters --> Snapshot["API snapshots"]
  Counters --> Aggregator["minute aggregator"]
  Aggregator --> Rollups["StatsStore 1m / 1h / 1d"]
```

## Development Workflow

```mermaid
flowchart LR
  EditGo["Edit Go packages"] --> GoFmt["make fmt"]
  EditWeb["Edit webui"] --> WebBuild["make webui"]
  GoFmt --> Unit["make test"]
  WebBuild --> WebCheck["webui build + lint"]
  Unit --> Coverage["make coverage"]
  WebCheck --> Check["make check"]
  Coverage --> Check
  Check --> Build["make build"]
  Build --> Run["sis serve -config ..."]
```

Useful entry points:

| Task | Start here |
| --- | --- |
| DNS behavior | `internal/dns/pipeline.go`, then `internal/dns/server.go` |
| Policy rules | `internal/policy/engine.go`, `internal/policy/group.go`, `internal/policy/domains.go` |
| API behavior | `internal/api/server.go`, then the route-specific file |
| Config shape | `internal/config/types.go`, `internal/config/validate.go`, `internal/config/load.go` |
| Persistence | `internal/store/store.go`, then `internal/store/file.go` |
| WebUI behavior | `webui/src/App.tsx`, `webui/src/lib/api.ts` |
| Runtime wiring | `cmd/sis/main.go` |

## Important Design Properties

- **Single-process runtime:** DNS, API, WebUI, policy, stats, logs, and background workers share memory and lifecycle.
- **Config-centered behavior:** most runtime choices come from `config.Config`; reload callbacks keep long-lived components in sync.
- **Policy before upstream:** allow/block decisions happen before cache misses are forwarded, preventing unwanted external resolution.
- **Privacy controls in the hot path:** ECS stripping happens before upstream forwarding; log privacy modes are applied before fanout and disk writes.
- **Backend isolation:** the store is accessed through interfaces, making the current file-backed persistence replaceable.
- **Operational simplicity:** common deployment only needs a config file, writable data directory, and one static binary.

## Known Architectural Gaps / Next Hardening Areas

- The store backend is a temporary file-backed JSON database; it is suitable for early v1 development but not ideal for high write concurrency or large installations.
- The WebUI source and embedded `internal/webui/dist` must stay synchronized through `make webui`.
- Runtime reload updates many shared components, but DNS listen addresses themselves are established at server start.
- More explicit architecture-level integration tests would help cover full DNS-to-API-to-store behavior across reloads.
- Long-term persistence, backup, and migration strategy should be finalized before a production v1 release.
