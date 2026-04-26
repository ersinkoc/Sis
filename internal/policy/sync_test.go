package policy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

func TestSyncerForceSyncReplacesPolicyList(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "ads.txt")
	if err := os.WriteFile(listPath, []byte("ads.example.com\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	holder := config.NewHolder(&config.Config{
		Server: config.Server{TZ: "Local"},
		Blocklists: []config.Blocklist{{
			ID: "ads", URL: "file://" + listPath, Enabled: true,
			RefreshInterval: config.Duration{Duration: time.Hour},
		}},
		Groups: []config.Group{{Name: "default", Blocklists: []string{"ads"}}},
	})
	engine, err := NewEngine(holder.Get(), StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(holder, NewFetcher(filepath.Join(dir, "cache")), engine, nil)
	result, err := syncer.ForceSync(context.Background(), "ads")
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Accepted != 1 {
		t.Fatalf("accepted = %d", result.Stats.Accepted)
	}
	decision := engine.For(Identity{Key: "client"}).Evaluate("ads.example.com.", 1, time.Now())
	if !decision.Blocked || decision.List != "ads" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestSyncerForceSyncUnknownList(t *testing.T) {
	holder := config.NewHolder(&config.Config{
		Server: config.Server{TZ: "Local"},
		Groups: []config.Group{{Name: "default"}},
	})
	engine, err := NewEngine(holder.Get(), StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(holder, NewFetcher(t.TempDir()), engine, nil)
	if _, err := syncer.ForceSync(context.Background(), "missing"); err == nil {
		t.Fatal("expected unknown list error")
	}
}

func TestSyncerForceSyncDisabledList(t *testing.T) {
	holder := config.NewHolder(&config.Config{
		Server: config.Server{TZ: "Local"},
		Blocklists: []config.Blocklist{{ID: "ads", Enabled: false}},
		Groups: []config.Group{{Name: "default", Blocklists: []string{"ads"}}},
	})
	engine, err := NewEngine(holder.Get(), StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(holder, NewFetcher(t.TempDir()), engine, nil)
	if _, err := syncer.ForceSync(context.Background(), "ads"); err == nil {
		t.Fatal("expected disabled list error")
	}
}

func TestSyncerForceSyncRequiresDependencies(t *testing.T) {
	_, err := (*Syncer)(nil).ForceSync(context.Background(), "ads")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("nil syncer err = %v", err)
	}
	syncer := NewSyncer(config.NewHolder(&config.Config{}), nil, nil, nil)
	_, err = syncer.ForceSync(context.Background(), "ads")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("empty syncer err = %v", err)
	}
}
