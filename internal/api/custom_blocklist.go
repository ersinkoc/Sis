package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/ersinkoc/sis/internal/store"
)

type customBlockAddRequest struct {
	Domain string `json:"domain"`
}

func (s *Server) customBlocklistGet(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	domains, err := s.store.CustomLists().List("custom")
	if err != nil {
		s.internalError(w, "custom blocklist unavailable", err)
		return
	}
	writeJSON(w, map[string]any{"domains": domains})
}

func (s *Server) customBlocklistAdd(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	var req customBlockAddRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	domain, ok := normalizeDomainInput(req.Domain)
	if !ok {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	if err := s.store.CustomLists().Add("custom", domain); err != nil {
		s.internalError(w, "custom blocklist update failed", err)
		return
	}
	if s.policy != nil {
		s.policy.AddCustomBlock(domain)
	}
	if s.audit != nil {
		_ = s.audit.Auditf("custom-blocklist.add", domain, nil, map[string]string{"domain": domain})
	}
	writeJSONStatus(w, http.StatusCreated, map[string]string{"domain": domain})
}

func (s *Server) customBlocklistDelete(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	domain, err := url.PathUnescape(r.PathValue("domain"))
	if err != nil || domain == "" {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	normalized, ok := normalizeDomainInput(domain)
	if !ok {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	if err := s.store.CustomLists().Remove("custom", normalized); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.internalError(w, "custom blocklist update failed", err)
		return
	}
	if s.policy != nil {
		s.policy.RemoveCustomBlock(normalized)
	}
	if s.audit != nil {
		_ = s.audit.Auditf("custom-blocklist.delete", normalized, map[string]string{"domain": normalized}, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}
