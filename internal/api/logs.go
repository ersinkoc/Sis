package api

import (
	"net/http"
	"strconv"

	sislog "github.com/ersinkoc/sis/internal/log"
)

func (s *Server) queryLogList(w http.ResponseWriter, r *http.Request) {
	if s.queryLog == nil {
		http.Error(w, "query log unavailable", http.StatusServiceUnavailable)
		return
	}
	limit, err := intQuery(r, "limit", 100, 1000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var blocked *bool
	if raw := r.URL.Query().Get("blocked"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "invalid blocked", http.StatusBadRequest)
			return
		}
		blocked = &parsed
	}
	entries := s.queryLog.Recent(sislog.Filter{
		Client:  r.URL.Query().Get("client"),
		QName:   r.URL.Query().Get("qname"),
		Blocked: blocked,
		Limit:   limit,
	})
	writeJSON(w, map[string]any{"entries": entries})
}
