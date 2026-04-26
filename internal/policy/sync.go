package policy

import (
	"context"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

// AuditLogger is the audit sink used by Syncer.
type AuditLogger interface {
	Auditf(action, target string, before, after any) error
}

// Syncer keeps enabled blocklists refreshed and swapped into the policy engine.
type Syncer struct {
	cfg     *config.Holder
	fetcher *Fetcher
	engine  *Engine
	audit   AuditLogger

	mu      sync.Mutex
	lastRun map[string]time.Time
}

// NewSyncer creates a blocklist sync worker.
func NewSyncer(cfg *config.Holder, fetcher *Fetcher, engine *Engine, audit AuditLogger) *Syncer {
	return &Syncer{
		cfg: cfg, fetcher: fetcher, engine: engine, audit: audit,
		lastRun: make(map[string]time.Time),
	}
}

// Run periodically syncs due blocklists until ctx is canceled.
func (s *Syncer) Run(ctx context.Context) {
	if s == nil {
		return
	}
	s.syncDue(ctx, true)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncDue(ctx, false)
		}
	}
}

// ForceSync synchronizes one configured blocklist immediately.
func (s *Syncer) ForceSync(ctx context.Context, id string) (*FetchResult, error) {
	cfg := s.cfg.Get()
	for _, list := range cfg.Blocklists {
		if list.ID == id {
			return s.syncOne(ctx, list)
		}
	}
	return nil, errUnknownList(id)
}

func (s *Syncer) syncDue(ctx context.Context, initial bool) {
	cfg := s.cfg.Get()
	now := time.Now()
	for _, list := range cfg.Blocklists {
		if !list.Enabled {
			continue
		}
		interval := list.RefreshInterval.Duration
		if interval <= 0 {
			interval = 24 * time.Hour
		}
		s.mu.Lock()
		last := s.lastRun[list.ID]
		due := initial || last.IsZero() || now.Sub(last) >= interval
		s.mu.Unlock()
		if !due {
			continue
		}
		_, _ = s.syncOne(ctx, list)
	}
}

func (s *Syncer) syncOne(ctx context.Context, list config.Blocklist) (*FetchResult, error) {
	result, err := s.fetcher.Fetch(ctx, list.ID, list.URL)
	if err != nil {
		if s.audit != nil {
			_ = s.audit.Auditf("blocklist.sync.failure", list.ID, nil, map[string]any{"error": err.Error()})
		}
		return nil, err
	}
	s.engine.ReplaceList(list.ID, result.Domains)
	s.mu.Lock()
	s.lastRun[list.ID] = time.Now()
	s.mu.Unlock()
	if s.audit != nil {
		_ = s.audit.Auditf("blocklist.sync.success", list.ID, nil, map[string]any{
			"accepted": result.Stats.Accepted, "from_cache": result.FromCache, "not_modified": result.NotModified,
		})
	}
	return result, nil
}
