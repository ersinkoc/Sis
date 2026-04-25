package api

import (
	"net/http"

	"github.com/ersinkoc/sis/internal/config"
)

type settingsView struct {
	Cache   config.Cache   `json:"cache"`
	Privacy config.Privacy `json:"privacy"`
	Logging config.Logging `json:"logging"`
	Block   config.Block   `json:"block"`
}

type settingsPatch struct {
	Cache   *config.Cache   `json:"cache"`
	Privacy *config.Privacy `json:"privacy"`
	Logging *config.Logging `json:"logging"`
	Block   *config.Block   `json:"block"`
}

func (s *Server) settingsGet(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	cfg := s.cfg.Get()
	writeJSON(w, settingsView{Cache: cfg.Cache, Privacy: cfg.Privacy, Logging: cfg.Logging, Block: cfg.Block})
}

func (s *Server) settingsPatch(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	var patch settingsPatch
	if err := decodeJSON(r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before := settingsView{Cache: s.cfg.Get().Cache, Privacy: s.cfg.Get().Privacy, Logging: s.cfg.Get().Logging, Block: s.cfg.Get().Block}
	next := cloneConfig(s.cfg.Get())
	if patch.Cache != nil {
		next.Cache = *patch.Cache
	}
	if patch.Privacy != nil {
		next.Privacy = *patch.Privacy
	}
	if patch.Logging != nil {
		next.Logging = *patch.Logging
	}
	if patch.Block != nil {
		next.Block = *patch.Block
	}
	after := settingsView{Cache: next.Cache, Privacy: next.Privacy, Logging: next.Logging, Block: next.Block}
	if !s.applyConfig(w, next, "settings.update", "settings", before, after) {
		return
	}
	writeJSON(w, after)
}
