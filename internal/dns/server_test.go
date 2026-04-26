package dns

import (
	"context"
	"net"
	"testing"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

func TestServerTCPSlots(t *testing.T) {
	s := &Server{tcpSlots: make(chan struct{}, 1)}
	if !s.acquireTCPSlot() {
		t.Fatal("first slot should be acquired")
	}
	if s.acquireTCPSlot() {
		t.Fatal("second slot should be rejected")
	}
	s.releaseTCPSlot()
	if !s.acquireTCPSlot() {
		t.Fatal("slot should be available after release")
	}
}

func TestServerStartCleansUpPartialListenersOnError(t *testing.T) {
	cfg := config.NewHolder(&config.Config{
		Server: config.Server{
			DNS: config.DNSServer{Listen: []string{"127.0.0.1:0", "bad address"}},
		},
	})
	s := NewServer(cfg, NewPipeline(nil))
	if err := s.Start(context.Background()); err == nil {
		t.Fatal("expected start error")
	}
	if len(s.udpConns) != 0 || len(s.tcpLns) != 0 || s.workers != nil || s.cancel != nil {
		t.Fatalf("partial start was not cleaned up: udp=%d tcp=%d workers=%v cancel=%v", len(s.udpConns), len(s.tcpLns), s.workers != nil, s.cancel != nil)
	}
}

func TestServerShutdownIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &Server{workers: newWorkerPool(ctx, 1, 1)}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestServerStartRequiresConfigAndPipeline(t *testing.T) {
	if err := NewServer(nil, NewPipeline(nil)).Start(context.Background()); err == nil {
		t.Fatal("expected missing config error")
	}
	holder := config.NewHolder(&config.Config{})
	if err := NewServer(holder, nil).Start(context.Background()); err == nil {
		t.Fatal("expected missing pipeline error")
	}
}

func TestPackUDPResponseTruncatesOversizedMessage(t *testing.T) {
	req := query("large.example.", mdns.TypeTXT)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	for i := 0; i < 20; i++ {
		resp.Answer = append(resp.Answer, &mdns.TXT{
			Hdr: rrHeader("large.example.", mdns.TypeTXT, 300),
			Txt: []string{"0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz"},
		})
	}

	wire, err := packUDPResponse(resp, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) > 200 {
		t.Fatalf("wire len = %d, want <= 200", len(wire))
	}
	var got mdns.Msg
	if err := got.Unpack(wire); err != nil {
		t.Fatal(err)
	}
	if !got.Truncated {
		t.Fatal("expected TC bit")
	}
	if len(got.Answer) != 0 || len(got.Ns) != 0 || len(got.Extra) != 0 {
		t.Fatalf("expected question-only truncated response, got answer=%d ns=%d extra=%d", len(got.Answer), len(got.Ns), len(got.Extra))
	}
}

func TestPackUDPResponseLeavesSmallMessageIntact(t *testing.T) {
	req := query("small.example.", mdns.TypeA)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader("small.example.", mdns.TypeA, 300), A: net.IPv4(192, 0, 2, 1)}}

	wire, err := packUDPResponse(resp, 1232)
	if err != nil {
		t.Fatal(err)
	}
	var got mdns.Msg
	if err := got.Unpack(wire); err != nil {
		t.Fatal(err)
	}
	if got.Truncated || len(got.Answer) != 1 {
		t.Fatalf("unexpected response: truncated=%v answers=%d", got.Truncated, len(got.Answer))
	}
}

func TestPackUDPResponseFallsBackToHeaderOnlyWhenQuestionDoesNotFit(t *testing.T) {
	req := query("large.example.", mdns.TypeTXT)
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Answer = []mdns.RR{&mdns.TXT{
		Hdr: rrHeader("large.example.", mdns.TypeTXT, 300),
		Txt: []string{"0123456789abcdefghijklmnopqrstuvwxyz"},
	}}

	wire, err := packUDPResponse(resp, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 12 {
		t.Fatalf("wire len = %d, want DNS header only", len(wire))
	}
	var got mdns.Msg
	if err := got.Unpack(wire); err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Question) != 0 {
		t.Fatalf("expected header-only truncated response, got truncated=%v question=%d", got.Truncated, len(got.Question))
	}
}
