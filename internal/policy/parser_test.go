package policy

import (
	"strings"
	"testing"
)

func TestParseBlocklistHostsAndDomainOnly(t *testing.T) {
	input := `
# comment
0.0.0.0 ads.example.com
127.0.0.1 tracker.example.com # inline
malware.example.org
1.2.3.4
not a hosts line
*.wild.example.net
`
	domains, stats, err := ParseBlocklist(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range []string{"ads.example.com", "x.tracker.example.com", "malware.example.org", "sub.wild.example.net"} {
		if !domains.Match(domain) {
			t.Fatalf("expected match for %q", domain)
		}
	}
	if domains.Match("clean.example.com") {
		t.Fatal("unexpected clean domain match")
	}
	if stats.Accepted != 4 {
		t.Fatalf("accepted = %d", stats.Accepted)
	}
	if stats.Malformed != 2 {
		t.Fatalf("malformed = %d", stats.Malformed)
	}
}
