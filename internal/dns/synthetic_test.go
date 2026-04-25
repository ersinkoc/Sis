package dns

import (
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

func TestSpecialLocalhost(t *testing.T) {
	req := query("localhost.", mdns.TypeA)
	resp, ok := handleSpecial(req, "localhost.", mdns.TypeA, true)
	if !ok {
		t.Fatal("expected special response")
	}
	if resp.Rcode != mdns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestSpecialUseApplicationDNS(t *testing.T) {
	req := query("use-application-dns.net.", mdns.TypeA)
	resp, ok := handleSpecial(req, "use-application-dns.net.", mdns.TypeA, true)
	if !ok {
		t.Fatal("expected special response")
	}
	if resp.Rcode != mdns.RcodeNameError {
		t.Fatalf("rcode = %d", resp.Rcode)
	}
}

func TestSyntheticBlockA(t *testing.T) {
	req := query("blocked.example.", mdns.TypeA)
	resp := synthBlock(req, mdns.TypeA, BlockOptions{TTL: time.Minute})
	if resp.Rcode != mdns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("unexpected block response: %#v", resp)
	}
}

func TestStripECS(t *testing.T) {
	msg := query("example.com.", mdns.TypeA)
	opt := &mdns.OPT{Hdr: mdns.RR_Header{Name: ".", Rrtype: mdns.TypeOPT}}
	opt.Option = append(opt.Option, &mdns.EDNS0_SUBNET{Code: mdns.EDNS0SUBNET, Family: 1, SourceNetmask: 24})
	msg.Extra = append(msg.Extra, opt)
	out := StripECS(msg)
	outOpt := out.Extra[0].(*mdns.OPT)
	if len(outOpt.Option) != 0 {
		t.Fatalf("expected ECS stripped, got %#v", outOpt.Option)
	}
}
