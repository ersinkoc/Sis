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

func TestDomainsNormalizeIDNToALabel(t *testing.T) {
	d := NewDomains()
	if !d.Add("Bücher.Example.") {
		t.Fatal("Add returned false")
	}
	if !d.Match("www.xn--bcher-kva.example") || !d.Match("www.bücher.example") {
		t.Fatal("expected IDN and A-label matches")
	}
	if entries := d.Entries("", 10); len(entries) != 1 || entries[0] != "xn--bcher-kva.example" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestDomainsCloneIsIndependent(t *testing.T) {
	d := NewDomains()
	d.Add("example.com")
	clone := d.Clone()
	if !clone.Match("www.example.com") {
		t.Fatal("clone should preserve existing entries")
	}
	clone.Add("blocked.example")
	clone.Delete("example.com")
	if !d.Match("www.example.com") {
		t.Fatal("clone delete should not affect original")
	}
	if d.Match("blocked.example") {
		t.Fatal("clone add should not affect original")
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

func TestNilDomainsAreSafe(t *testing.T) {
	var d *Domains
	if d.Add("example.com") {
		t.Fatal("nil domains should reject add")
	}
	if d.Match("example.com") {
		t.Fatal("nil domains should not match")
	}
	if d.Delete("example.com") {
		t.Fatal("nil domains should reject delete")
	}
	if d.Len() != 0 {
		t.Fatalf("nil domains len = %d", d.Len())
	}
	if entries := d.Entries("", 10); len(entries) != 0 {
		t.Fatalf("nil domains entries = %#v", entries)
	}
}
