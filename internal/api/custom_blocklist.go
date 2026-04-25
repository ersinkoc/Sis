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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	if req.Domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}
	if err := s.store.CustomLists().Add("custom", req.Domain); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.policy != nil {
		s.policy.AddCustomBlock(req.Domain)
	}
	if s.audit != nil {
		_ = s.audit.Auditf("custom-blocklist.add", req.Domain, nil, map[string]string{"domain": req.Domain})
	}
	writeJSONStatus(w, http.StatusCreated, map[string]string{"domain": req.Domain})
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
	if err := s.store.CustomLists().Remove("custom", domain); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.policy != nil {
		s.policy.RemoveCustomBlock(domain)
	}
	if s.audit != nil {
		_ = s.audit.Auditf("custom-blocklist.delete", domain, map[string]string{"domain": domain}, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}
