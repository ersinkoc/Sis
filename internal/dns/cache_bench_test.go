package dns

import (
	"fmt"
	"net"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

func BenchmarkCacheHit(b *testing.B) {
	cache := NewCache(CacheOptions{MaxEntries: 1024, MinTTL: time.Second, MaxTTL: time.Hour})
	req := query("example.com.", mdns.TypeA)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader("example.com.", mdns.TypeA, 300), A: net.IPv4(1, 2, 3, 4)}}
	key := cacheKey{qname: "example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}
	cache.Put(key, resp)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := cache.Get(key, req); !ok {
			b.Fatal("cache miss")
		}
	}
}

func BenchmarkCachePutEvict(b *testing.B) {
	cache := NewCache(CacheOptions{MaxEntries: 128, MinTTL: time.Second, MaxTTL: time.Hour})
	resp := new(mdns.Msg)
	req := query("example.com.", mdns.TypeA)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader("example.com.", mdns.TypeA, 300), A: net.IPv4(1, 2, 3, 4)}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := cacheKey{qname: fmt.Sprintf("example-%d.com.", i), qtype: mdns.TypeA, qclass: mdns.ClassINET}
		cache.Put(key, resp)
	}
}
