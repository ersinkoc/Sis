package api

import "testing"

func TestNormalizeDomainInputRejectsWhitespaceBeforeTrailingDot(t *testing.T) {
	normalized, ok := normalizeDomainInput("0 .")
	if ok || normalized != "" {
		t.Fatalf("normalizeDomainInput returned %q, %v; want empty, false", normalized, ok)
	}
}

func TestNormalizeDomainInputConvertsIDNToALabel(t *testing.T) {
	normalized, ok := normalizeDomainInput(" Bücher.Example. ")
	if !ok || normalized != "xn--bcher-kva.example" {
		t.Fatalf("normalizeDomainInput returned %q, %v; want A-label", normalized, ok)
	}
}

func TestNormalizeDomainInputRejectsInvalidUTF8(t *testing.T) {
	normalized, ok := normalizeDomainInput("\x8f")
	if ok || normalized != "" {
		t.Fatalf("normalizeDomainInput returned %q, %v; want empty, false", normalized, ok)
	}
}

func TestNormalizeDomainInputRejectsUnstableIDNA(t *testing.T) {
	normalized, ok := normalizeDomainInput("0ℸ")
	if ok || normalized != "" {
		t.Fatalf("normalizeDomainInput returned %q, %v; want empty, false", normalized, ok)
	}
}

func FuzzNormalizeDomainInput(f *testing.F) {
	for _, seed := range []string{
		"example.com",
		" Example.COM. ",
		"bücher.example",
		"*.example.net",
		"bad domain",
		"0 .",
		"0..",
		"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, domain string) {
		normalized, ok := normalizeDomainInput(domain)
		if !ok {
			if normalized != "" {
				t.Fatalf("invalid domain returned value %q", normalized)
			}
			return
		}
		renormalized, ok := normalizeDomainInput(normalized)
		if !ok || renormalized != normalized {
			t.Fatalf("normalization is not stable: input=%q normalized=%q renormalized=%q ok=%v", domain, normalized, renormalized, ok)
		}
	})
}
