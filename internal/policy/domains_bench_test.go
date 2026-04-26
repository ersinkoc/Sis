package policy

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkDomainsMatch(b *testing.B) {
	domains := NewDomains()
	for i := 0; i < 10000; i++ {
		domains.Add(fmt.Sprintf("blocked-%05d.example.com", i))
	}
	queries := []string{
		"blocked-00001.example.com",
		"blocked-05000.example.com",
		"blocked-09999.example.com",
		"clean.example.com",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = domains.Match(queries[i%len(queries)])
	}
}

func BenchmarkParseBlocklist(b *testing.B) {
	var input strings.Builder
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&input, "0.0.0.0 ads-%05d.example.com\n", i)
	}
	raw := input.String()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		domains, stats, err := ParseBlocklist(strings.NewReader(raw))
		if err != nil {
			b.Fatal(err)
		}
		if domains.Len() != 10000 || stats.Accepted != 10000 {
			b.Fatalf("unexpected parse result: len=%d accepted=%d", domains.Len(), stats.Accepted)
		}
	}
}
