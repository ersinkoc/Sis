package api

import (
	"net/http"
	"strings"

	"github.com/ersinkoc/sis/internal/config"
)

type upstreamPatchRequest struct {
	ID        *string          `json:"id"`
	Name      *string          `json:"name"`
	URL       *string          `json:"url"`
	Bootstrap *[]string        `json:"bootstrap"`
	Timeout   *config.Duration `json:"timeout"`
}

func (s *Server) upstreamCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	var upstream config.Upstream
	if err := decodeJSON(r, &upstream); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	upstream.ID = strings.TrimSpace(upstream.ID)
	if upstream.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	next := cloneConfig(s.cfg.Get())
	if upstreamIndex(next.Upstreams, upstream.ID) >= 0 {
		http.Error(w, "upstream already exists", http.StatusConflict)
		return
	}
	next.Upstreams = append(next.Upstreams, upstream)
	if !s.applyConfig(w, next, "upstream.create", upstream.ID, nil, upstream) {
		return
	}
	writeJSONStatus(w, http.StatusCreated, upstream)
}

func (s *Server) upstreamPatch(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	next := cloneConfig(s.cfg.Get())
	idx := upstreamIndex(next.Upstreams, id)
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	before := next.Upstreams[idx]
	var patch upstreamPatchRequest
	if err := decodeJSON(r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	upstream := before
	if patch.ID != nil {
		upstream.ID = strings.TrimSpace(*patch.ID)
	}
	if upstream.ID == "" {
		upstream.ID = id
	}
	if patch.Name != nil {
		upstream.Name = *patch.Name
	}
	if patch.URL != nil {
		upstream.URL = *patch.URL
	}
	if patch.Bootstrap != nil {
		upstream.Bootstrap = append([]string(nil), (*patch.Bootstrap)...)
	}
	if patch.Timeout != nil {
		upstream.Timeout = *patch.Timeout
	}
	if upstream.ID != id && upstreamIndex(next.Upstreams, upstream.ID) >= 0 {
		http.Error(w, "upstream already exists", http.StatusConflict)
		return
	}
	next.Upstreams[idx] = upstream
	if !s.applyConfig(w, next, "upstream.update", id, before, upstream) {
		return
	}
	writeJSON(w, upstream)
}

func (s *Server) upstreamDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	next := cloneConfig(s.cfg.Get())
	idx := upstreamIndex(next.Upstreams, id)
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	before := next.Upstreams[idx]
	next.Upstreams = append(next.Upstreams[:idx], next.Upstreams[idx+1:]...)
	if !s.applyConfig(w, next, "upstream.delete", id, before, nil) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func upstreamIndex(upstreams []config.Upstream, id string) int {
	for i, upstream := range upstreams {
		if upstream.ID == id {
			return i
		}
	}
	return -1
}
