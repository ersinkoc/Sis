package dns

import (
	"testing"

	mdns "github.com/miekg/dns"
)

func FuzzDNSMessageEdgeCases(f *testing.F) {
	for _, seed := range [][]byte{
		{},
		{0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0},
		mustPackFuzzQuestion("example.com.", mdns.TypeA),
		mustPackFuzzQuestion("use-application-dns.net.", mdns.TypeA),
		mustPackFuzzQuestion("1.0.0.10.in-addr.arpa.", mdns.TypePTR),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		var msg mdns.Msg
		if err := msg.Unpack(raw); err != nil {
			return
		}
		if _, err := msg.Pack(); err != nil {
			t.Fatalf("repack unpacked message: %v", err)
		}
		stripped := StripECS(&msg)
		if stripped == nil {
			t.Fatal("StripECS returned nil for non-nil message")
		}
		if _, err := packUDPResponse(stripped, 1232); err != nil {
			t.Fatalf("pack udp response: %v", err)
		}
		if len(msg.Question) == 0 {
			return
		}
		q := msg.Question[0]
		resp, ok := handleSpecial(&msg, canonicalName(q.Name), q.Qtype, true)
		if ok && resp == nil {
			t.Fatal("special handler returned ok with nil response")
		}
	})
}

func mustPackFuzzQuestion(name string, qtype uint16) []byte {
	msg := new(mdns.Msg)
	msg.SetQuestion(name, qtype)
	raw, err := msg.Pack()
	if err != nil {
		panic(err)
	}
	return raw
}
