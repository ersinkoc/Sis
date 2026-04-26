package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/ersinkoc/sis/internal/store"
)

type allowlistAddRequest struct {
	Domain string `json:"domain"`
}

func (s *Server) allowlistGet(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	domains, err := s.store.CustomLists().List("custom-allow")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"domains": domains})
}

func (s *Server) allowlistAdd(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	var req allowlistAddRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	domain, ok := normalizeDomainInput(req.Domain)
	if !ok {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	if err := s.store.CustomLists().Add("custom-allow", domain); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.policy != nil {
		s.policy.AddCustomAllow(domain)
	}
	if s.audit != nil {
		_ = s.audit.Auditf("allowlist.add", domain, nil, map[string]string{"domain": domain})
	}
	writeJSONStatus(w, http.StatusCreated, map[string]string{"domain": domain})
}

func (s *Server) allowlistDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.CustomLists().Remove("custom-allow", normalized); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.policy != nil {
		s.policy.RemoveCustomAllow(normalized)
	}
	if s.audit != nil {
		_ = s.audit.Auditf("allowlist.delete", normalized, map[string]string{"domain": normalized}, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}
