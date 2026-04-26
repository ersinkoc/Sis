package api

import (
	"net/http"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	sisdns "github.com/ersinkoc/sis/internal/dns"
	"github.com/ersinkoc/sis/internal/store"
	"gopkg.in/yaml.v3"
)

func (s *Server) applyConfig(w http.ResponseWriter, next *config.Config, action, target string, before, after any) bool {
	if next == nil {
		http.Error(w, "config is required", http.StatusBadRequest)
		return false
	}
	if _, err := config.EnsureLogSalt(next); err != nil {
		s.internalError(w, "config update failed", err)
		return false
	}
	if err := config.Validate(next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	if s.configPath != "" {
		if err := (&config.Loader{Path: s.configPath}).Save(next); err != nil {
			s.internalError(w, "config update failed", err)
			return false
		}
	}
	if s.store != nil {
		if err := appendConfigHistory(s.store, next); err != nil {
			s.internalError(w, "config update failed", err)
			return false
		}
	}
	if s.policy != nil {
		if err := s.policy.ReloadConfig(next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return false
		}
	}
	if s.queryLog != nil {
		if err := s.queryLog.Reconfigure(next); err != nil {
			s.internalError(w, "config update failed", err)
			return false
		}
	}
	if s.audit != nil {
		if err := s.audit.Reconfigure(next); err != nil {
			s.internalError(w, "config update failed", err)
			return false
		}
	}
	if s.upstream != nil {
		s.upstream.Replace(next.Upstreams)
	}
	if s.cache != nil {
		s.cache.Reconfigure(sisdns.CacheOptions{
			MaxEntries: next.Cache.MaxEntries,
			MinTTL:     next.Cache.MinTTL.Duration, MaxTTL: next.Cache.MaxTTL.Duration,
			NegativeTTL: next.Cache.NegativeTTL.Duration,
		})
	}
	if s.pipeline != nil {
		s.pipeline.Reconfigure(next)
	}
	if s.cfg != nil {
		s.cfg.Replace(next)
	}
	if s.audit != nil {
		_ = s.audit.Auditf(action, target, before, after)
	}
	return true
}

func appendConfigHistory(st store.Store, cfg *config.Config) error {
	raw, err := yaml.Marshal(config.RedactedCopy(cfg))
	if err != nil {
		return err
	}
	return st.ConfigHistory().Append(&store.ConfigSnapshot{
		TS:   time.Now().UTC(),
		YAML: string(raw),
	})
}

func cloneConfig(c *config.Config) *config.Config {
	if c == nil {
		return nil
	}
	next := *c
	next.Upstreams = append([]config.Upstream(nil), c.Upstreams...)
	next.Blocklists = append([]config.Blocklist(nil), c.Blocklists...)
	next.Allowlist.Domains = append([]string(nil), c.Allowlist.Domains...)
	next.Groups = append([]config.Group(nil), c.Groups...)
	for i := range next.Groups {
		next.Groups[i].Blocklists = append([]string(nil), c.Groups[i].Blocklists...)
		next.Groups[i].Allowlist = append([]string(nil), c.Groups[i].Allowlist...)
		next.Groups[i].Schedules = append([]config.Schedule(nil), c.Groups[i].Schedules...)
		for j := range next.Groups[i].Schedules {
			next.Groups[i].Schedules[j].Days = append([]string(nil), c.Groups[i].Schedules[j].Days...)
			next.Groups[i].Schedules[j].Block = append([]string(nil), c.Groups[i].Schedules[j].Block...)
		}
	}
	next.Clients = append([]config.Client(nil), c.Clients...)
	next.Auth.Users = append([]config.User(nil), c.Auth.Users...)
	return &next
}
