package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadExample(t *testing.T) {
	cfg, err := (&Loader{Path: filepath.Join("..", "..", "examples", "sis.yaml")}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.DNS.UDPSize != 1232 {
		t.Fatalf("UDPSize = %d, want default 1232", cfg.Server.DNS.UDPSize)
	}
	if len(cfg.Upstreams) != 1 || cfg.Upstreams[0].ID != "cloudflare" {
		t.Fatalf("unexpected upstreams: %#v", cfg.Upstreams)
	}
}

func TestDefaultsKeepHTTPManagementOnLocalhost(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Server.HTTP.Listen != "127.0.0.1:8080" {
		t.Fatalf("HTTP listen default = %q", cfg.Server.HTTP.Listen)
	}
}

func TestEnvOverridePrecedence(t *testing.T) {
	t.Setenv("SIS_HTTP_LISTEN", "127.0.0.1:9090")
	t.Setenv("SIS_DNS_LISTEN", "127.0.0.1:5354,[::1]:5354")
	t.Setenv("SIS_DNS_RATE_LIMIT_QPS", "50")
	t.Setenv("SIS_DNS_RATE_LIMIT_BURST", "75")
	t.Setenv("SIS_HTTP_RATE_LIMIT_PER_MINUTE", "25")
	t.Setenv("SIS_CACHE_MAX_ENTRIES", "1234")
	t.Setenv("SIS_CACHE_MIN_TTL", "30s")
	t.Setenv("SIS_LOGGING_GZIP", "true")
	t.Setenv("SIS_AUTH_SESSION_TTL", "2h")
	t.Setenv("SIS_AUTH_SECURE_COOKIE", "true")
	t.Setenv("SIS_STORE_BACKEND", "json")
	cfg, err := (&Loader{Path: filepath.Join("..", "..", "examples", "sis.yaml")}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.HTTP.Listen != "127.0.0.1:9090" {
		t.Fatalf("HTTP listen = %q", cfg.Server.HTTP.Listen)
	}
	if got := cfg.Server.DNS.Listen; len(got) != 2 || got[0] != "127.0.0.1:5354" || got[1] != "[::1]:5354" {
		t.Fatalf("DNS listen = %#v", got)
	}
	if cfg.Server.DNS.RateLimitQPS != 50 || cfg.Server.DNS.RateLimitBurst != 75 {
		t.Fatalf("rate limit = %d/%d", cfg.Server.DNS.RateLimitQPS, cfg.Server.DNS.RateLimitBurst)
	}
	if cfg.Server.HTTP.RateLimitPerMinute != 25 {
		t.Fatalf("HTTP rate limit = %d", cfg.Server.HTTP.RateLimitPerMinute)
	}
	if cfg.Cache.MaxEntries != 1234 || cfg.Cache.MinTTL.Duration != 30*time.Second {
		t.Fatalf("cache env overrides not applied: %#v", cfg.Cache)
	}
	if !cfg.Logging.Gzip {
		t.Fatal("logging gzip override not applied")
	}
	if cfg.Auth.SessionTTL.Duration != 2*time.Hour {
		t.Fatalf("session ttl = %s", cfg.Auth.SessionTTL.Duration)
	}
	if !cfg.Auth.SecureCookie {
		t.Fatal("secure cookie override not applied")
	}
	if cfg.Server.StoreBackend != "json" {
		t.Fatalf("store backend = %q", cfg.Server.StoreBackend)
	}
}

func TestSaveCreatesParentDirAndRestrictsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "sis.yaml")
	cfg := &Config{Server: Server{TZ: "Local"}}
	if err := (&Loader{Path: path}).Save(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %o, want 640", got)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sis.yaml" {
		t.Fatalf("unexpected save artifacts: %#v", entries)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "tz: Local") {
		t.Fatalf("saved config does not contain timezone: %s", raw)
	}
}
