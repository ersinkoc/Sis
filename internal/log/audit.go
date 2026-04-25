package log

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

type Audit struct {
	mu      sync.Mutex
	rotator *Rotator
	enc     *json.Encoder
	enabled bool
}

func OpenAudit(c *config.Config) (*Audit, error) {
	a := &Audit{}
	if err := a.Reconfigure(c); err != nil {
		return nil, err
	}
	return a, nil
}

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

func (a *Audit) Auditf(action, target string, before, after any) error {
	return a.Write(&AuditEntry{
		Actor: "system", Action: action, Target: target,
		Before: before, After: after,
	})
}

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

func (a *Audit) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rotator == nil {
		return nil
	}
	return a.rotator.Close()
}
