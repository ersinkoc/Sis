package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var domainPattern = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.?$`)

// Validate checks c for schema consistency and cross-reference errors.
func Validate(c *Config) error {
	if c == nil {
		return errors.New("config: nil")
	}
	var errs []error
	errf := func(path, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...)))
	}

	blocklistIDs := map[string]struct{}{}
	for i, b := range c.Blocklists {
		if b.ID == "" {
			errf(fmt.Sprintf("blocklists[%d].id", i), "required")
		} else if _, exists := blocklistIDs[b.ID]; exists {
			errf(fmt.Sprintf("blocklists[%d].id", i), "duplicate %q", b.ID)
		} else {
			blocklistIDs[b.ID] = struct{}{}
		}
		if b.Enabled && b.URL == "" {
			errf(fmt.Sprintf("blocklists[%d].url", i), "required when enabled is true")
		}
		if b.URL != "" {
			if u, err := url.Parse(b.URL); err != nil || u.Scheme == "" {
				errf(fmt.Sprintf("blocklists[%d].url", i), "must be a valid URL")
			} else if !oneOf(u.Scheme, "http", "https", "file") {
				errf(fmt.Sprintf("blocklists[%d].url", i), "must use http, https, or file scheme")
			}
		}
		if b.RefreshInterval.Duration < 0 {
			errf(fmt.Sprintf("blocklists[%d].refresh_interval", i), "must be >= 0")
		}
	}

	defaultCount := 0
	groupNames := map[string]struct{}{}
	for i, g := range c.Groups {
		path := fmt.Sprintf("groups[%d]", i)
		if g.Name == "" {
			errf(path+".name", "required")
		}
		if g.Name == "default" {
			defaultCount++
		}
		if _, exists := groupNames[g.Name]; g.Name != "" && exists {
			errf(path+".name", "duplicate %q", g.Name)
		}
		groupNames[g.Name] = struct{}{}
		for j, id := range g.Blocklists {
			if _, ok := blocklistIDs[id]; !ok {
				errf(fmt.Sprintf("%s.blocklists[%d]", path, j), "unknown blocklist %q", id)
			}
		}
		for j, d := range g.Allowlist {
			if !validDomainPattern(d) {
				errf(fmt.Sprintf("%s.allowlist[%d]", path, j), "invalid domain pattern %q", d)
			}
		}
		for j, s := range g.Schedules {
			spath := fmt.Sprintf("%s.schedules[%d]", path, j)
			validateSchedule(spath, s, blocklistIDs, errf)
		}
	}
	if defaultCount != 1 {
		errf("groups", "default group must exist exactly once")
	}
	clientKeys := map[string]struct{}{}
	for i, client := range c.Clients {
		path := fmt.Sprintf("clients[%d]", i)
		if client.Key == "" {
			errf(path+".key", "required")
		} else if _, exists := clientKeys[client.Key]; exists {
			errf(path+".key", "duplicate %q", client.Key)
		} else {
			clientKeys[client.Key] = struct{}{}
		}
		if client.Group != "" {
			if _, ok := groupNames[client.Group]; !ok {
				errf(path+".group", "unknown group %q", client.Group)
			}
		}
		if client.Type != "" && !oneOf(client.Type, "ip", "mac") {
			errf(path+".type", "must be ip or mac")
		}
		if client.Key != "" && client.Type == "ip" && net.ParseIP(client.Key) == nil {
			errf(path+".key", "must be an IP address when type is ip")
		}
		if client.Key != "" && client.Type == "mac" {
			if _, err := net.ParseMAC(client.Key); err != nil {
				errf(path+".key", "must be a MAC address when type is mac")
			}
		}
	}
	if c.Server.HTTP.TLS {
		if c.Server.HTTP.CertFile == "" {
			errf("server.http.cert_file", "required when tls is true")
		}
		if c.Server.HTTP.KeyFile == "" {
			errf("server.http.key_file", "required when tls is true")
		}
	}
	for i, addr := range c.Server.DNS.Listen {
		if _, err := net.ResolveUDPAddr("udp", addr); err != nil {
			errf(fmt.Sprintf("server.dns.listen[%d]", i), "invalid address %q", addr)
		}
	}
	if c.Server.HTTP.Listen != "" {
		if _, err := net.ResolveTCPAddr("tcp", c.Server.HTTP.Listen); err != nil {
			errf("server.http.listen", "invalid address %q", c.Server.HTTP.Listen)
		}
	}
	if c.Server.StoreBackend != "" && !oneOf(c.Server.StoreBackend, "json") {
		errf("server.store_backend", "unsupported %q; supported values: json", c.Server.StoreBackend)
	}

	for i, d := range c.Allowlist.Domains {
		if !validDomainPattern(d) {
			errf(fmt.Sprintf("allowlist.domains[%d]", i), "invalid domain pattern %q", d)
		}
	}
	if len(c.Upstreams) == 0 {
		errf("upstreams", "at least one upstream is required")
	}
	upstreamIDs := map[string]struct{}{}
	for i, u := range c.Upstreams {
		path := fmt.Sprintf("upstreams[%d]", i)
		if u.ID == "" {
			errf(path+".id", "required")
		} else if _, exists := upstreamIDs[u.ID]; exists {
			errf(path+".id", "duplicate %q", u.ID)
		} else {
			upstreamIDs[u.ID] = struct{}{}
		}
		if u.URL == "" {
			errf(path+".url", "required")
		} else if parsed, err := url.Parse(u.URL); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			errf(path+".url", "must be an https URL")
		}
		if len(u.Bootstrap) == 0 {
			errf(path+".bootstrap", "at least one bootstrap IP is required")
		}
		for j, ip := range u.Bootstrap {
			if net.ParseIP(ip) == nil {
				errf(fmt.Sprintf("%s.bootstrap[%d]", path, j), "invalid IP %q", ip)
			}
		}
		if u.Timeout.Duration < 0 {
			errf(path+".timeout", "must be >= 0")
		}
	}
	if len(c.Auth.Users) == 0 && !c.Auth.FirstRun {
		errf("auth.users", "non-empty unless auth.first_run is true")
	}
	if c.Server.DNS.RateLimitQPS < 0 {
		errf("server.dns.rate_limit_qps", "must be >= 0")
	}
	if c.Server.DNS.RateLimitBurst < 0 {
		errf("server.dns.rate_limit_burst", "must be >= 0")
	}
	if c.Server.DNS.UDPWorkers < 0 {
		errf("server.dns.udp_workers", "must be >= 0")
	}
	if c.Server.DNS.TCPWorkers < 0 {
		errf("server.dns.tcp_workers", "must be >= 0")
	}
	if c.Server.DNS.UDPSize < 0 {
		errf("server.dns.udp_size", "must be >= 0")
	} else if c.Server.DNS.UDPSize > 65535 {
		errf("server.dns.udp_size", "must be <= 65535")
	}
	usernames := map[string]struct{}{}
	for i, user := range c.Auth.Users {
		path := fmt.Sprintf("auth.users[%d]", i)
		if user.Username == "" {
			errf(path+".username", "required")
		} else if _, exists := usernames[user.Username]; exists {
			errf(path+".username", "duplicate %q", user.Username)
		} else {
			usernames[user.Username] = struct{}{}
		}
		if user.PasswordHash == "" {
			errf(path+".password_hash", "required")
		}
	}
	if c.Cache.MinTTL.Duration > c.Cache.MaxTTL.Duration {
		errf("cache.min_ttl", "must be <= cache.max_ttl")
	}
	if c.Cache.MaxEntries < 0 {
		errf("cache.max_entries", "must be >= 0")
	}
	if c.Cache.MinTTL.Duration < 0 {
		errf("cache.min_ttl", "must be >= 0")
	}
	if c.Cache.MaxTTL.Duration < 0 {
		errf("cache.max_ttl", "must be >= 0")
	}
	if c.Cache.NegativeTTL.Duration < 0 {
		errf("cache.negative_ttl", "must be >= 0")
	}
	if c.Block.ResponseTTL.Duration < 0 {
		errf("block.response_ttl", "must be >= 0")
	}
	if c.Logging.RotateSizeMB < 0 {
		errf("logging.rotate_size_mb", "must be >= 0")
	}
	if c.Logging.RetentionDays < 0 {
		errf("logging.retention_days", "must be >= 0")
	}
	if c.Auth.SessionTTL.Duration < 0 {
		errf("auth.session_ttl", "must be >= 0")
	}
	if c.Auth.CookieName != "" && !validCookieName(c.Auth.CookieName) {
		errf("auth.cookie_name", "must be a valid HTTP cookie name")
	}
	if c.Block.ResponseA != "" {
		ip := net.ParseIP(c.Block.ResponseA)
		if ip == nil || ip.To4() == nil {
			errf("block.response_a", "must be an IPv4 address")
		}
	}
	if c.Block.ResponseAAAA != "" {
		ip := net.ParseIP(c.Block.ResponseAAAA)
		if ip == nil || ip.To4() != nil {
			errf("block.response_aaaa", "must be an IPv6 address")
		}
	}
	if c.Server.TZ != "" && c.Server.TZ != "Local" {
		if _, err := time.LoadLocation(c.Server.TZ); err != nil {
			errf("server.tz", "invalid IANA timezone %q", c.Server.TZ)
		}
	}
	if c.Privacy.LogMode != "" && !oneOf(c.Privacy.LogMode, "full", "hashed", "anonymous") {
		errf("privacy.log_mode", "must be full, hashed, or anonymous")
	}
	if c.Server.DataDir == "" {
		errf("server.data_dir", "required")
	} else if err := os.MkdirAll(filepath.Clean(c.Server.DataDir), 0o755); err != nil {
		errf("server.data_dir", "must exist or be creatable: %v", err)
	} else if c.Logging.QueryLog || c.Logging.AuditLog {
		logDir := filepath.Join(filepath.Clean(c.Server.DataDir), "logs")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			errf("server.data_dir", "logs directory must be creatable: %v", err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func validateSchedule(path string, s Schedule, blocklistIDs map[string]struct{}, errf func(string, string, ...any)) {
	if s.Name == "" {
		errf(path+".name", "required")
	}
	if !validClock(s.From) {
		errf(path+".from", "must be HH:MM")
	}
	if !validClock(s.To) {
		errf(path+".to", "must be HH:MM")
	}
	if len(s.Days) == 0 {
		errf(path+".days", "required")
	}
	for k, day := range s.Days {
		if !oneOf(strings.ToLower(day), "mon", "tue", "wed", "thu", "fri", "sat", "sun", "all", "weekday", "weekend") {
			errf(fmt.Sprintf("%s.days[%d]", path, k), "unknown day token %q", day)
		}
	}
	for k, id := range s.Block {
		if _, ok := blocklistIDs[id]; !ok {
			errf(fmt.Sprintf("%s.block[%d]", path, k), "unknown blocklist %q", id)
		}
	}
}

func validClock(v string) bool {
	_, err := time.Parse("15:04", v)
	return err == nil
}

func validDomainPattern(v string) bool {
	return v != "" && domainPattern.MatchString(v)
}

func validCookieName(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r < 0x21 || r > 0x7e {
			return false
		}
		switch r {
		case '(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}':
			return false
		}
	}
	return true
}

func oneOf(v string, opts ...string) bool {
	for _, opt := range opts {
		if v == opt {
			return true
		}
	}
	return false
}
