package stats

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Counters struct {
	QueryTotal   atomic.Uint64
	CacheHit     atomic.Uint64
	CacheMiss    atomic.Uint64
	BlockedTotal atomic.Uint64

	upstreams sync.Map
	latency   *Histogram
	mu        sync.Mutex
	domains   map[string]uint64
	blockedDomains map[string]uint64
	clients   map[string]uint64
}

type UpstreamCounters struct {
	Requests          atomic.Uint64
	Errors            atomic.Uint64
	ConsecutiveErrors atomic.Uint64
	Healthy           atomic.Bool
	Latency           *Histogram
}

type Snapshot struct {
	QueryTotal   uint64                       `json:"query_total"`
	CacheHit     uint64                       `json:"cache_hit"`
	CacheMiss    uint64                       `json:"cache_miss"`
	BlockedTotal uint64                       `json:"blocked_total"`
	Latency      HistogramSnapshot            `json:"latency"`
	Upstreams    map[string]UpstreamSnapshot  `json:"upstreams"`
}

type UpstreamSnapshot struct {
	Requests          uint64            `json:"requests"`
	Errors            uint64            `json:"errors"`
	ConsecutiveErrors uint64            `json:"consecutive_errors"`
	Healthy           bool              `json:"healthy"`
	Latency           HistogramSnapshot `json:"latency"`
}

func New() *Counters {
	return &Counters{
		latency: NewHistogram(),
		domains: make(map[string]uint64), blockedDomains: make(map[string]uint64),
		clients: make(map[string]uint64),
	}
}

func (c *Counters) IncQuery() {
	c.QueryTotal.Add(1)
}

func (c *Counters) IncCacheHit() {
	c.CacheHit.Add(1)
}

func (c *Counters) IncCacheMiss() {
	c.CacheMiss.Add(1)
}

func (c *Counters) IncBlocked() {
	c.BlockedTotal.Add(1)
}

func (c *Counters) ObserveLatency(d time.Duration) {
	c.latency.Observe(d)
}

func (c *Counters) AddDomain(domain string, blocked bool) {
	if domain == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.domains[domain]++
	if blocked {
		c.blockedDomains[domain]++
	}
}

func (c *Counters) AddClient(client string) {
	if client == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clients[client]++
}

type TopItem struct {
	Key   string `json:"key"`
	Count uint64 `json:"count"`
}

func (c *Counters) TopDomains(n int, blocked bool) []TopItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	if blocked {
		return topN(c.blockedDomains, n)
	}
	return topN(c.domains, n)
}

func (c *Counters) TopClients(n int) []TopItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return topN(c.clients, n)
}

func (c *Counters) Upstream(id string) *UpstreamCounters {
	value, _ := c.upstreams.LoadOrStore(id, newUpstreamCounters())
	return value.(*UpstreamCounters)
}

func (c *Counters) Snapshot() Snapshot {
	out := Snapshot{
		QueryTotal: c.QueryTotal.Load(), CacheHit: c.CacheHit.Load(),
		CacheMiss: c.CacheMiss.Load(), BlockedTotal: c.BlockedTotal.Load(),
		Latency: c.latency.Snapshot(), Upstreams: make(map[string]UpstreamSnapshot),
	}
	c.upstreams.Range(func(key, value any) bool {
		id := key.(string)
		u := value.(*UpstreamCounters)
		out.Upstreams[id] = UpstreamSnapshot{
			Requests: u.Requests.Load(), Errors: u.Errors.Load(),
			ConsecutiveErrors: u.ConsecutiveErrors.Load(), Healthy: u.Healthy.Load(),
			Latency: u.Latency.Snapshot(),
		}
		return true
	})
	return out
}

func newUpstreamCounters() *UpstreamCounters {
	u := &UpstreamCounters{Latency: NewHistogram()}
	u.Healthy.Store(true)
	return u
}

func topN(values map[string]uint64, n int) []TopItem {
	if n <= 0 || n > 100 {
		n = 10
	}
	items := make([]TopItem, 0, len(values))
	for key, count := range values {
		items = append(items, TopItem{Key: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > n {
		items = items[:n]
	}
	return items
}

func (u *UpstreamCounters) IncRequest() {
	u.Requests.Add(1)
}

func (u *UpstreamCounters) IncError() {
	u.Errors.Add(1)
	u.ConsecutiveErrors.Add(1)
}

func (u *UpstreamCounters) MarkSuccess() {
	u.ConsecutiveErrors.Store(0)
	u.Healthy.Store(true)
}

func (u *UpstreamCounters) MarkUnhealthy() {
	u.Healthy.Store(false)
}

func (u *UpstreamCounters) ObserveLatency(d time.Duration) {
	u.Latency.Observe(d)
}
