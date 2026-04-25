package stats

import (
	"sync"
	"testing"
	"time"
)

func TestCountersConcurrentSnapshot(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.IncQuery()
				c.IncCacheHit()
				c.ObserveLatency(time.Millisecond)
				u := c.Upstream("cloudflare")
				u.IncRequest()
				u.ObserveLatency(2 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
	snap := c.Snapshot()
	if snap.QueryTotal != 64000 || snap.CacheHit != 64000 {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}
	if snap.Upstreams["cloudflare"].Requests != 64000 {
		t.Fatalf("unexpected upstream snapshot: %#v", snap.Upstreams)
	}
}

func TestHistogramQuantiles(t *testing.T) {
	h := NewHistogram()
	for i := 0; i < 100; i++ {
		h.Observe(time.Duration(i+1) * time.Millisecond)
	}
	snap := h.Snapshot()
	if snap.Count != 100 {
		t.Fatalf("count = %d", snap.Count)
	}
	if snap.P50 < 32*time.Millisecond || snap.P50 > 64*time.Millisecond {
		t.Fatalf("p50 = %s", snap.P50)
	}
	if snap.P95 < 64*time.Millisecond || snap.P95 > 128*time.Millisecond {
		t.Fatalf("p95 = %s", snap.P95)
	}
}

func TestTopDomainsAndClients(t *testing.T) {
	c := New()
	c.AddDomain("b.example.", false)
	c.AddDomain("a.example.", true)
	c.AddDomain("a.example.", true)
	c.AddClient("client-b")
	c.AddClient("client-a")
	c.AddClient("client-a")
	domains := c.TopDomains(1, false)
	if len(domains) != 1 || domains[0].Key != "a.example." || domains[0].Count != 2 {
		t.Fatalf("domains = %#v", domains)
	}
	blocked := c.TopDomains(1, true)
	if len(blocked) != 1 || blocked[0].Key != "a.example." || blocked[0].Count != 2 {
		t.Fatalf("blocked = %#v", blocked)
	}
	clients := c.TopClients(1)
	if len(clients) != 1 || clients[0].Key != "client-a" || clients[0].Count != 2 {
		t.Fatalf("clients = %#v", clients)
	}
}
