# Sis — Implementation

This document describes the internal design of Sis: package boundaries, data structures, key algorithms, and the concrete contracts each package exposes to the rest of the system. It is the engineering counterpart to `SPECIFICATION.md`.

- **Repository:** `github.com/ersinkoc/sis`
- **Audience:** contributors and the implementer (Claude Code).
- **Scope:** v1 only.

---

## 1. Module & Package Topology

```
github.com/ersinkoc/sis
│
├── cmd/sis              entry point, CLI dispatch
├── internal/
│   ├── config           load/validate/reload, schema
│   ├── store            interface-backed persistence, JSON and SQLite backends
│   ├── dns              listeners, pipeline, cache, client identity
│   ├── policy           groups, schedules, blocklists, allowlists, eval
│   ├── upstream         DoH client, pool, health
│   ├── log              query log, audit log, rotation
│   ├── stats            counters and aggregator
│   ├── api              HTTP REST + SSE
│   ├── webui            //go:embed of compiled React bundle
│   └── tools            release/support tooling
└── pkg/
    └── version          build-time vars (version, commit, date)
```

**Rules:**

- Nothing under `internal/` may import `cmd/` or external WebUI source code.
- `internal/dns` may import `policy`, `upstream`, `log`, `stats`, `config`.
- `internal/api` is the only package that depends on all others (it's the integration layer).
- No package imports a sibling that depends on it (no cycles enforced by Go anyway, but kept linear by convention).
- All HTTP handlers live in `internal/api`; never inside domain packages.

---

## 2. Bootstrapping

### 2.1 Lifecycle

```
main.go
  parse flags
  load config (file → env → flags overlay)
  open configured store backend (`json` or `sqlite`)
  build dependency graph
  start subsystems in order
  wait for SIGINT/SIGTERM/SIGHUP
  shutdown in reverse order
```

### 2.2 Startup Order

1. `store.Open()`
2. `log.Open(query, audit)`
3. `stats.New()`
4. `policy.New(store, blocklistFetcher)` — loads cached lists from disk before any network fetch.
5. `upstream.NewPool(config.Upstreams)`
6. `dns.NewServer(policy, upstream, cache, stats, log)`
7. `api.NewServer(...)` — serves WebUI + REST.
8. **Background workers:** blocklist sync ticker, upstream health prober, log rotator, stats aggregator.

### 2.3 Shutdown Order

Reverse of startup. Each subsystem honors a `context.Context` for cancellation; `Shutdown(ctx)` waits for in-flight work or the context deadline (default 10 s).

### 2.4 Signal Handling

| Signal     | Behavior                                           |
|------------|----------------------------------------------------|
| `SIGINT`   | Graceful shutdown                                  |
| `SIGTERM`  | Graceful shutdown                                  |
| `SIGHUP`   | Reload config from disk                            |
| `SIGUSR1`  | Force log rotation                                 |
| `SIGUSR2`  | Dump goroutine + heap profile to `<data_dir>/dbg/` |

---

## 3. Configuration (`internal/config`)

### 3.1 Structures

```go
type Config struct {
    Server     Server      `yaml:"server"`
    Cache      Cache       `yaml:"cache"`
    Privacy    Privacy     `yaml:"privacy"`
    Logging    Logging     `yaml:"logging"`
    Block      Block       `yaml:"block"`
    Upstreams  []Upstream  `yaml:"upstreams"`
    Blocklists []Blocklist `yaml:"blocklists"`
    Allowlist  Allowlist   `yaml:"allowlist"`
    Groups     []Group     `yaml:"groups"`
    Auth       Auth        `yaml:"auth"`
}

type Server struct {
    DNS     DNSServer    `yaml:"dns"`
    HTTP    HTTPServer   `yaml:"http"`
    DataDir string       `yaml:"data_dir"`
    TZ      string       `yaml:"tz"` // IANA name, default "Local"
}

type DNSServer struct {
    Listen     []string      `yaml:"listen"`
    UDPWorkers int           `yaml:"udp_workers"`
    TCPWorkers int           `yaml:"tcp_workers"`
    UDPSize    int           `yaml:"udp_size"`     // default 1232
}

type Group struct {
    Name       string     `yaml:"name"`
    Blocklists []string   `yaml:"blocklists"`
    Allowlist  []string   `yaml:"allowlist"`
    Schedules  []Schedule `yaml:"schedules"`
}

type Schedule struct {
    Name  string   `yaml:"name"`
    Days  []string `yaml:"days"`
    From  string   `yaml:"from"` // HH:MM
    To    string   `yaml:"to"`   // HH:MM
    Block []string `yaml:"block"` // blocklist ids
}
// (other types follow the spec; see SPECIFICATION §9)
```

### 3.2 Load Path

```go
type Loader struct {
    Path string
}

func (l *Loader) Load() (*Config, error) {
    raw, err := os.ReadFile(l.Path)
    if err != nil { return nil, err }
    var c Config
    if err := yaml.Unmarshal(raw, &c); err != nil { return nil, err }
    applyEnvOverrides(&c)
    if err := Validate(&c); err != nil { return nil, err }
    return &c, nil
}
```

YAML library: **`gopkg.in/yaml.v3`** is accepted as a pinned dependency (decision recorded in SPEC §16.2). Hand-rolling YAML for a moving config is more risk than benefit.

### 3.3 Validation

`Validate(*Config) error` enforces:

- Default group exists exactly once.
- All `groups[*].blocklists` and `groups[*].schedules[*].block` references resolve to declared blocklist IDs.
- All `groups[*].allowlist` patterns are valid domains or wildcards.
- Schedule `from`/`to` parse as `HH:MM`; `days` only contains known tokens.
- At least one upstream defined.
- Each upstream has at least one bootstrap IP.
- `auth.users` non-empty OR `auth.first_run: true` (the first-run wizard sets this).
- `data_dir` exists or is creatable.
- `cache.min_ttl <= cache.max_ttl`.
- Returns multi-error (all problems at once, not bail-on-first).

### 3.4 Hot Reload

```go
type Holder struct {
    cur atomic.Pointer[Config]
}

func (h *Holder) Get() *Config        { return h.cur.Load() }
func (h *Holder) Replace(c *Config)   { h.cur.Store(c) }
```

Subsystems that need config read it through `Holder.Get()` — never cached locally. On `SIGHUP`, the loader runs, validates, and only swaps the pointer on success. Subsystems may register optional `OnReload(old, new *Config) error` callbacks (e.g., upstream pool needs to rebuild HTTP clients if upstream URLs change).

---

## 4. Storage (`internal/store`)

### 4.1 Interface

```go
type Store interface {
    Clients() ClientStore
    CustomLists() CustomListStore
    Sessions() SessionStore
    Stats() StatsStore
    ConfigHistory() ConfigHistoryStore
    Close() error
}

type ClientStore interface {
    Get(key string) (*Client, error)
    List() ([]*Client, error)
    Upsert(*Client) error
    Delete(key string) error
}

// (similar narrow interfaces per concern)
```

### 4.2 Backend

The implementation exposes narrow store interfaces and supports two local backends:

- `json`: file-backed JSON at `<data_dir>/sis.db.json`, written with temp-file, fsync,
  atomic rename, and parent-directory fsync.
- `sqlite`: pure-Go SQLite at `<data_dir>/sis.db`, with portable KV payloads plus
  normalized operational tables for clients, sessions, custom lists, stats, and config
  history.

The logical payloads use one keyspace per concern:

```
clients:<key>           → Client JSON
customlist:<id>:<dom>   → 1
session:<token>         → Session JSON
stats:1m:<bucket>       → counters
stats:1h:<bucket>       → counters
stats:1d:<bucket>       → counters
confhist:<ts>           → snapshot
```

### 4.3 Migrations

A migration registry runs at `Open()`:

```go
type Migration struct {
    Version int
    Apply   func(Store) error
}
```

`store_meta:schema_version` is bumped after each apply. SQLite migrations preserve the
portable logical payload while adding indexed collection metadata and normalized tables.

### 4.4 Concurrency

- JSON reads and writes are protected by the file store lock.
- JSON writes serialize the in-memory map to a temporary file, fsync it, atomically rename it
  over `sis.db.json`, and fsync the parent directory.
- SQLite operations use database transactions where multi-row consistency matters.
- Long-running scans use snapshot copies so callers do not mutate shared store state.

The store package remains interface-driven so DNS, API, policy, stats, and WebUI callers do
not depend on backend details. Backup/restore uses a portable logical JSON snapshot for both
backends, and store verification reports backend, path, schema version, record counts, and
SQLite `PRAGMA quick_check` when applicable.

---

## 5. DNS Subsystem (`internal/dns`)

### 5.1 Listeners

```go
type Server struct {
    cfg      *config.Holder
    pipeline *Pipeline
    udpConns []*net.UDPConn
    tcpLns   []*net.TCPListener
    workers  *workerPool
    log      *log.Query
    stats    *stats.Counters
}

func (s *Server) Start(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
```

**UDP loop:**

```go
for {
    n, addr, err := conn.ReadFromUDP(buf)
    // hand off to worker
    s.workers.Submit(func() {
        s.serveUDP(addr, buf[:n])
    })
}
```

Workers own response buffers from a `sync.Pool` to avoid allocation per query.

**TCP loop:** standard `net.TCPListener.Accept` → dedicated goroutine per connection (DNS over TCP is rare and connections are short).

### 5.2 Pipeline

```go
type Pipeline struct {
    cache    *Cache
    policy   *policy.Engine
    upstream *upstream.Pool
    log      *log.Query
    stats    *stats.Counters
    clientID *ClientID
}

type Request struct {
    Msg       *dns.Msg     // miekg/dns Msg
    SrcIP     net.IP
    Proto     string       // "udp" | "tcp"
    StartedAt time.Time
}

type Response struct {
    Msg     *dns.Msg
    Source  string         // "cache" | "upstream:<id>" | "synthetic" | "local"
    Latency time.Duration
}

func (p *Pipeline) Handle(ctx context.Context, r *Request) *Response
```

Pipeline steps are **inlined**, not pluggable, in v1. Plugin interface is reserved for v2 to keep hot-path allocations to zero.

### 5.3 Pseudocode for `Handle`

```go
func (p *Pipeline) Handle(ctx context.Context, r *Request) *Response {
    q := r.Msg.Question[0]
    qname := canonicalize(q.Name)

    // 1. Special names
    if resp, ok := p.handleSpecial(r, qname, q.Qtype); ok {
        p.logAndStat(r, resp, /*blocked*/false, "", "")
        return resp
    }

    // 2. Identify client
    cid := p.clientID.Resolve(r.SrcIP)

    // 3. Resolve group + policy snapshot
    pol := p.policy.For(cid)

    // 4. Cache lookup (only for non-blocked path; we still need to evaluate policy for blocks)
    cacheKey := makeKey(qname, q.Qtype, q.Qclass)

    // 5. Evaluate policy
    decision := pol.Evaluate(qname, q.Qtype, time.Now())
    if decision.Blocked {
        resp := p.synthesize(r.Msg, q, decision)
        p.logAndStat(r, resp, true, decision.Reason, decision.List)
        return resp
    }

    // 6. Cache check
    if cached, ok := p.cache.Get(cacheKey); ok {
        resp := buildResponseFromCache(r.Msg, cached)
        p.logAndStat(r, resp, false, "", "")
        return resp
    }

    // 7. Upstream
    out := stripECS(r.Msg)
    ans, srcID, err := p.upstream.Forward(ctx, out)
    if err != nil {
        resp := servfail(r.Msg)
        p.logAndStat(r, resp, false, "upstream-error", "")
        return resp
    }
    p.cache.Put(cacheKey, ans)
    p.logAndStat(r, &Response{Msg: ans, Source: "upstream:"+srcID}, false, "", "")
    return &Response{Msg: ans, Source: "upstream:"+srcID, Latency: time.Since(r.StartedAt)}
}
```

### 5.4 Cache

LRU with TTL. Two structures:

```go
type Cache struct {
    mu      sync.RWMutex
    items   map[key]*entry
    lruHead *entry
    lruTail *entry
    cap     int
    minTTL  time.Duration
    maxTTL  time.Duration
    negTTL  time.Duration
}

type entry struct {
    key      key
    msg      []byte        // wire-format response, prebuilt
    expires  time.Time
    prev,next *entry        // intrusive LRU
}
```

- `Get` checks expiration; expired entries are evicted on hit.
- `Put` clamps TTL into `[minTTL, maxTTL]`; for NXDOMAIN/NODATA uses `negTTL`.
- LRU operations are O(1) under a single `RWMutex`. No sharding in v1; revisit if profiling shows contention.
- Wire-format storage avoids re-marshalling on every hit; we rewrite the question section and ID at serve time.

### 5.5 Client Identity (`client_id.go`)

```go
type ClientID struct {
    arp *ARPTable
}

type Identity struct {
    Key  string  // mac if available, else ip
    Type string  // "mac" | "ip"
    IP   net.IP
}

func (c *ClientID) Resolve(ip net.IP) Identity {
    if mac, ok := c.arp.Lookup(ip); ok {
        return Identity{Key: mac, Type: "mac", IP: ip}
    }
    return Identity{Key: ip.String(), Type: "ip", IP: ip}
}
```

#### ARP Table

```go
type ARPTable struct {
    mu      sync.RWMutex
    entries map[string]string // ip → mac
    refresh time.Duration
}

// Refresh loop:
// - Linux: parse /proc/net/arp; parse `ip -6 neigh` output.
// - macOS/BSD: invoke `arp -an` and `ndp -an`, parse text output.
// - Windows v1: not supported; ARPTable returns no MAC entries (clients identified by IP).
```

The Linux parser reads `/proc/net/arp` directly (no shelling out). For IPv6 neighbors, v1 reads `/proc/net/route` plus `/sys/class/net/*/neigh/*` where available; otherwise the IPv6 path falls back to IP-based identity.

#### Auto-Registration Hook

After `Resolve`, the pipeline calls `p.clients.Touch(identity)` which:

- Creates the record if absent (`first_seen = last_seen = now`, `group = "default"`, `name = ""`).
- Updates `last_seen` and `last_ip` if present.
- Throttled: `Touch` is at most once per minute per key (debounced via in-memory map).

### 5.6 Special Name Handling

```go
func (p *Pipeline) handleSpecial(r *Request, qname string, qtype uint16) (*Response, bool) {
    switch {
    case qname == "localhost." || strings.HasSuffix(qname, ".localhost."):
        return synthLoopback(r.Msg, qtype), true
    case qname == "use-application-dns.net.":
        return synthNXDOMAIN(r.Msg), true
    case qtype == dns.TypePTR && isPrivatePTR(qname) && !p.policy.HasLocalZoneFor(qname):
        if p.cfg.Get().Privacy.BlockLocalPTR {
            return synthNXDOMAIN(r.Msg), true
        }
    }
    return nil, false
}
```

`isPrivatePTR` checks RFC 1918, link-local, and ULA reverse-zones (`*.in-addr.arpa.`, `*.ip6.arpa.`).

---

## 6. Policy (`internal/policy`)

### 6.1 Engine

```go
type Engine struct {
    mu        sync.RWMutex
    groups    map[string]*Group
    lists     map[string]*Domains  // blocklist id → domain set
    custom    *Domains             // custom block
    allowlist *Domains             // global allow
    customAllow *Domains
    clients   ClientResolver       // pulls client → group from store
    tz        *time.Location
}

type Decision struct {
    Blocked bool
    Reason  string  // "blocklist:ads" | "schedule:bedtime" | ""
    List    string  // logical list id that matched
}
```

```go
// For returns a snapshot policy bound to a specific client identity.
func (e *Engine) For(id dns.Identity) *Policy {
    e.mu.RLock()
    defer e.mu.RUnlock()
    grp := e.groups[e.clients.GroupOf(id.Key)]
    if grp == nil { grp = e.groups["default"] }
    return &Policy{group: grp, engine: e}
}
```

```go
// Evaluate returns a Decision in O(labels) for the qname.
func (p *Policy) Evaluate(qname string, qtype uint16, now time.Time) Decision {
    // 1. Global allowlist (highest priority unblock)
    if p.engine.allowlist.Match(qname) || p.engine.customAllow.Match(qname) {
        return Decision{Blocked: false}
    }
    // 2. Group allowlist
    if matchAllowlist(p.group.AllowlistTree, qname) {
        return Decision{Blocked: false}
    }
    // 3. Compute active blocklist set: base + active schedules
    active := p.group.BaseLists
    for _, s := range p.group.Schedules {
        if s.ActiveAt(now, p.engine.tz) {
            active = append(active, s.Block...)
        }
    }
    // 4. Match
    for _, listID := range active {
        if dom := p.engine.lists[listID]; dom != nil && dom.Match(qname) {
            reason := "blocklist:" + listID
            if isFromSchedule(p.group, listID, now) {
                reason = "schedule:" + scheduleNameFor(p.group, listID, now)
            }
            return Decision{Blocked: true, Reason: reason, List: listID}
        }
    }
    // 5. Custom block
    if p.engine.custom.Match(qname) {
        return Decision{Blocked: true, Reason: "blocklist:custom", List: "custom"}
    }
    return Decision{Blocked: false}
}
```

### 6.2 Domain Tree

A blocklist is compiled into a **suffix tree on labels**:

```
example.com   → root[com][example] = leaf
*.example.com → root[com][example] = wildcard-leaf
```

```go
type Domains struct {
    root *node
}

type node struct {
    children map[string]*node
    leaf     bool   // exact match here
    wild     bool   // any further labels match
}

func (d *Domains) Match(qname string) bool {
    labels := splitLabels(qname) // reversed: ["com","example","www"]
    n := d.root
    for _, lbl := range labels {
        if n == nil { return false }
        if c, ok := n.children[lbl]; ok {
            if c.leaf || c.wild { /* may match here, but keep walking */ }
            n = c
            continue
        }
        // No child for this label; check parent wild
        return n.wild
    }
    return n != nil && n.leaf
}
```

Match is O(L) where L is the number of labels in `qname`, regardless of list size. Memory cost is dominated by the children maps; a 1M-domain list compiles to ~120 MB in this representation, which is acceptable for v1 targets. (A radix-trie variant is a v2 optimization.)

#### Insertion

```go
func (d *Domains) Add(domain string) {
    domain = strings.ToLower(strings.TrimSuffix(domain, "."))
    wild := strings.HasPrefix(domain, "*.")
    if wild { domain = domain[2:] }
    labels := splitLabels(domain + ".")
    n := d.root
    for _, lbl := range labels {
        if n.children == nil {
            n.children = make(map[string]*node)
        }
        c, ok := n.children[lbl]
        if !ok {
            c = &node{}
            n.children[lbl] = c
        }
        n = c
    }
    if wild { n.wild = true } else { n.leaf = true }
}
```

### 6.3 Schedule Evaluation

```go
type CompiledSchedule struct {
    Name    string
    Days    uint8        // bitmask, bit i = weekday i (Sun=0)
    Start   int          // minutes since midnight
    End     int          // minutes since midnight; if End < Start, crosses midnight
    Block   []string
}

func (s CompiledSchedule) ActiveAt(now time.Time, tz *time.Location) bool {
    t := now.In(tz)
    weekdayBit := uint8(1) << uint(t.Weekday())
    if s.Days & weekdayBit == 0 {
        // Could still be active from a window that started yesterday
        return s.crossesMidnightFromPrior(t)
    }
    minutes := t.Hour()*60 + t.Minute()
    if s.End >= s.Start {
        return minutes >= s.Start && minutes < s.End
    }
    // wraps midnight
    return minutes >= s.Start || minutes < s.End
}
```

`crossesMidnightFromPrior` handles the case where today is excluded but yesterday's window extends into today's morning hours (e.g., a Friday 22:00→07:00 schedule that should also block on Saturday 00:00–07:00).

### 6.4 Blocklist Lifecycle

```go
type Fetcher struct {
    httpClient *http.Client
    cacheDir   string  // <data_dir>/blocklists
}

func (f *Fetcher) Fetch(ctx context.Context, b config.Blocklist) (*Domains, time.Time, error) {
    cachePath := filepath.Join(f.cacheDir, b.ID + ".cache")
    meta := loadMeta(cachePath + ".meta")
    req := buildHTTPReq(b.URL, meta) // sets If-Modified-Since, ETag
    resp, err := f.httpClient.Do(req)
    if err != nil { return nil, meta.LastFetched, err }
    defer resp.Body.Close()

    if resp.StatusCode == 304 {
        return loadCompiled(cachePath), meta.LastFetched, nil
    }
    if resp.StatusCode != 200 {
        return nil, meta.LastFetched, fmt.Errorf("status %d", resp.StatusCode)
    }
    domains, err := parseStream(resp.Body)
    if err != nil { return nil, meta.LastFetched, err }
    saveCompiled(cachePath, domains)
    saveMeta(cachePath+".meta", meta)
    return domains, time.Now(), nil
}
```

#### Parser

```go
func parseStream(r io.Reader) (*Domains, error) {
    d := &Domains{root: &node{}}
    sc := bufio.NewScanner(r)
    sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
    for sc.Scan() {
        line := strings.TrimSpace(sc.Text())
        if line == "" || line[0] == '#' { continue }
        // hosts format: "0.0.0.0 example.com" or "127.0.0.1 example.com"
        // domain-only:  "example.com"
        var dom string
        if i := strings.IndexAny(line, " \t"); i > 0 {
            dom = strings.TrimSpace(line[i+1:])
        } else {
            dom = line
        }
        if cut := strings.Index(dom, "#"); cut >= 0 {
            dom = strings.TrimSpace(dom[:cut])
        }
        if dom == "" || dom == "localhost" || dom == "0.0.0.0" { continue }
        if !looksLikeDomain(dom) { continue }
        d.Add(dom)
    }
    return d, sc.Err()
}
```

#### Atomic Swap

After parsing, the new `*Domains` is written into `Engine.lists[id]` under the engine's write lock. In-flight queries holding a read lock complete with the old set; new queries see the new set. No queries are dropped.

---

## 7. Upstream (`internal/upstream`)

### 7.1 Pool

```go
type Pool struct {
    mu        sync.RWMutex
    upstreams []*Upstream
    health    map[string]*HealthState
    log       *log.Audit
}

type Upstream struct {
    ID         string
    URL        string
    Bootstrap  []net.IP
    Timeout    time.Duration
    httpClient *http.Client
}

type HealthState struct {
    Healthy        bool
    LastErrorAt    time.Time
    ConsecutiveErr int
    UnhealthyUntil time.Time
}
```

### 7.2 Forward

```go
func (p *Pool) Forward(ctx context.Context, q *dns.Msg) (*dns.Msg, string, error) {
    p.mu.RLock()
    list := p.upstreams
    p.mu.RUnlock()

    var firstErr error
    for _, u := range list {
        if !p.isHealthy(u.ID) { continue }
        resp, err := u.send(ctx, q)
        if err == nil {
            p.markSuccess(u.ID)
            return resp, u.ID, nil
        }
        p.markFailure(u.ID, err)
        if firstErr == nil { firstErr = err }
    }
    if firstErr == nil {
        return nil, "", errors.New("no healthy upstreams")
    }
    return nil, "", firstErr
}
```

### 7.3 DoH Send

```go
func (u *Upstream) send(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
    wire, err := q.Pack()
    if err != nil { return nil, err }
    ctx, cancel := context.WithTimeout(ctx, u.Timeout)
    defer cancel()
    req, _ := http.NewRequestWithContext(ctx, "POST", u.URL, bytes.NewReader(wire))
    req.Header.Set("Content-Type", "application/dns-message")
    req.Header.Set("Accept", "application/dns-message")
    resp, err := u.httpClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("doh status %d", resp.StatusCode)
    }
    body, err := io.ReadAll(io.LimitReader(resp.Body, 65535))
    if err != nil { return nil, err }
    out := new(dns.Msg)
    if err := out.Unpack(body); err != nil { return nil, err }
    out.Id = q.Id
    return out, nil
}
```

### 7.4 Bootstrap Dialer

```go
func newHTTPClient(u config.Upstream) *http.Client {
    bootstrap := u.Bootstrap
    dialer := &net.Dialer{Timeout: 3 * time.Second}
    transport := &http.Transport{
        ForceAttemptHTTP2:   true,
        MaxIdleConnsPerHost: 4,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 5 * time.Second,
        TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
            host, port, _ := net.SplitHostPort(addr)
            // Replace hostname with bootstrap IP, preserve SNI via TLSClientConfig.ServerName
            for _, ip := range bootstrap {
                conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
                if err == nil {
                    transport.TLSClientConfig.ServerName = host
                    return conn, nil
                }
            }
            return nil, fmt.Errorf("all bootstrap IPs failed for %s", host)
        },
    }
    return &http.Client{Transport: transport, Timeout: u.Timeout}
}
```

(Note: setting `TLSClientConfig.ServerName` after dial is racy across connections; the production version clones the transport per-host so SNI is correct. This snippet is illustrative.)

### 7.5 Health Prober

A goroutine ticks every 60 seconds:

```go
for _, u := range pool.upstreams {
    if pool.isUnhealthy(u.ID) && time.Now().After(state.UnhealthyUntil) {
        if err := probe(u); err == nil { pool.markHealthy(u.ID) }
    }
}
```

Probe sends `A example.com` (or a configurable probe domain) and accepts any NOERROR/NXDOMAIN as healthy.

### 7.6 Metrics Recorded

- `upstream.request{id,result=ok|err}`
- `upstream.latency{id}` (histogram, recorded only on success)
- `upstream.consecutive_errors{id}` (gauge)

---

## 8. Logging (`internal/log`)

### 8.1 Query Log Writer

```go
type Query struct {
    mu      sync.Mutex
    f       *os.File
    rotator *Rotator
    enc     *json.Encoder
    enabled bool
    mode    string // full | hashed | anonymous
    salt    []byte
}

func (q *Query) Write(e *Entry) {
    if !q.enabled { return }
    q.applyPrivacy(e)
    q.mu.Lock()
    defer q.mu.Unlock()
    q.enc.Encode(e) // adds trailing newline
}

func (q *Query) applyPrivacy(e *Entry) {
    switch q.mode {
    case "hashed":
        e.ClientKey = hashKey(q.salt, e.ClientKey)
        e.ClientIP = ""
    case "anonymous":
        e.ClientKey = ""
        e.ClientName = ""
        e.ClientIP = ""
    }
}
```

The mutex protects file writes only; the JSON encode is allocation-free for fixed-shape entries (sis uses a hand-coded marshal for `Entry` to avoid `json.Encoder` reflection on the hot path).

### 8.2 Rotation

```go
type Rotator struct {
    path        string
    maxBytes    int64
    retentDays  int
    gzip        bool
    cur         *os.File
    curSize     int64
}

func (r *Rotator) Write(p []byte) (int, error) {
    if r.curSize+int64(len(p)) > r.maxBytes {
        r.rotate()
    }
    n, err := r.cur.Write(p)
    r.curSize += int64(n)
    return n, err
}

func (r *Rotator) rotate() {
    r.cur.Close()
    ts := time.Now().UTC().Format("20060102-150405")
    rotated := r.path + "." + ts
    os.Rename(r.path, rotated)
    if r.gzip { go gzipFile(rotated) }
    r.cur, _ = os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
    r.curSize = 0
    go r.evictOld()
}
```

Eviction lists `<dir>` and removes files older than `retentDays`.

### 8.3 SSE Tail (used by API)

The query log writer also publishes each entry to a single in-memory ring buffer + `chan` fanout for live tail. SSE handlers subscribe to the fanout; each subscriber has a bounded buffer (drops oldest on overflow). The fanout has zero overhead when there are no subscribers.

---

## 9. Stats (`internal/stats`)

### 9.1 In-Memory Counters

```go
type Counters struct {
    QueryTotal       atomic.Uint64
    CacheHit         atomic.Uint64
    BlockedTotal     atomic.Uint64
    UpstreamReq      sync.Map // upstream id → *atomic.Uint64
    UpstreamErr      sync.Map
    Latency          *Histogram

    perClient        sync.Map // key → *ClientCounter
    perDomain        *TopK    // top-N domains
    perBlockedDomain *TopK
}
```

### 9.2 Top-K

A bounded min-heap with size N (default 200), refreshed every minute. Domains below the heap floor are tracked in a count-min sketch and only promoted into the heap when their estimate exceeds the floor. This keeps memory bounded regardless of domain cardinality.

### 9.3 Aggregator

A 1-minute ticker flushes counters into `store.Stats()`:

```
stats:1m:<unix-minute> = { queries, blocked, cache_hits, ... }
stats:1m:<unix-minute>:client:<key> = { queries, blocked }
stats:1m:<unix-minute>:domain:<dom> = count
```

Hourly compaction (every hour) folds the previous hour's `1m` rows into a single `1h` row and deletes the originals. Daily compaction does the same `1h` → `1d`.

Retention windows:

| Bucket | Kept   |
|--------|--------|
| 1m     | 24h    |
| 1h     | 30d    |
| 1d     | 365d   |

### 9.4 API Surface

```go
func (c *Counters) Summary(since time.Duration) Summary
func (c *Counters) TimeSeries(metric string, range_ time.Duration, bucket time.Duration) []Point
func (c *Counters) TopClients(range_ time.Duration, n int) []ClientStat
func (c *Counters) TopDomains(range_ time.Duration, n int, blocked bool) []DomainStat
```

---

## 10. API (`internal/api`)

### 10.1 Server Construction

```go
type Server struct {
    cfg      *config.Holder
    store    store.Store
    policy   *policy.Engine
    upstream *upstream.Pool
    cache    *dns.Cache
    stats    *stats.Counters
    log      *log.Query
    audit    *log.Audit
    auth     *AuthService
    mux      *http.ServeMux
}

func New(deps Deps) *Server
func (s *Server) Handler() http.Handler  // returns the root handler with middleware applied
```

### 10.2 Routing

Stdlib `http.ServeMux` (Go 1.22+ supports method+pattern routing). Each handler registered as `mux.HandleFunc("GET /api/v1/clients", s.handleListClients)`.

Middleware composed manually (no framework):

```go
func chain(h http.Handler, mws ...Middleware) http.Handler {
    for i := len(mws) - 1; i >= 0; i-- {
        h = mws[i](h)
    }
    return h
}

handler := chain(
    s.mux,
    Recover,
    RequestID,
    AccessLog,
    SecurityHeaders,
    RateLimit(s.cfg),
    AuthRequired(s.auth, /*exempt*/ []string{"/api/v1/auth/login", "/healthz", "/readyz", "/api/v1/system/info"}),
)
```

### 10.3 Auth

```go
type AuthService struct {
    store store.SessionStore
    users UserStore
}

func (a *AuthService) Login(user, pass string) (*Session, error) {
    u, err := a.users.Get(user)
    if err != nil { return nil, ErrInvalidCredentials }
    if !verifyPBKDF2SHA256(u.Hash, pass) {
        return nil, ErrInvalidCredentials
    }
    s := &Session{
        Token:     randomToken(32),
        Username:  user,
        IssuedAt:  time.Now(),
        ExpiresAt: time.Now().Add(a.ttl),
    }
    a.store.Put(s)
    return s, nil
}
```

Session cookie:

```
Set-Cookie: sis_session=<token>; HttpOnly; SameSite=Lax; Path=/; Max-Age=86400
            Secure         # when TLS is enabled or auth.secure_cookie is set
```

Middleware reads cookie, looks up session, attaches `User` to request context. Sliding expiration: each authenticated request bumps `ExpiresAt`.
Password hashes use the documented pre-v1 PBKDF2-SHA256 compatibility contract; any future
algorithm change requires an explicit migration path.

### 10.4 First-Run Wizard

When `auth.users` is empty and `auth.first_run: true`:

- All `/api/*` endpoints except `/api/v1/auth/setup` return 412 Precondition Required.
- WebUI redirects to `/setup`, presents a one-time "create admin user" form.
- `POST /api/v1/auth/setup` creates the user, writes config back to disk, flips `first_run: false`, then performs an automatic login.

### 10.5 SSE for Live Logs

```go
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok { http.Error(w, "no streaming", 500); return }
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("X-Accel-Buffering", "no")

    sub := s.log.Subscribe(64)
    defer s.log.Unsubscribe(sub)
    for {
        select {
        case <-r.Context().Done(): return
        case e := <-sub:
            fmt.Fprintf(w, "data: %s\n\n", mustJSON(e))
            flusher.Flush()
        }
    }
}
```

### 10.6 Error Format

```json
{
  "error": {
    "code": "validation_failed",
    "message": "human-readable",
    "details": [
      {"field": "groups[1].schedules[0].from", "issue": "invalid time"}
    ]
  }
}
```

Codes are stable strings, not HTTP statuses. The handler always pairs a code with the appropriate status.

### 10.7 Rate Limit

In-memory token bucket per client IP:

- `/auth/login`: 5/min, burst 5.
- Other endpoints: 600/min, burst 100.

Buckets evicted after 5 min of inactivity. Total bucket count capped at 10k; oldest evicted when full.

---

## 11. WebUI Embedding (`internal/webui`)

```go
//go:embed all:dist
var assets embed.FS

func Handler() http.Handler {
    sub, _ := fs.Sub(assets, "dist")
    fileServer := http.FileServer(http.FS(sub))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // SPA fallback: if path has no extension and isn't an asset, serve index.html
        if shouldFallbackToIndex(r.URL.Path) {
            r2 := r.Clone(r.Context())
            r2.URL.Path = "/"
            fileServer.ServeHTTP(w, r2)
            return
        }
        // Set strong caching for /assets/*, no-cache for index.html
        applyCaching(w, r.URL.Path)
        fileServer.ServeHTTP(w, r)
    })
}
```

Assets are pre-compressed with gzip and brotli at build time; the handler serves the encoded variant when the client supports it. The `dist/` directory is committed only as a build artifact for releases; in development the API server can be configured to proxy `vite dev` instead.

---

## 12. TUI

The local TUI and Unix-socket JSON-RPC control plane are not in the current v1 release
scope. The supported management surfaces are the embedded WebUI and the HTTP-backed CLI.
If a TUI is reintroduced later, it should be treated as a v2 feature and specified against
the same authenticated API semantics instead of a second privileged control plane.

---

## 13. CLI (`cmd/sis`)

### 13.1 Dispatch

A tree of commands implemented with stdlib `flag`:

```go
type Command struct {
    Name     string
    Short    string
    Run      func(ctx context.Context, args []string) int
    Sub      []*Command
    Flags    func(*flag.FlagSet)
}

func Dispatch(root *Command, argv []string) int { ... }
```

No third-party CLI library. Help text generated from `Short` + flag help.

### 13.2 Local-Only vs Remote

CLI commands fall into three categories:

- **Local config commands** (`config validate`, `config show`): read the config file directly. No running server needed.
- **Local maintenance commands** (`backup`, `store migrate-json-to-sqlite`,
  `store export-sqlite-json`, `store compact`, `store verify`): operate on configured
  files and the data directory while following runbook guidance.
- **Live commands** (`client`, `group`, `cache`, `query`, `logs`, `stats`, `upstream`,
  `system`): call the authenticated HTTP API with an operator-provided session cookie.

If `sis serve` is not running and a live command is invoked, the CLI reports the failed
HTTP request and points operators toward `sis auth login` or local maintenance commands
where applicable.

### 13.3 Output Formats

`--json` flag forces machine-readable output. Default is a human table for `list` commands and key=value pairs for single-record commands.

---

## 14. Testing Strategy

### 14.1 Unit Tests

- `internal/policy`: domain tree match correctness, schedule active-window edge cases (midnight cross, weekday filters, DST transitions), allowlist precedence.
- `internal/dns`: cache TTL clamping, eviction, special-name handling, ECS strip, synthetic responses.
- `internal/upstream`: failover order, health state machine, timeout handling.
- `internal/config`: validator catches every documented misconfiguration.
- `internal/log`: rotation triggers, retention deletes, privacy mode transformations.

### 14.2 Integration Tests

`tests/integration/`:

- Spin up Sis with a fake upstream HTTP server; send DNS queries via `miekg/dns` client; assert responses, logs, stats.
- Test full pipeline including blocklist sync (using a static file URL), per-client policy, schedule activation, hot reload.

### 14.3 Conformance Tests

A small DNS conformance suite that validates Sis against:

- Standard query types (A/AAAA/CNAME/MX/TXT/NS/PTR/SRV/CAA).
- Truncation behavior (UDP > 1232 bytes, TC bit, TCP retry).
- Edge cases: empty question section, multiple questions (REFUSED), unknown opcodes.

### 14.4 Fuzz Targets

- `policy.Domains.Match` — random domain inputs, no panics.
- DNS message parser is `miekg/dns`; we trust their fuzz coverage.
- Hosts/blocklist parser fuzz: mangled inputs must never panic, must terminate.

### 14.5 Benchmarks

`benchmarks/`:

- `BenchmarkPipelineCacheHit` — target ≥ 200k ops/sec.
- `BenchmarkDomainMatch` — target ≥ 1M ops/sec on a 1M-entry list.
- `BenchmarkBlocklistParse1M` — target ≤ 5s.
- `BenchmarkDoHRoundTrip` — sanity (network-dependent).

---

## 15. Build & Release

### 15.1 Build

```
make build       # go build -ldflags ... -o sis ./cmd/sis
make webui       # cd webui && pnpm install && pnpm build → copies dist into internal/webui/
make all         # webui then build
make release     # cross-compile for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
```

`-ldflags` injects `pkg/version.{Version,Commit,Date}`.

### 15.2 Reproducible Builds

- `CGO_ENABLED=0`
- `-trimpath`
- Go module proxy pinned via `GOFLAGS=-mod=readonly`
- Frontend lockfile committed; build is deterministic if Node and pnpm versions match.

### 15.3 Distribution

- GitHub Releases with checksums and optional signed `SHA256SUMS`.
- Static binaries plus Linux install/upgrade/backup/verification helper scripts.
- A hardened `systemd` unit and example config/env shipped under `examples/`.

---

## 16. Observability Hooks

For development and field debugging:

- `SIGUSR2` writes goroutine and heap profiles under `<data_dir>/dbg/`.
- Authenticated HTTP pprof endpoints are exposed under `/api/v1/system/pprof/*`.
- `scripts/collect-linux-diagnostics.sh` collects a support bundle without config,
  database, backup contents, or journal logs unless explicitly enabled.
- Store verification is available locally and through the authenticated API/WebUI.

---

## 17. Known Trade-offs in v1

| Choice                                    | Reason                                       | When to revisit (v2) |
|-------------------------------------------|----------------------------------------------|----------------------|
| In-memory cache, no shards                | Simpler; profiling will show if needed       | If RWMutex contention shows up |
| Sequential upstream failover only         | Simpler than weighted/latency strategies     | When load patterns demand it |
| ARP-only client identity                  | DHCP integration adds OS surface             | When DHCP server feature lands |
| No DNSSEC validation                      | Upstream resolvers already validate           | Air-gap or compliance use cases |
| No DoT/DoH ingress                        | Home LAN clients almost all speak Plain      | When Sis goes past LAN |
| Hand-rolled HTTP middleware               | Avoids router framework dependency           | If routes explode in count |
| Local JSON or SQLite store                | Simple deployment without external DB        | External DB only if multi-node scope appears |
| YAML config (mutable by API)              | Friction-free editing for power users        | If schema grows complex enough to warrant a different format |

---

*End of IMPLEMENTATION.*
