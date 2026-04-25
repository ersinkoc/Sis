package policy

import (
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

func TestPolicyAllowlistWinsOverBlock(t *testing.T) {
	engine := mustEngine(t, []config.Group{{
		Name: "default", Blocklists: []string{"ads"}, Allowlist: []string{"ads.example.com"},
	}}, config.Allowlist{})
	ads := NewDomains()
	ads.Add("ads.example.com")
	engine.ReplaceList("ads", ads)

	decision := engine.For(Identity{Key: "client"}).Evaluate("ads.example.com.", 1, time.Now())
	if decision.Blocked {
		t.Fatalf("expected allowlist to win, got %#v", decision)
	}
}

func TestPolicyScheduleBlock(t *testing.T) {
	engine := mustEngine(t, []config.Group{{
		Name: "default",
		Schedules: []config.Schedule{{
			Name: "bedtime", Days: []string{"all"}, From: "22:00", To: "07:00", Block: []string{"social"},
		}},
	}}, config.Allowlist{})
	social := NewDomains()
	social.Add("tiktok.com")
	engine.ReplaceList("social", social)

	now := time.Date(2026, 4, 25, 23, 0, 0, 0, time.Local)
	decision := engine.For(Identity{Key: "client"}).Evaluate("tiktok.com.", 1, now)
	if !decision.Blocked || decision.Reason != "schedule:bedtime" || decision.List != "social" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestPolicyClientGroupFallbackAndCustomAllow(t *testing.T) {
	engine := mustEngine(t, []config.Group{
		{Name: "default", Blocklists: []string{"ads"}},
		{Name: "kids", Blocklists: []string{"social"}},
	}, config.Allowlist{})
	ads := NewDomains()
	ads.Add("ads.example.com")
	social := NewDomains()
	social.Add("game.example.com")
	engine.ReplaceList("ads", ads)
	engine.ReplaceList("social", social)
	engine.AddCustomAllow("ads.example.com")

	defaultDecision := engine.For(Identity{Key: "unknown"}).Evaluate("ads.example.com", 1, time.Now())
	if defaultDecision.Blocked {
		t.Fatalf("custom allow should unblock default group, got %#v", defaultDecision)
	}
	kids := StaticClientResolver{"kid": "kids"}
	engine.clients = kids
	kidDecision := engine.For(Identity{Key: "kid"}).Evaluate("game.example.com", 1, time.Now())
	if !kidDecision.Blocked || kidDecision.List != "social" {
		t.Fatalf("expected kids social block, got %#v", kidDecision)
	}
}

func mustEngine(t *testing.T, groups []config.Group, allow config.Allowlist) *Engine {
	t.Helper()
	engine, err := NewEngine(&config.Config{
		Server:    config.Server{TZ: "Local"},
		Groups:    groups,
		Allowlist: allow,
	}, StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}
