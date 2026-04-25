package api

import "net/http"

func (s *Server) cacheFlush(w http.ResponseWriter, _ *http.Request) {
	if s.cache == nil {
		http.Error(w, "cache unavailable", http.StatusServiceUnavailable)
		return
	}
	before := s.cache.Len()
	s.cache.Flush()
	if s.audit != nil {
		_ = s.audit.Auditf("cache.flush", "dns-cache", map[string]int{"entries": before}, map[string]int{"entries": 0})
	}
	writeJSON(w, map[string]any{"flushed": true, "entries": before})
}
