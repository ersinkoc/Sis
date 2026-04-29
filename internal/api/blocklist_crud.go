package api

import (
	"net/http"
	"strings"

	"github.com/ersinkoc/sis/internal/config"
)

type blocklistPatchRequest struct {
	ID              *string          `json:"id"`
	Name            *string          `json:"name"`
	URL             *string          `json:"url"`
	Enabled         *bool            `json:"enabled"`
	RefreshInterval *config.Duration `json:"refresh_interval"`
}

func (s *Server) blocklistCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	var blocklist config.Blocklist
	if err := decodeJSON(r, &blocklist); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	blocklist.ID = strings.TrimSpace(blocklist.ID)
	if blocklist.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	next := cloneConfig(s.cfg.Get())
	if blocklistIndex(next.Blocklists, blocklist.ID) >= 0 {
		http.Error(w, "blocklist already exists", http.StatusConflict)
		return
	}
	next.Blocklists = append(next.Blocklists, blocklist)
	if !s.applyConfig(w, next, "blocklist.create", blocklist.ID, nil, blocklist) {
		return
	}
	writeJSONStatus(w, http.StatusCreated, blocklist)
}

func (s *Server) blocklistPatch(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	next := cloneConfig(s.cfg.Get())
	idx := blocklistIndex(next.Blocklists, id)
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	before := next.Blocklists[idx]
	var patch blocklistPatchRequest
	if err := decodeJSON(r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	blocklist := before
	if patch.ID != nil {
		blocklist.ID = strings.TrimSpace(*patch.ID)
	}
	if blocklist.ID == "" {
		blocklist.ID = id
	}
	if patch.Name != nil {
		blocklist.Name = *patch.Name
	}
	if patch.URL != nil {
		blocklist.URL = *patch.URL
	}
	if patch.Enabled != nil {
		blocklist.Enabled = *patch.Enabled
	}
	if patch.RefreshInterval != nil {
		blocklist.RefreshInterval = *patch.RefreshInterval
	}
	if blocklist.ID != id && blocklistIndex(next.Blocklists, blocklist.ID) >= 0 {
		http.Error(w, "blocklist already exists", http.StatusConflict)
		return
	}
	next.Blocklists[idx] = blocklist
	if !s.applyConfig(w, next, "blocklist.update", id, before, blocklist) {
		return
	}
	writeJSON(w, blocklist)
}

func (s *Server) blocklistDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	next := cloneConfig(s.cfg.Get())
	idx := blocklistIndex(next.Blocklists, id)
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	before := next.Blocklists[idx]
	next.Blocklists = append(next.Blocklists[:idx], next.Blocklists[idx+1:]...)
	if !s.applyConfig(w, next, "blocklist.delete", id, before, nil) {
		return
	}
	if s.policy != nil {
		s.policy.ReplaceList(id, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}

func blocklistIndex(blocklists []config.Blocklist, id string) int {
	for i, blocklist := range blocklists {
		if blocklist.ID == id {
			return i
		}
	}
	return -1
}
