package api

import (
	"net/http"
	"strings"

	"github.com/ersinkoc/sis/internal/config"
)

func (s *Server) groupsList(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.cfg.Get().Groups)
}

func (s *Server) groupGet(w http.ResponseWriter, r *http.Request) {
	group, ok := s.findGroup(r.PathValue("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, group)
}

func (s *Server) groupCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	var group config.Group
	if err := decodeJSON(r, &group); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if _, ok := s.findGroup(group.Name); ok {
		http.Error(w, "group already exists", http.StatusConflict)
		return
	}
	next := cloneConfig(s.cfg.Get())
	next.Groups = append(next.Groups, group)
	if !s.applyConfig(w, next, "group.create", group.Name, nil, group) {
		return
	}
	writeJSONStatus(w, http.StatusCreated, group)
}

func (s *Server) groupPatch(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	name := r.PathValue("name")
	next := cloneConfig(s.cfg.Get())
	idx := groupIndex(next.Groups, name)
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	before := next.Groups[idx]
	var patch config.Group
	if err := decodeJSON(r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	patch.Name = strings.TrimSpace(patch.Name)
	if patch.Name == "" {
		patch.Name = name
	}
	if name == "default" && patch.Name != "default" {
		http.Error(w, "default group cannot be renamed", http.StatusBadRequest)
		return
	}
	if patch.Name != name && groupIndex(next.Groups, patch.Name) >= 0 {
		http.Error(w, "group already exists", http.StatusConflict)
		return
	}
	next.Groups[idx] = patch
	if !s.applyConfig(w, next, "group.update", name, before, patch) {
		return
	}
	writeJSON(w, patch)
}

func (s *Server) groupDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	name := r.PathValue("name")
	if name == "default" {
		http.Error(w, "default group cannot be deleted", http.StatusBadRequest)
		return
	}
	next := cloneConfig(s.cfg.Get())
	idx := groupIndex(next.Groups, name)
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	before := next.Groups[idx]
	next.Groups = append(next.Groups[:idx], next.Groups[idx+1:]...)
	if !s.applyConfig(w, next, "group.delete", name, before, nil) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) findGroup(name string) (config.Group, bool) {
	if s.cfg == nil || s.cfg.Get() == nil {
		return config.Group{}, false
	}
	for _, group := range s.cfg.Get().Groups {
		if group.Name == name {
			return group, true
		}
	}
	return config.Group{}, false
}

func groupIndex(groups []config.Group, name string) int {
	for i, group := range groups {
		if group.Name == name {
			return i
		}
	}
	return -1
}
