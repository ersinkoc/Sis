package api

import (
	"net/http"

	"github.com/ersinkoc/sis/internal/config"
	"github.com/ersinkoc/sis/internal/store"
	"gopkg.in/yaml.v3"
)

func (s *Server) systemInfo(w http.ResponseWriter, _ *http.Request) {
	info := map[string]any{"service": "sis"}
	if s.cfg != nil && s.cfg.Get() != nil {
		info["dns_listen"] = s.cfg.Get().Server.DNS.Listen
		info["http_listen"] = s.cfg.Get().Server.HTTP.Listen
		info["http_tls"] = s.cfg.Get().Server.HTTP.TLS
		info["data_dir"] = s.cfg.Get().Server.DataDir
		backend := s.cfg.Get().Server.StoreBackend
		if backend == "" {
			backend = store.BackendJSON
		}
		info["store_backend"] = backend
		info["first_run"] = s.cfg.Get().Auth.FirstRun
	}
	writeJSON(w, info)
}

func (s *Server) storeVerify(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	cfg := s.cfg.Get()
	result, err := store.VerifyBackend(cfg.Server.StoreBackend, cfg.Server.DataDir)
	if err != nil {
		s.internalError(w, "store verification failed", err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "store": result})
}

func (s *Server) configReload(w http.ResponseWriter, _ *http.Request) {
	if s.configPath == "" || s.cfg == nil {
		http.Error(w, "config reload unavailable", http.StatusServiceUnavailable)
		return
	}
	next, err := (&configLoader{path: s.configPath}).load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.applyConfig(w, next, "config.reload", s.configPath, nil, nil) {
		return
	}
	writeJSON(w, map[string]any{"reloaded": true})
}

type configHistoryView struct {
	TS   string `json:"ts"`
	YAML string `json:"yaml"`
}

func (s *Server) configHistory(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	limit, err := intQuery(r, "limit", 20, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	snapshots, err := s.store.ConfigHistory().List(limit)
	if err != nil {
		s.internalError(w, "config history unavailable", err)
		return
	}
	out := make([]configHistoryView, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, configHistoryView{
			TS:   snapshot.TS.Format(http.TimeFormat),
			YAML: redactConfigSnapshot(snapshot),
		})
	}
	writeJSON(w, map[string]any{"snapshots": out})
}

func redactConfigSnapshot(snapshot *store.ConfigSnapshot) string {
	if snapshot == nil || snapshot.YAML == "" {
		return ""
	}
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(snapshot.YAML), &cfg); err != nil {
		return ""
	}
	for i := range cfg.Auth.Users {
		if cfg.Auth.Users[i].PasswordHash != "" {
			cfg.Auth.Users[i].PasswordHash = "redacted"
		}
	}
	if cfg.Privacy.LogSalt != "" {
		cfg.Privacy.LogSalt = "redacted"
	}
	raw, err := yaml.Marshal(&cfg)
	if err != nil {
		return ""
	}
	return string(raw)
}

type configLoader struct {
	path string
}

func (l configLoader) load() (*config.Config, error) {
	return (&config.Loader{Path: l.path}).Load()
}
