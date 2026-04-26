package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	"github.com/ersinkoc/sis/internal/stats"
	"github.com/ersinkoc/sis/internal/upstream"
	mdns "github.com/miekg/dns"
)

func TestPipelineSpecial(t *testing.T) {
	p := NewPipeline(nil)
	resp := p.Handle(context.Background(), &Request{Msg: query("localhost.", mdns.TypeA), Proto: "udp"})
	if resp.Source != "local" {
		t.Fatalf("source = %q", resp.Source)
	}
	if resp.Msg.Rcode != mdns.RcodeSuccess || len(resp.Msg.Answer) != 1 {
		t.Fatalf("unexpected msg: %#v", resp.Msg)
	}
}

func TestPipelinePlaceholderCaches(t *testing.T) {
	p := NewPipeline(NewCache(CacheOptions{}))
	req := query("example.com.", mdns.TypeA)
	first := p.Handle(context.Background(), &Request{Msg: req, Proto: "udp"})
	if first.Source != "synthetic" {
		t.Fatalf("first source = %q", first.Source)
	}
	secondReq := query("example.com.", mdns.TypeA)
	second := p.Handle(context.Background(), &Request{Msg: secondReq, Proto: "udp"})
	if second.Source != "cache" {
		t.Fatalf("second source = %q", second.Source)
	}
}

func TestPipelineRejectsUnsupportedOpcode(t *testing.T) {
	p := NewPipeline(nil)
	req := query("example.com.", mdns.TypeA)
	req.Opcode = mdns.OpcodeStatus
	resp := p.Handle(context.Background(), &Request{Msg: req, Proto: "udp"})
	if resp.Msg == nil || resp.Msg.Rcode != mdns.RcodeNotImplemented {
		t.Fatalf("expected NOTIMP response, got %#v", resp.Msg)
	}
}

func TestPipelineRejectsUnsupportedClass(t *testing.T) {
	p := NewPipeline(nil)
	req := query("example.com.", mdns.TypeA)
	req.Question[0].Qclass = mdns.ClassCHAOS
	resp := p.Handle(context.Background(), &Request{Msg: req, Proto: "udp"})
	if resp.Msg == nil || resp.Msg.Rcode != mdns.RcodeRefused {
		t.Fatalf("expected REFUSED response, got %#v", resp.Msg)
	}
}

func TestPipelineRateLimitTCPRefused(t *testing.T) {
	p := NewPipelineWithDeps(PipelineOptions{Limiter: NewRateLimiter(1, 1)})
	ip := net.ParseIP("192.0.2.20")
	_ = p.Handle(context.Background(), &Request{Msg: query("example.com.", mdns.TypeA), SrcIP: ip, Proto: "tcp"})
	resp := p.Handle(context.Background(), &Request{Msg: query("example.org.", mdns.TypeA), SrcIP: ip, Proto: "tcp"})
	if resp.Msg == nil || resp.Msg.Rcode != mdns.RcodeRefused {
		t.Fatalf("expected refused response, got %#v", resp.Msg)
	}
}

func TestPipelineRateLimitUDPDrop(t *testing.T) {
	p := NewPipelineWithDeps(PipelineOptions{Limiter: NewRateLimiter(1, 1)})
	ip := net.ParseIP("192.0.2.21")
	_ = p.Handle(context.Background(), &Request{Msg: query("example.com.", mdns.TypeA), SrcIP: ip, Proto: "udp"})
	resp := p.Handle(context.Background(), &Request{Msg: query("example.org.", mdns.TypeA), SrcIP: ip, Proto: "udp"})
	if resp.Msg != nil || resp.Source != "rate-limit" {
		t.Fatalf("expected dropped UDP response, got %#v", resp)
	}
}

func TestPipelineReconfigureRateLimit(t *testing.T) {
	p := NewPipelineWithDeps(PipelineOptions{Limiter: NewRateLimiter(1, 1)})
	ip := net.ParseIP("192.0.2.22")
	_ = p.Handle(context.Background(), &Request{Msg: query("example.com.", mdns.TypeA), SrcIP: ip, Proto: "tcp"})
	limited := p.Handle(context.Background(), &Request{Msg: query("example.org.", mdns.TypeA), SrcIP: ip, Proto: "tcp"})
	if limited.Msg == nil || limited.Msg.Rcode != mdns.RcodeRefused {
		t.Fatalf("expected refused before reconfigure, got %#v", limited.Msg)
	}
	p.Reconfigure(&config.Config{})
	after := p.Handle(context.Background(), &Request{Msg: query("example.net.", mdns.TypeA), SrcIP: ip, Proto: "tcp"})
	if after.Msg == nil || after.Msg.Rcode == mdns.RcodeRefused || after.Source == "rate-limit" {
		t.Fatalf("expected limiter disabled after reconfigure, got %#v", after)
	}
}

func TestPipelineRecordsUpstreamAttempts(t *testing.T) {
	counters := stats.New()
	p := NewPipelineWithDeps(PipelineOptions{Stats: counters})
	p.recordUpstreamAttempts([]upstream.Attempt{
		{ID: "bad", OK: false, Healthy: false},
		{ID: "good", OK: true, Healthy: true},
	}, time.Now())
	snap := counters.Snapshot()
	if snap.Upstreams["bad"].Requests != 1 || snap.Upstreams["bad"].Errors != 1 || snap.Upstreams["bad"].Healthy {
		t.Fatalf("bad upstream stats = %#v", snap.Upstreams["bad"])
	}
	if snap.Upstreams["good"].Requests != 1 || snap.Upstreams["good"].Errors != 0 || !snap.Upstreams["good"].Healthy {
		t.Fatalf("good upstream stats = %#v", snap.Upstreams["good"])
	}
}
