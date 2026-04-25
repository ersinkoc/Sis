package api

import (
	"net/http"
	"strconv"
)

func (s *Server) statsTimeseries(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "1m"
	}
	if bucket != "1m" && bucket != "1h" && bucket != "1d" {
		http.Error(w, "invalid bucket", http.StatusBadRequest)
		return
	}
	rows, err := s.store.Stats().List(bucket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	limit, err := intQuery(r, "limit", 60, 1440)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	writeJSON(w, map[string]any{"bucket": bucket, "rows": rows})
}

func (s *Server) statsUpstreams(w http.ResponseWriter, _ *http.Request) {
	if s.stats == nil {
		http.Error(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.stats.Snapshot().Upstreams)
}

func (s *Server) statsTopDomains(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		http.Error(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}
	limit, err := intQuery(r, "limit", 10, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	blocked := false
	if raw := r.URL.Query().Get("blocked"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "invalid blocked", http.StatusBadRequest)
			return
		}
		blocked = parsed
	}
	writeJSON(w, map[string]any{"domains": s.stats.TopDomains(limit, blocked)})
}

func (s *Server) statsTopClients(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		http.Error(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}
	limit, err := intQuery(r, "limit", 10, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"clients": s.stats.TopClients(limit)})
}
