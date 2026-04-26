package policy

import "testing"

func TestDomainsMatchSuffix(t *testing.T) {
	d := NewDomains()
	if !d.Add("example.com") {
		t.Fatal("Add returned false")
	}
	tests := map[string]bool{
		"example.com":      true,
		"www.example.com":  true,
		"deep.example.com": true,
		"notexample.com":   false,
		"example.org":      false,
	}
	for domain, want := range tests {
		if got := d.Match(domain); got != want {
			t.Fatalf("Match(%q) = %v, want %v", domain, got, want)
		}
	}
}

func TestDomainsWildcardNormalizes(t *testing.T) {
	d := NewDomains()
	d.Add("*.Example.COM.")
	if !d.Match("a.example.com.") {
		t.Fatal("expected wildcard suffix match")
	}
}

func TestDomainsRejectsOverlongDomain(t *testing.T) {
	d := NewDomains()
	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa." +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb." +
		"ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc." +
		"ddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd." +
		"example.com"
	if d.Add(long) {
		t.Fatal("expected overlong domain to be rejected")
	}
}
