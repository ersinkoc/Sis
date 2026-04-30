package config

import (
	"os"
	"path/filepath"
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

func TestValidateAcceptsIDNAllowlistPatterns(t *testing.T) {
	cfg := validConfig(t)
	cfg.Allowlist.Domains = []string{"Bücher.Example.", "*.München.Example"}
	cfg.Groups[0].Allowlist = []string{"Café.Example"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("IDN allowlist patterns should validate: %v", err)
	}
}

func TestValidateRejectsUnstableIDNAllowlistPatterns(t *testing.T) {
	cfg := validConfig(t)
	cfg.Allowlist.Domains = []string{"0ℸ", "\x8f"}
	err := Validate(cfg)
	assertErrContains(t, err, "allowlist.domains[0]")
	assertErrContains(t, err, "allowlist.domains[1]")
}

func TestValidateBlocklistURLAndRefresh(t *testing.T) {
	cfg := validConfig(t)
	cfg.Blocklists[0].URL = "ftp://example.com/list.txt"
	cfg.Blocklists[0].RefreshInterval = Duration{Duration: -time.Second}
	err := Validate(cfg)
	assertErrContains(t, err, "blocklists[0].url")
	assertErrContains(t, err, "blocklists[0].refresh_interval")
}

func TestValidateEnabledBlocklistRequiresURL(t *testing.T) {
	cfg := validConfig(t)
	cfg.Blocklists[0].Enabled = true
	cfg.Blocklists[0].URL = ""
	err := Validate(cfg)
	assertErrContains(t, err, "blocklists[0].url")
}

func TestValidateUpstreamTimeout(t *testing.T) {
	cfg := validConfig(t)
	cfg.Upstreams[0].Timeout = Duration{Duration: -time.Second}
	err := Validate(cfg)
	assertErrContains(t, err, "upstreams[0].timeout")
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

func TestValidateHTTPRateLimit(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server.HTTP.RateLimitPerMinute = -1
	err := Validate(cfg)
	assertErrContains(t, err, "server.http.rate_limit_per_minute")
}

func TestValidateClients(t *testing.T) {
	cfg := validConfig(t)
	cfg.Clients = []Client{
		{Key: "192.168.1.10", Group: "missing"},
		{Key: "192.168.1.10"},
		{Group: "default"},
		{Key: "aa:bb:cc:dd:ee:ff", Type: "device"},
	}
	err := Validate(cfg)
	assertErrContains(t, err, "clients[0].group")
	assertErrContains(t, err, "clients[1].key")
	assertErrContains(t, err, "clients[2].key")
	assertErrContains(t, err, "clients[3].type")
}

func TestValidateClientKeyMatchesType(t *testing.T) {
	cfg := validConfig(t)
	cfg.Clients = []Client{
		{Key: "not-an-ip", Type: "ip"},
		{Key: "not-a-mac", Type: "mac"},
	}
	err := Validate(cfg)
	assertErrContains(t, err, "clients[0].key")
	assertErrContains(t, err, "clients[1].key")
}

func TestValidateCacheTTLOrder(t *testing.T) {
	cfg := validConfig(t)
	cfg.Cache.MinTTL = Duration{Duration: 2 * time.Hour}
	err := Validate(cfg)
	assertErrContains(t, err, "cache.min_ttl")
}

func TestValidateBlockResponseIPs(t *testing.T) {
	cfg := validConfig(t)
	cfg.Block.ResponseA = "::1"
	cfg.Block.ResponseAAAA = "192.0.2.1"
	err := Validate(cfg)
	assertErrContains(t, err, "block.response_a")
	assertErrContains(t, err, "block.response_aaaa")
}

func TestValidateListenerAddresses(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server.DNS.Listen = []string{"bad address"}
	cfg.Server.HTTP.Listen = "also bad"
	err := Validate(cfg)
	assertErrContains(t, err, "server.dns.listen[0]")
	assertErrContains(t, err, "server.http.listen")
}

func TestValidateDNSUDPSizeUpperBound(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server.DNS.UDPSize = 65536
	err := Validate(cfg)
	assertErrContains(t, err, "server.dns.udp_size")
}

func TestValidateNonNegativeRuntimeNumbers(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server.DNS.UDPWorkers = -1
	cfg.Server.DNS.TCPWorkers = -1
	cfg.Server.DNS.UDPSize = -1
	cfg.Cache.MaxEntries = -1
	cfg.Cache.MinTTL = Duration{Duration: -time.Second}
	cfg.Cache.MaxTTL = Duration{Duration: -time.Second}
	cfg.Cache.NegativeTTL = Duration{Duration: -time.Second}
	cfg.Block.ResponseTTL = Duration{Duration: -time.Second}
	cfg.Logging.RotateSizeMB = -1
	cfg.Logging.RetentionDays = -1
	cfg.Auth.SessionTTL = Duration{Duration: -time.Second}
	err := Validate(cfg)
	for _, want := range []string{
		"server.dns.udp_workers",
		"server.dns.tcp_workers",
		"server.dns.udp_size",
		"cache.max_entries",
		"cache.min_ttl",
		"cache.max_ttl",
		"cache.negative_ttl",
		"block.response_ttl",
		"logging.rotate_size_mb",
		"logging.retention_days",
		"auth.session_ttl",
	} {
		assertErrContains(t, err, want)
	}
}

func TestValidateLoggingRequiresCreatableLogDir(t *testing.T) {
	cfg := validConfig(t)
	cfg.Logging.QueryLog = true
	if err := os.WriteFile(filepath.Join(cfg.Server.DataDir, "logs"), []byte("not a directory"), 0o640); err != nil {
		t.Fatal(err)
	}
	err := Validate(cfg)
	assertErrContains(t, err, "server.data_dir")
}

func TestValidateStoreBackend(t *testing.T) {
	cfg := validConfig(t)
	cfg.Server.StoreBackend = "sqlite"
	if err := Validate(cfg); err != nil {
		t.Fatalf("sqlite backend should validate: %v", err)
	}

	cfg.Server.StoreBackend = "postgres"
	err := Validate(cfg)
	assertErrContains(t, err, "server.store_backend")
}

func TestValidateAuthCookieName(t *testing.T) {
	cfg := validConfig(t)
	cfg.Auth.CookieName = "bad name"
	err := Validate(cfg)
	assertErrContains(t, err, "auth.cookie_name")
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
