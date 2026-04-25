package api

import (
	"errors"
	"net/http"

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	before := *client
	var patch clientPatchRequest
	if err := decodeJSON(r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if patch.Name != nil {
		client.Name = *patch.Name
	}
	if patch.Group != nil {
		client.Group = *patch.Group
	}
	if patch.Hidden != nil {
		client.Hidden = *patch.Hidden
	}
	if client.Group == "" {
		client.Group = "default"
	}
	if err := s.store.Clients().Upsert(client); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.store.Clients().Delete(key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.audit != nil {
		_ = s.audit.Auditf("client.delete", key, client, nil)
	}
	w.WriteHeader(http.StatusNoContent)
}
