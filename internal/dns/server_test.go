package dns

import (
	"net"
	"testing"

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
