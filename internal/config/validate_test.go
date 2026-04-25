package config

import (
	"strings"
	"testing"
	"time"
)

func validConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		Server: Server{DataDir: t.TempDir(), TZ: "Local"},
		Cache: Cache{
			MinTTL: Duration{Duration: time.Minute},
			MaxTTL: Duration{Duration: time.Hour},
		},
		Privacy: Privacy{LogMode: "full"},
		Upstreams: []Upstream{{
			ID: "cloudflare", URL: "https://cloudflare-dns.com/dns-query",
			Bootstrap: []string{"1.1.1.1"},
		}},
		Blocklists: []Blocklist{{ID: "ads", URL: "file:///tmp/ads.txt"}},
		Groups:     []Group{{Name: "default", Blocklists: []string{"ads"}}},
		Auth:       Auth{FirstRun: true},
	}
}

func TestValidateRequiresDefaultGroup(t *testing.T) {
	cfg := validConfig(t)
	cfg.Groups = nil
	err := Validate(cfg)
	assertErrContains(t, err, "groups")
}

func TestValidateUnknownBlocklistPath(t *testing.T) {
	cfg := validConfig(t)
	cfg.Groups[0].Blocklists = []string{"missing"}
	err := Validate(cfg)
	assertErrContains(t, err, "groups[0].blocklists[0]")
}

func TestValidateSchedulePaths(t *testing.T) {
	cfg := validConfig(t)
	cfg.Groups[0].Schedules = []Schedule{{
		Name: "bad", Days: []string{"noday"}, From: "25:00", To: "xx", Block: []string{"missing"},
	}}
	err := Validate(cfg)
	assertErrContains(t, err, "groups[0].schedules[0].from")
	assertErrContains(t, err, "groups[0].schedules[0].to")
	assertErrContains(t, err, "groups[0].schedules[0].days[0]")
	assertErrContains(t, err, "groups[0].schedules[0].block[0]")
}

func TestValidateAuthUsersOrFirstRun(t *testing.T) {
	cfg := validConfig(t)
	cfg.Auth.FirstRun = false
	err := Validate(cfg)
	assertErrContains(t, err, "auth.users")
}

func TestValidateAuthUsers(t *testing.T) {
	cfg := validConfig(t)
	cfg.Auth.Users = []User{
		{Username: "admin", PasswordHash: "hash"},
		{Username: "admin"},
	}
	err := Validate(cfg)
	assertErrContains(t, err, "auth.users[1].username")
	assertErrContains(t, err, "auth.users[1].password_hash")
}

func TestValidateHTTPRequiresTLSFiles(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server.HTTP.TLS = true
	err := Validate(cfg)
	assertErrContains(t, err, "server.http.cert_file")
	assertErrContains(t, err, "server.http.key_file")
}

func TestValidateClients(t *testing.T) {
	cfg := validConfig(t)
	cfg.Clients = []Client{
		{Key: "192.168.1.10", Group: "missing"},
		{Key: "192.168.1.10"},
		{Group: "default"},
	}
	err := Validate(cfg)
	assertErrContains(t, err, "clients[0].group")
	assertErrContains(t, err, "clients[1].key")
	assertErrContains(t, err, "clients[2].key")
}

func TestValidateCacheTTLOrder(t *testing.T) {
	cfg := validConfig(t)
	cfg.Cache.MinTTL = Duration{Duration: 2 * time.Hour}
	err := Validate(cfg)
	assertErrContains(t, err, "cache.min_ttl")
}

func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}
