package dns

import (
	"net"
	"sync"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

func TestCacheGetRewritesIDAndQuestion(t *testing.T) {
	cache := NewCache(CacheOptions{MaxEntries: 2, MinTTL: time.Second, MaxTTL: time.Hour})
	req := query("example.com.", mdns.TypeA)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader("example.com.", mdns.TypeA, 30), A: net.IPv4(1, 2, 3, 4)}}
	key := cacheKey{qname: "example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}
	cache.Put(key, resp)

	nextReq := query("example.com.", mdns.TypeA)
	nextReq.Id = 999
	got, ok := cache.Get(key, nextReq)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Id != 999 {
		t.Fatalf("id = %d", got.Id)
	}
	if got.Question[0].Name != "example.com." {
		t.Fatalf("question = %#v", got.Question)
	}
}

func TestCacheGetUsesCacheClockForRemainingTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	cache := NewCache(CacheOptions{MaxEntries: 2, MinTTL: time.Second, MaxTTL: time.Hour})
	cache.now = func() time.Time { return now }
	req := query("example.com.", mdns.TypeA)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader("example.com.", mdns.TypeA, 30), A: net.IPv4(1, 2, 3, 4)}}
	key := cacheKey{qname: "example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}
	cache.Put(key, resp)

	now = now.Add(10 * time.Second)
	got, ok := cache.Get(key, req)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Answer[0].Header().Ttl != 20 {
		t.Fatalf("ttl = %d, want 20", got.Answer[0].Header().Ttl)
	}
}

func TestCachePutUsesSingleTimestamp(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	cache := NewCache(CacheOptions{MaxEntries: 2, MinTTL: time.Second, MaxTTL: time.Hour})
	cache.now = func() time.Time {
		calls++
		return now.Add(time.Duration(calls-1) * time.Second)
	}
	req := query("example.com.", mdns.TypeA)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader("example.com.", mdns.TypeA, 30), A: net.IPv4(1, 2, 3, 4)}}
	key := cacheKey{qname: "example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}
	cache.Put(key, resp)
	if calls != 1 {
		t.Fatalf("now calls = %d, want 1", calls)
	}
}

func TestCacheDoesNotStoreSERVFAIL(t *testing.T) {
	cache := NewCache(CacheOptions{MaxEntries: 2})
	req := query("example.com.", mdns.TypeA)
	resp := synthServerFailure(req)
	key := cacheKey{qname: "example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}
	cache.Put(key, resp)
	if got, ok := cache.Get(key, req); ok {
		t.Fatalf("unexpected SERVFAIL cache hit: %#v", got)
	}
}

func TestCacheStoresNODATA(t *testing.T) {
	cache := NewCache(CacheOptions{MaxEntries: 2, NegativeTTL: time.Minute})
	req := query("example.com.", mdns.TypeMX)
	resp := synthNODATA(req, 60)
	key := cacheKey{qname: "example.com.", qtype: mdns.TypeMX, qclass: mdns.ClassINET}
	cache.Put(key, resp)
	if _, ok := cache.Get(key, req); !ok {
		t.Fatal("expected NODATA cache hit")
	}
}

func TestCacheStoresNXDOMAIN(t *testing.T) {
	cache := NewCache(CacheOptions{MaxEntries: 2, NegativeTTL: time.Minute})
	req := query("missing.example.com.", mdns.TypeA)
	resp := synthNXDOMAIN(req)
	key := cacheKey{qname: "missing.example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}
	cache.Put(key, resp)
	got, ok := cache.Get(key, req)
	if !ok {
		t.Fatal("expected NXDOMAIN cache hit")
	}
	if got.Rcode != mdns.RcodeNameError {
		t.Fatalf("rcode = %d, want NXDOMAIN", got.Rcode)
	}
}

func TestCacheConcurrent(t *testing.T) {
	cache := NewCache(CacheOptions{MaxEntries: 16})
	req := query("example.com.", mdns.TypeA)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader("example.com.", mdns.TypeA, 30), A: net.IPv4(1, 2, 3, 4)}}
	key := cacheKey{qname: "example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				cache.Put(key, resp)
				_, _ = cache.Get(key, req)
			}
		}()
	}
	wg.Wait()
}

func query(name string, qtype uint16) *mdns.Msg {
	msg := new(mdns.Msg)
	msg.SetQuestion(name, qtype)
	return msg
}
