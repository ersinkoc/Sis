package config

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration with YAML and JSON string support.
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a duration string or numeric seconds from YAML.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	var raw string
	if err := value.Decode(&raw); err == nil {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		d.Duration = parsed
		return nil
	}
	var seconds int64
	if err := value.Decode(&seconds); err == nil {
		d.Duration = time.Duration(seconds) * time.Second
		return nil
	}
	return fmt.Errorf("duration must be a string like %q", "60s")
}

// MarshalYAML writes the duration using Go duration syntax.
func (d Duration) MarshalYAML() (any, error) {
	return d.Duration.String(), nil
}

// MarshalJSON writes the duration using Go duration syntax.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

// UnmarshalJSON parses a duration string or numeric seconds from JSON.
func (d *Duration) UnmarshalJSON(raw []byte) error {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		d.Duration = parsed
		return nil
	}
	var seconds int64
	if err := json.Unmarshal(raw, &seconds); err == nil {
		d.Duration = time.Duration(seconds) * time.Second
		return nil
	}
	return fmt.Errorf("duration must be a string like %q", "60s")
}

// Config is the complete Sis runtime configuration.
type Config struct {
	Server     Server      `yaml:"server" json:"server"`
	Cache      Cache       `yaml:"cache" json:"cache"`
	Privacy    Privacy     `yaml:"privacy" json:"privacy"`
	Logging    Logging     `yaml:"logging" json:"logging"`
	Block      Block       `yaml:"block" json:"block"`
	Upstreams  []Upstream  `yaml:"upstreams" json:"upstreams"`
	Blocklists []Blocklist `yaml:"blocklists" json:"blocklists"`
	Allowlist  Allowlist   `yaml:"allowlist" json:"allowlist"`
	Groups     []Group     `yaml:"groups" json:"groups"`
	Clients    []Client    `yaml:"clients" json:"clients"`
	Auth       Auth        `yaml:"auth" json:"auth"`
}

// Server groups listener, storage, and timezone settings.
type Server struct {
	DNS          DNSServer  `yaml:"dns" json:"dns"`
	HTTP         HTTPServer `yaml:"http" json:"http"`
	DataDir      string     `yaml:"data_dir" json:"data_dir"`
	StoreBackend string     `yaml:"store_backend" json:"store_backend"`
	TZ           string     `yaml:"tz" json:"tz"`
}

// DNSServer configures classic DNS listeners and throttling.
type DNSServer struct {
	Listen         []string `yaml:"listen" json:"listen"`
	UDPWorkers     int      `yaml:"udp_workers" json:"udp_workers"`
	TCPWorkers     int      `yaml:"tcp_workers" json:"tcp_workers"`
	UDPSize        int      `yaml:"udp_size" json:"udp_size"`
	RateLimitQPS   int      `yaml:"rate_limit_qps" json:"rate_limit_qps"`
	RateLimitBurst int      `yaml:"rate_limit_burst" json:"rate_limit_burst"`
}

// HTTPServer configures the management API and WebUI listener.
type HTTPServer struct {
	Listen             string `yaml:"listen" json:"listen"`
	TLS                bool   `yaml:"tls" json:"tls"`
	CertFile           string `yaml:"cert_file" json:"cert_file"`
	KeyFile            string `yaml:"key_file" json:"key_file"`
	RateLimitPerMinute int    `yaml:"rate_limit_per_minute" json:"rate_limit_per_minute"`
}

// Cache configures DNS response caching behavior.
type Cache struct {
	MaxEntries  int      `yaml:"max_entries" json:"max_entries"`
	MinTTL      Duration `yaml:"min_ttl" json:"min_ttl"`
	MaxTTL      Duration `yaml:"max_ttl" json:"max_ttl"`
	NegativeTTL Duration `yaml:"negative_ttl" json:"negative_ttl"`
}

// Privacy configures upstream and logging privacy behavior.
type Privacy struct {
	StripECS      bool   `yaml:"strip_ecs" json:"strip_ecs"`
	BlockLocalPTR bool   `yaml:"block_local_ptr" json:"block_local_ptr"`
	LogMode       string `yaml:"log_mode" json:"log_mode"`
	LogSalt       string `yaml:"log_salt" json:"log_salt"`
}

// Logging configures query and audit log persistence.
type Logging struct {
	QueryLog      bool `yaml:"query_log" json:"query_log"`
	AuditLog      bool `yaml:"audit_log" json:"audit_log"`
	RotateSizeMB  int  `yaml:"rotate_size_mb" json:"rotate_size_mb"`
	RetentionDays int  `yaml:"retention_days" json:"retention_days"`
	Gzip          bool `yaml:"gzip" json:"gzip"`
}

// Block configures synthetic responses for blocked queries.
type Block struct {
	ResponseA    string   `yaml:"response_a" json:"response_a"`
	ResponseAAAA string   `yaml:"response_aaaa" json:"response_aaaa"`
	ResponseTTL  Duration `yaml:"response_ttl" json:"response_ttl"`
	UseNXDOMAIN  bool     `yaml:"use_nxdomain" json:"use_nxdomain"`
}

// Upstream configures one DNS-over-HTTPS resolver.
type Upstream struct {
	ID        string   `yaml:"id" json:"id"`
	Name      string   `yaml:"name" json:"name"`
	URL       string   `yaml:"url" json:"url"`
	Bootstrap []string `yaml:"bootstrap" json:"bootstrap"`
	Timeout   Duration `yaml:"timeout" json:"timeout"`
}

// Blocklist configures one managed blocklist source.
type Blocklist struct {
	ID              string   `yaml:"id" json:"id"`
	Name            string   `yaml:"name" json:"name"`
	URL             string   `yaml:"url" json:"url"`
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	RefreshInterval Duration `yaml:"refresh_interval" json:"refresh_interval"`
}

// Allowlist configures globally allowed domain suffixes.
type Allowlist struct {
	Domains []string `yaml:"domains" json:"domains"`
}

// Group configures a client policy group.
type Group struct {
	Name       string     `yaml:"name" json:"name"`
	Blocklists []string   `yaml:"blocklists" json:"blocklists"`
	Allowlist  []string   `yaml:"allowlist" json:"allowlist"`
	Schedules  []Schedule `yaml:"schedules" json:"schedules"`
}

// Schedule configures time-windowed extra blocklists for a group.
type Schedule struct {
	Name  string   `yaml:"name" json:"name"`
	Days  []string `yaml:"days" json:"days"`
	From  string   `yaml:"from" json:"from"`
	To    string   `yaml:"to" json:"to"`
	Block []string `yaml:"block" json:"block"`
}

// Client configures static metadata for a known client.
type Client struct {
	Key    string `yaml:"key" json:"key"`
	Type   string `yaml:"type" json:"type"`
	Name   string `yaml:"name" json:"name"`
	Group  string `yaml:"group" json:"group"`
	Hidden bool   `yaml:"hidden" json:"hidden"`
}

// Auth configures local users and cookie sessions.
type Auth struct {
	Users        []User   `yaml:"users" json:"users"`
	FirstRun     bool     `yaml:"first_run" json:"first_run"`
	SessionTTL   Duration `yaml:"session_ttl" json:"session_ttl"`
	CookieName   string   `yaml:"cookie_name" json:"cookie_name"`
	SecureCookie bool     `yaml:"secure_cookie" json:"secure_cookie"`
}

// User configures one local management user.
type User struct {
	Username     string `yaml:"username" json:"username"`
	PasswordHash string `yaml:"password_hash" json:"password_hash"`
}
