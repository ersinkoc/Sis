package dns

import (
	"strings"

	mdns "github.com/miekg/dns"
)

func handleSpecial(req *mdns.Msg, qname string, qtype uint16, blockLocalPTR bool) (*mdns.Msg, bool) {
	switch {
	case qname == "localhost." || strings.HasSuffix(qname, ".localhost."):
		return synthLoopback(req, qtype), true
	case qname == "use-application-dns.net.":
		return synthNXDOMAIN(req), true
	case blockLocalPTR && qtype == mdns.TypePTR && isPrivatePTR(qname):
		return synthNXDOMAIN(req), true
	default:
		return nil, false
	}
}

func isPrivatePTR(qname string) bool {
	qname = strings.ToLower(qname)
	if strings.HasSuffix(qname, ".10.in-addr.arpa.") ||
		strings.HasSuffix(qname, ".168.192.in-addr.arpa.") ||
		strings.HasSuffix(qname, ".254.169.in-addr.arpa.") {
		return true
	}
	for i := 16; i <= 31; i++ {
		if strings.HasSuffix(qname, "."+itoa(i)+".172.in-addr.arpa.") {
			return true
		}
	}
	return strings.HasSuffix(qname, ".c.f.ip6.arpa.") ||
		strings.HasSuffix(qname, ".d.f.ip6.arpa.") ||
		strings.HasSuffix(qname, ".8.e.f.ip6.arpa.") ||
		strings.HasSuffix(qname, ".9.e.f.ip6.arpa.") ||
		strings.HasSuffix(qname, ".a.e.f.ip6.arpa.") ||
		strings.HasSuffix(qname, ".b.e.f.ip6.arpa.")
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string([]rune{rune('0' + i/10), rune('0' + i%10)})
}
