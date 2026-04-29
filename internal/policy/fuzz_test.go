package policy

import (
	"strings"
	"testing"
)

func FuzzParseBlocklist(f *testing.F) {
	for _, seed := range []string{
		"ads.example.com\n",
		"0.0.0.0 tracker.example.net\n",
		"127.0.0.1 localhost # comment\n",
		"bad domain\n*.wild.example\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		domains, stats, err := ParseBlocklist(strings.NewReader(raw))
		if err != nil {
			return
		}
		if domains == nil {
			t.Fatal("domains is nil")
		}
		if stats.Lines < stats.Accepted || stats.Lines < stats.Skipped || stats.Lines < stats.Malformed {
			t.Fatalf("stats out of range: %#v", stats)
		}
	})
}

func FuzzDomainsAddMatchDelete(f *testing.F) {
	for _, seed := range []string{
		"example.com",
		"*.example.net.",
		"MiXeD.Example.ORG",
		"-bad.example",
		"bad domain",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, domain string) {
		domains := NewDomains()
		added := domains.Add(domain)
		if !added {
			if domains.Match(domain) || domains.Delete(domain) {
				t.Fatalf("invalid domain should not match/delete: %q", domain)
			}
			return
		}
		if !domains.Match(domain) {
			t.Fatalf("added domain did not match: %q", domain)
		}
		if !domains.Delete(domain) {
			t.Fatalf("added domain did not delete: %q", domain)
		}
		if domains.Match(domain) {
			t.Fatalf("deleted domain still matched: %q", domain)
		}
	})
}
