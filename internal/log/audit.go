package log

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

// Audit writes administrative and system audit events.
type Audit struct {
	mu      sync.Mutex
	rotator *Rotator
	enc     *json.Encoder
	enabled bool
}

// OpenAudit creates an audit logger from config.
func OpenAudit(c *config.Config) (*Audit, error) {
	a := &Audit{}
	if err := a.Reconfigure(c); err != nil {
		return nil, err
	}
	return a, nil
}

// Reconfigure applies runtime audit logging settings.
func (a *Audit) Reconfigure(c *config.Config) error {
	if a == nil || c == nil {
		return nil
	}
	var nextRotator *Rotator
	var nextEncoder *json.Encoder
	if c.Logging.AuditLog {
		maxBytes := int64(c.Logging.RotateSizeMB) * 1024 * 1024
		rotator, err := NewRotator(filepath.Join(c.Server.DataDir, "logs", "sis-audit.log"), maxBytes, c.Logging.RetentionDays, c.Logging.Gzip)
		if err != nil {
			return err
		}
		nextRotator = rotator
		nextEncoder = json.NewEncoder(rotator)
	}
	a.mu.Lock()
	oldRotator := a.rotator
	a.enabled = c.Logging.AuditLog
	a.rotator = nextRotator
	a.enc = nextEncoder
	a.mu.Unlock()
	if oldRotator != nil {
		_ = oldRotator.Close()
	}
	return nil
}

// Write persists one audit entry when audit logging is enabled.
func (a *Audit) Write(e *AuditEntry) error {
	if a == nil || e == nil {
		return nil
	}
	entry := *e
	if entry.TS.IsZero() {
		entry.TS = time.Now().UTC()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled || a.enc == nil {
		return nil
	}
	return a.enc.Encode(&entry)
}

// Auditf writes a system audit event with before/after payloads.
func (a *Audit) Auditf(action, target string, before, after any) error {
	return a.Write(&AuditEntry{
		Actor: "system", Action: action, Target: target,
		Before: redactAuditPayload(before), After: redactAuditPayload(after),
	})
}

// Rotate forces the audit log file to rotate.
func (a *Audit) Rotate() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rotator == nil {
		return nil
	}
	return a.rotator.Rotate()
}

// Close closes the active audit log file.
func (a *Audit) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rotator == nil {
		return nil
	}
	rotator := a.rotator
	a.rotator = nil
	a.enc = nil
	a.enabled = false
	return rotator.Close()
}

func redactAuditPayload(value any) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case *config.Config:
		return config.RedactedCopy(typed)
	case config.Config:
		return config.RedactedCopy(&typed)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return value
	}
	redactAuditJSON(decoded)
	return decoded
}

func redactAuditJSON(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if sensitiveAuditKey(key) {
				typed[key] = "redacted"
				continue
			}
			redactAuditJSON(nested)
		}
	case []any:
		for _, nested := range typed {
			redactAuditJSON(nested)
		}
	}
}

func sensitiveAuditKey(key string) bool {
	normalized := strings.ToLower(key)
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "salt")
}
