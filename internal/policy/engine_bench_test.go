package policy

import (
	"fmt"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

func BenchmarkPolicyEvaluate(b *testing.B) {
	groups := []config.Group{{
		Name:       "default",
		Blocklists: []string{"ads", "trackers"},
		Schedules: []config.Schedule{{
			Name: "work-hours", Days: []string{"all"}, From: "00:00", To: "00:00", Block: []string{"social"},
		}},
	}}
	engine := benchmarkEngine(b, groups, config.Allowlist{Domains: []string{"allowed.example.com"}})
	for _, listID := range []string{"ads", "trackers", "social"} {
		domains := NewDomains()
		for i := 0; i < 5000; i++ {
			domains.Add(fmt.Sprintf("%s-%05d.example.com", listID, i))
		}
		engine.ReplaceList(listID, domains)
	}
	engine.AddCustomBlock("custom-blocked.example.com")
	policy := engine.For(Identity{Key: "bench-client"})
	queries := []string{
		"ads-00001.example.com.",
		"trackers-02500.example.com.",
		"social-04999.example.com.",
		"custom-blocked.example.com.",
		"allowed.example.com.",
		"clean.example.com.",
	}
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = policy.Evaluate(queries[i%len(queries)], 1, now)
	}
}

func BenchmarkPolicySnapshotWithManyLists(b *testing.B) {
	engine := benchmarkEngine(b, []config.Group{{Name: "default", Blocklists: []string{"list-000"}}}, config.Allowlist{})
	for i := 0; i < 1000; i++ {
		domains := NewDomains()
		domains.Add(fmt.Sprintf("blocked-%05d.example.com", i))
		engine.ReplaceList(fmt.Sprintf("list-%03d", i), domains)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.For(Identity{Key: "bench-client"})
	}
}

func benchmarkEngine(b *testing.B, groups []config.Group, allow config.Allowlist) *Engine {
	b.Helper()
	engine, err := NewEngine(&config.Config{
		Server:    config.Server{TZ: "Local"},
		Groups:    groups,
		Allowlist: allow,
	}, StaticClientResolver{})
	if err != nil {
		b.Fatal(err)
	}
	return engine
}
