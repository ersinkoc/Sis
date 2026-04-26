package dns

import (
	"container/list"
	"sync"
	"time"

	mdns "github.com/miekg/dns"
)

// CacheOptions configures the DNS response cache.
type CacheOptions struct {
	MaxEntries  int
	MinTTL      time.Duration
	MaxTTL      time.Duration
	NegativeTTL time.Duration
}

// Cache is an in-memory LRU DNS response cache with TTL clamping.
type Cache struct {
	mu         sync.Mutex
	maxEntries int
	minTTL     time.Duration
	maxTTL     time.Duration
	negTTL     time.Duration
	now        func() time.Time
	items      map[cacheKey]*list.Element
	lru        *list.List
}

type cacheKey struct {
	qname  string
	qtype  uint16
	qclass uint16
}

type cacheEntry struct {
	key     cacheKey
	wire    []byte
	expires time.Time
}

// NewCache creates a cache with sane defaults for omitted options.
func NewCache(opts CacheOptions) *Cache {
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 100000
	}
	if opts.MinTTL <= 0 {
		opts.MinTTL = time.Minute
	}
	if opts.MaxTTL <= 0 {
		opts.MaxTTL = 24 * time.Hour
	}
	if opts.NegativeTTL <= 0 {
		opts.NegativeTTL = time.Hour
	}
	return &Cache{
		maxEntries: opts.MaxEntries, minTTL: opts.MinTTL, maxTTL: opts.MaxTTL, negTTL: opts.NegativeTTL,
		now: time.Now, items: make(map[cacheKey]*list.Element), lru: list.New(),
	}
}

// Get returns a cached response, rewriting the DNS ID and question for req.
func (c *Cache) Get(key cacheKey, req *mdns.Msg) (*mdns.Msg, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	elem := c.items[key]
	if elem == nil {
		return nil, false
	}
	entry := elem.Value.(*cacheEntry)
	now := c.now()
	if !entry.expires.After(now) {
		c.remove(elem)
		return nil, false
	}
	var msg mdns.Msg
	if err := msg.Unpack(entry.wire); err != nil {
		c.remove(elem)
		return nil, false
	}
	if req != nil {
		msg.Id = req.Id
		msg.Question = append([]mdns.Question(nil), req.Question...)
	}
	remaining := uint32(entry.expires.Sub(now).Seconds())
	if remaining == 0 {
		remaining = 1
	}
	setTTL(msg.Answer, remaining)
	setTTL(msg.Ns, remaining)
	setTTL(msg.Extra, remaining)
	c.lru.MoveToFront(elem)
	return &msg, true
}

// Put stores a response using the effective TTL derived from the DNS message.
func (c *Cache) Put(key cacheKey, msg *mdns.Msg) {
	if c == nil || msg == nil || c.maxEntries <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := c.ttlFor(msg)
	if ttl <= 0 {
		return
	}
	wire, err := msg.Pack()
	if err != nil {
		return
	}
	if elem := c.items[key]; elem != nil {
		entry := elem.Value.(*cacheEntry)
		entry.wire = wire
		entry.expires = c.now().Add(ttl)
		c.lru.MoveToFront(elem)
		return
	}
	entry := &cacheEntry{key: key, wire: wire, expires: c.now().Add(ttl)}
	c.items[key] = c.lru.PushFront(entry)
	for len(c.items) > c.maxEntries {
		c.remove(c.lru.Back())
	}
}

// Flush removes all cached responses.
func (c *Cache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[cacheKey]*list.Element)
	c.lru.Init()
}

// Reconfigure updates cache limits and TTL policy while preserving live entries.
func (c *Cache) Reconfigure(opts CacheOptions) {
	if c == nil {
		return
	}
	next := NewCache(opts)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxEntries = next.maxEntries
	c.minTTL = next.minTTL
	c.maxTTL = next.maxTTL
	c.negTTL = next.negTTL
	for len(c.items) > c.maxEntries {
		c.remove(c.lru.Back())
	}
}

// Len returns the current number of cached entries.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *Cache) ttlFor(msg *mdns.Msg) time.Duration {
	if msg.Rcode == mdns.RcodeNameError || (msg.Rcode == mdns.RcodeSuccess && len(msg.Answer) == 0) {
		return c.negTTL
	}
	if msg.Rcode != mdns.RcodeSuccess {
		return 0
	}
	min := uint32(0)
	for i, rr := range msg.Answer {
		ttl := rr.Header().Ttl
		if i == 0 || ttl < min {
			min = ttl
		}
	}
	ttl := time.Duration(min) * time.Second
	if ttl < c.minTTL {
		ttl = c.minTTL
	}
	if ttl > c.maxTTL {
		ttl = c.maxTTL
	}
	return ttl
}

func (c *Cache) remove(elem *list.Element) {
	if elem == nil {
		return
	}
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
	c.lru.Remove(elem)
}

func setTTL(records []mdns.RR, ttl uint32) {
	for _, rr := range records {
		if rr.Header().Rrtype == mdns.TypeOPT {
			continue
		}
		if rr.Header().Ttl > ttl {
			rr.Header().Ttl = ttl
		}
	}
}
