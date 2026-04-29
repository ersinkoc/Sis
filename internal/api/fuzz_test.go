package api

import "testing"

func FuzzNormalizeDomainInput(f *testing.F) {
	for _, seed := range []string{
		"example.com",
		" Example.COM. ",
		"*.example.net",
		"bad domain",
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
