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

func TestSpecialLocalhostSubdomainNODATA(t *testing.T) {
	req := query("printer.localhost.", mdns.TypeMX)
	resp, ok := handleSpecial(req, "printer.localhost.", mdns.TypeMX, true)
	if !ok {
		t.Fatal("expected special response")
	}
	if resp.Rcode != mdns.RcodeSuccess || len(resp.Answer) != 0 || len(resp.Ns) != 1 {
		t.Fatalf("expected NODATA-style response, got %#v", resp)
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

func TestSpecialPrivatePTR(t *testing.T) {
	tests := []string{
		"42.1.168.192.in-addr.arpa.",
		"1.0.16.172.in-addr.arpa.",
		"1.0.0.10.in-addr.arpa.",
		"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.c.f.ip6.arpa.",
	}
	for _, qname := range tests {
		req := query(qname, mdns.TypePTR)
		resp, ok := handleSpecial(req, qname, mdns.TypePTR, true)
		if !ok {
			t.Fatalf("expected private PTR special handling for %q", qname)
		}
		if resp.Rcode != mdns.RcodeNameError {
			t.Fatalf("rcode for %q = %d", qname, resp.Rcode)
		}
	}
}

func TestSpecialPrivatePTRCanBeDisabled(t *testing.T) {
	qname := "42.1.168.192.in-addr.arpa."
	req := query(qname, mdns.TypePTR)
	if _, ok := handleSpecial(req, qname, mdns.TypePTR, false); ok {
		t.Fatal("private PTR handling should be disabled")
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
