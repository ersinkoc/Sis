package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	"github.com/ersinkoc/sis/internal/store"
)

func TestSeedConfigClientsCreatesAndUpdates(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Clients: []config.Client{{
		Key: "192.168.1.50", Type: "ip", Name: "TV", Group: "default", Hidden: true,
	}}}
	if err := seedConfigClients(st, cfg); err != nil {
		t.Fatal(err)
	}
	client, err := st.Clients().Get("192.168.1.50")
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "TV" || client.Group != "default" || !client.Hidden {
		t.Fatalf("client = %#v", client)
	}
	firstSeen := client.FirstSeen

	client.LastSeen = time.Now().UTC().Add(time.Hour)
	if err := st.Clients().Upsert(client); err != nil {
		t.Fatal(err)
	}
	cfg.Clients[0].Name = "Living Room TV"
	cfg.Clients[0].Hidden = false
	if err := seedConfigClients(st, cfg); err != nil {
		t.Fatal(err)
	}
	client, err = st.Clients().Get("192.168.1.50")
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "Living Room TV" || client.Hidden {
		t.Fatalf("client was not updated: %#v", client)
	}
	if !client.FirstSeen.Equal(firstSeen) {
		t.Fatalf("first_seen changed: before=%s after=%s", firstSeen, client.FirstSeen)
	}
}

func TestSeedConfigClientsDefaults(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := seedConfigClients(st, &config.Config{Clients: []config.Client{{Key: "192.168.1.51"}}}); err != nil {
		t.Fatal(err)
	}
	client, err := st.Clients().Get("192.168.1.51")
	if err != nil {
		t.Fatal(err)
	}
	if client.Type != "ip" || client.Group != "default" {
		t.Fatalf("client defaults = %#v", client)
	}
}

func TestUpsertConfigUserTrimsUsernameAndRejectsWeakPassword(t *testing.T) {
	path := writeUserTestConfig(t)
	if err := upsertConfigUser(path, " admin ", "secret123", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Auth.Users) != 1 || cfg.Auth.Users[0].Username != "admin" {
		t.Fatalf("users = %#v", cfg.Auth.Users)
	}
	if err := upsertConfigUser(path, "operator", "short", false); err == nil || !strings.Contains(err.Error(), "at least 8 chars") {
		t.Fatalf("weak password err = %v", err)
	}
}

func writeUserTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sis.yaml")
	cfg := &config.Config{
		Server: config.Server{DataDir: filepath.Join(dir, "data"), TZ: "Local"},
		Cache: config.Cache{
			MinTTL: config.Duration{Duration: time.Minute},
			MaxTTL: config.Duration{Duration: time.Hour},
		},
		Privacy: config.Privacy{LogMode: "full"},
		Upstreams: []config.Upstream{{
			ID: "cloudflare", URL: "https://cloudflare-dns.com/dns-query",
			Bootstrap: []string{"1.1.1.1"},
		}},
		Groups: []config.Group{{Name: "default"}},
		Auth:   config.Auth{FirstRun: true},
	}
	if err := (&config.Loader{Path: path}).Save(cfg); err != nil {
		t.Fatal(err)
	}
	return path
}
