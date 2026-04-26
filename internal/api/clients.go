package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ersinkoc/sis/internal/store"
)

type clientPatchRequest struct {
	Name   *string `json:"name"`
	Group  *string `json:"group"`
	Hidden *bool   `json:"hidden"`
}

func (s *Server) clientsList(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	clients, err := s.store.Clients().List()
	if err != nil {
		s.internalError(w, "clients unavailable", err)
		return
	}
	writeJSON(w, clients)
}

func (s *Server) clientGet(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	client, err := s.store.Clients().Get(r.PathValue("key"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, "client unavailable", err)
		return
	}
	writeJSON(w, client)
}

func (s *Server) clientPatch(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	key := r.PathValue("key")
	client, err := s.store.Clients().Get(key)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, "client unavailable", err)
		return
	}
	before := *client
	var patch clientPatchRequest
	if err := decodeJSON(r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if patch.Name != nil {
		client.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Group != nil {
		client.Group = strings.TrimSpace(*patch.Group)
	}
	if patch.Hidden != nil {
		client.Hidden = *patch.Hidden
	}
	if client.Group == "" {
		client.Group = "default"
	}
	if !s.clientGroupExists(client.Group) {
		http.Error(w, "unknown group", http.StatusBadRequest)
		return
	}
	if err := s.store.Clients().Upsert(client); err != nil {
		s.internalError(w, "client update failed", err)
		return
	}
	if s.audit != nil {
		_ = s.audit.Auditf("client.update", key, before, client)
	}
	writeJSON(w, client)
}

func (s *Server) clientDelete(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	key := r.PathValue("key")
	client, err := s.store.Clients().Get(key)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, "client unavailable", err)
		return
	}
	if err := s.store.Clients().Delete(key); err != nil {
		s.internalError(w, "client delete failed", err)
		return
	}
	if s.audit != nil {
		_ = s.audit.Auditf("client.delete", key, client, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clientGroupExists(group string) bool {
	if s.cfg == nil || s.cfg.Get() == nil {
		return true
	}
	for _, configured := range s.cfg.Get().Groups {
		if configured.Name == group {
			return true
		}
	}
	return false
}
