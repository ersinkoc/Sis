package dns

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	"github.com/ersinkoc/sis/internal/policy"
	"github.com/ersinkoc/sis/internal/stats"
	mdns "github.com/miekg/dns"
)

func BenchmarkPipelineCacheHit(b *testing.B) {
	cache := NewCache(CacheOptions{MaxEntries: 4096, MinTTL: time.Minute, MaxTTL: time.Hour})
	pipeline := NewPipelineWithDeps(PipelineOptions{Cache: cache, Stats: stats.New()})
	req := query("cached.example.com.", mdns.TypeA)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = append(resp.Answer, &mdns.A{
		Hdr: rrHeader("cached.example.com.", mdns.TypeA, 60),
		A:   net.ParseIP("203.0.113.80").To4(),
	})
	cache.Put(cacheKey{qname: "cached.example.com.", qtype: mdns.TypeA, qclass: mdns.ClassINET}, resp)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := pipeline.Handle(context.Background(), &Request{
			Msg:       query("cached.example.com.", mdns.TypeA),
			SrcIP:     net.ParseIP("192.0.2.10"),
			Proto:     "udp",
			StartedAt: time.Now(),
		})
		if out.Source != "cache" {
			b.Fatalf("source = %q", out.Source)
		}
	}
}

func BenchmarkPipelinePolicyBlock(b *testing.B) {
	cfg := &config.Config{
		Server: config.Server{TZ: "Local"},
		Block:  config.Block{ResponseA: "0.0.0.0", ResponseAAAA: "::", ResponseTTL: config.Duration{Duration: time.Minute}},
		Groups: []config.Group{{Name: "default", Blocklists: []string{"ads"}}},
	}
	engine, err := policy.NewEngine(cfg, policy.StaticClientResolver{})
	if err != nil {
		b.Fatal(err)
	}
	ads := policy.NewDomains()
	for i := 0; i < 5000; i++ {
		ads.Add(fmt.Sprintf("ads-%05d.example.com", i))
	}
	engine.ReplaceList("ads", ads)
	pipeline := NewPipelineWithDeps(PipelineOptions{
		Config: config.NewHolder(cfg),
		Cache:  NewCache(CacheOptions{MaxEntries: 4096}),
		Policy: engine,
		Stats:  stats.New(),
	})
	queries := []string{"ads-00001.example.com.", "ads-02500.example.com.", "ads-04999.example.com."}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := pipeline.Handle(context.Background(), &Request{
			Msg:       query(queries[i%len(queries)], mdns.TypeA),
			SrcIP:     net.ParseIP("192.0.2.11"),
			Proto:     "udp",
			StartedAt: time.Now(),
		})
		if out.Msg == nil || out.Msg.Rcode != mdns.RcodeSuccess || out.Source != "synthetic" {
			b.Fatalf("unexpected response: source=%q msg=%#v", out.Source, out.Msg)
		}
	}
}
