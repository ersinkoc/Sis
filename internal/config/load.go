package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Loader reads and writes Sis YAML configuration files.
type Loader struct {
	Path string
}

// Load reads, defaults, applies environment overrides, and validates a config file.
func (l *Loader) Load() (*Config, error) {
	raw, err := os.ReadFile(l.Path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	applyDefaults(&c)
	applyEnvOverrides(&c)
	if err := Validate(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save writes c to the loader path as YAML.
func (l *Loader) Save(c *Config) error {
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	dir := filepath.Dir(l.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(l.Path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, l.Path)
}

func applyDefaults(c *Config) {
	if len(c.Server.DNS.Listen) == 0 {
		c.Server.DNS.Listen = []string{"0.0.0.0:53", "[::]:53"}
	}
	if c.Server.DNS.UDPSize == 0 {
		c.Server.DNS.UDPSize = 1232
	}
	if c.Server.DNS.RateLimitQPS == 0 {
		c.Server.DNS.RateLimitQPS = 200
	}
	if c.Server.DNS.RateLimitBurst == 0 {
		c.Server.DNS.RateLimitBurst = 400
	}
	if c.Server.HTTP.Listen == "" {
		c.Server.HTTP.Listen = "0.0.0.0:8080"
	}
	if c.Server.DataDir == "" {
		c.Server.DataDir = "./data"
	}
	if c.Server.StoreBackend == "" {
		c.Server.StoreBackend = "json"
	}
	if c.Server.TZ == "" {
		c.Server.TZ = "Local"
	}
	if c.Cache.MaxEntries == 0 {
		c.Cache.MaxEntries = 100000
	}
	if c.Cache.MinTTL.Duration == 0 {
		c.Cache.MinTTL.Duration = time.Minute
	}
	if c.Cache.MaxTTL.Duration == 0 {
		c.Cache.MaxTTL.Duration = 24 * time.Hour
	}
	if c.Cache.NegativeTTL.Duration == 0 {
		c.Cache.NegativeTTL.Duration = time.Hour
	}
	if c.Privacy.LogMode == "" {
		c.Privacy.LogMode = "full"
	}
	if c.Logging.RotateSizeMB == 0 {
		c.Logging.RotateSizeMB = 100
	}
	if c.Logging.RetentionDays == 0 {
		c.Logging.RetentionDays = 7
	}
	if c.Block.ResponseA == "" {
		c.Block.ResponseA = "0.0.0.0"
	}
	if c.Block.ResponseAAAA == "" {
		c.Block.ResponseAAAA = "::"
	}
	if c.Block.ResponseTTL.Duration == 0 {
		c.Block.ResponseTTL.Duration = time.Minute
	}
	if c.Auth.SessionTTL.Duration == 0 {
		c.Auth.SessionTTL.Duration = 24 * time.Hour
	}
	if c.Auth.CookieName == "" {
		c.Auth.CookieName = "sis_session"
	}
}

func applyEnvOverrides(c *Config) {
	setString(os.Getenv("SIS_DATA_DIR"), &c.Server.DataDir)
	setString(os.Getenv("SIS_SERVER_DATA_DIR"), &c.Server.DataDir)
	setString(os.Getenv("SIS_STORE_BACKEND"), &c.Server.StoreBackend)
	setString(os.Getenv("SIS_SERVER_STORE_BACKEND"), &c.Server.StoreBackend)
	setString(os.Getenv("SIS_HTTP_LISTEN"), &c.Server.HTTP.Listen)
	setString(os.Getenv("SIS_SERVER_HTTP_LISTEN"), &c.Server.HTTP.Listen)
	setString(os.Getenv("SIS_HTTP_CERT_FILE"), &c.Server.HTTP.CertFile)
	setString(os.Getenv("SIS_HTTP_KEY_FILE"), &c.Server.HTTP.KeyFile)
	setString(os.Getenv("SIS_TZ"), &c.Server.TZ)
	setString(os.Getenv("SIS_PRIVACY_LOG_MODE"), &c.Privacy.LogMode)
	setString(os.Getenv("SIS_PRIVACY_LOG_SALT"), &c.Privacy.LogSalt)
	setString(os.Getenv("SIS_AUTH_COOKIE_NAME"), &c.Auth.CookieName)

	if v := os.Getenv("SIS_DNS_LISTEN"); v != "" {
		c.Server.DNS.Listen = splitCSV(v)
	}
	setInt(os.Getenv("SIS_DNS_UDP_WORKERS"), &c.Server.DNS.UDPWorkers)
	setInt(os.Getenv("SIS_DNS_TCP_WORKERS"), &c.Server.DNS.TCPWorkers)
	setInt(os.Getenv("SIS_DNS_UDP_SIZE"), &c.Server.DNS.UDPSize)
	setInt(os.Getenv("SIS_DNS_RATE_LIMIT_QPS"), &c.Server.DNS.RateLimitQPS)
	setInt(os.Getenv("SIS_DNS_RATE_LIMIT_BURST"), &c.Server.DNS.RateLimitBurst)
	setBool(os.Getenv("SIS_HTTP_TLS"), &c.Server.HTTP.TLS)
	setInt(os.Getenv("SIS_CACHE_MAX_ENTRIES"), &c.Cache.MaxEntries)
	setDuration(os.Getenv("SIS_CACHE_MIN_TTL"), &c.Cache.MinTTL)
	setDuration(os.Getenv("SIS_CACHE_MAX_TTL"), &c.Cache.MaxTTL)
	setDuration(os.Getenv("SIS_CACHE_NEGATIVE_TTL"), &c.Cache.NegativeTTL)
	setBool(os.Getenv("SIS_PRIVACY_STRIP_ECS"), &c.Privacy.StripECS)
	setBool(os.Getenv("SIS_PRIVACY_BLOCK_LOCAL_PTR"), &c.Privacy.BlockLocalPTR)
	setBool(os.Getenv("SIS_LOGGING_QUERY_LOG"), &c.Logging.QueryLog)
	setBool(os.Getenv("SIS_LOGGING_AUDIT_LOG"), &c.Logging.AuditLog)
	setInt(os.Getenv("SIS_LOGGING_ROTATE_SIZE_MB"), &c.Logging.RotateSizeMB)
	setInt(os.Getenv("SIS_LOGGING_RETENTION_DAYS"), &c.Logging.RetentionDays)
	setBool(os.Getenv("SIS_LOGGING_GZIP"), &c.Logging.Gzip)
	setString(os.Getenv("SIS_BLOCK_RESPONSE_A"), &c.Block.ResponseA)
	setString(os.Getenv("SIS_BLOCK_RESPONSE_AAAA"), &c.Block.ResponseAAAA)
	setDuration(os.Getenv("SIS_BLOCK_RESPONSE_TTL"), &c.Block.ResponseTTL)
	setBool(os.Getenv("SIS_BLOCK_USE_NXDOMAIN"), &c.Block.UseNXDOMAIN)
	setBool(os.Getenv("SIS_AUTH_FIRST_RUN"), &c.Auth.FirstRun)
	setDuration(os.Getenv("SIS_AUTH_SESSION_TTL"), &c.Auth.SessionTTL)
}

func setString(v string, target *string) {
	if v != "" {
		*target = v
	}
}

func setInt(v string, target *int) {
	if v == "" {
		return
	}
	n, err := strconv.Atoi(v)
	if err == nil {
		*target = n
	}
}

func setBool(v string, target *bool) {
	if v == "" {
		return
	}
	b, err := strconv.ParseBool(v)
	if err == nil {
		*target = b
	}
}

func setDuration(v string, target *Duration) {
	if v == "" {
		return
	}
	d, err := time.ParseDuration(v)
	if err == nil {
		target.Duration = d
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
