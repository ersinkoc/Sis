package api

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) blocklistsList(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.cfg.Get().Blocklists)
}

func (s *Server) blocklistSync(w http.ResponseWriter, r *http.Request) {
	if s.syncer == nil {
		http.Error(w, "syncer unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := s.syncer.ForceSync(ctx, r.PathValue("id"))
	if err != nil {
		s.gatewayError(w, "blocklist sync failed", err)
		return
	}
	writeJSON(w, map[string]any{
		"id": result.ID, "accepted": result.Stats.Accepted,
		"from_cache": result.FromCache, "not_modified": result.NotModified,
	})
}

func (s *Server) blocklistEntries(w http.ResponseWriter, r *http.Request) {
	if s.policy == nil {
		http.Error(w, "policy unavailable", http.StatusServiceUnavailable)
		return
	}
	limit, err := intQuery(r, "limit", 100, 1000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, ok := s.policy.ListEntries(r.PathValue("id"), r.URL.Query().Get("q"), limit)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, map[string]any{"entries": entries, "count": len(entries)})
}
