package api

import (
	"context"
	"net/http"
	"time"

	mdns "github.com/miekg/dns"
)

func (s *Server) upstreamsList(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	healthy := map[string]bool{}
	if s.upstream != nil {
		for _, id := range s.upstream.HealthyIDs() {
			healthy[id] = true
		}
	}
	type upstreamView struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		URL       string   `json:"url"`
		Bootstrap []string `json:"bootstrap"`
		Healthy   bool     `json:"healthy"`
	}
	var out []upstreamView
	for _, u := range s.cfg.Get().Upstreams {
		out = append(out, upstreamView{
			ID: u.ID, Name: u.Name, URL: u.URL, Bootstrap: append([]string(nil), u.Bootstrap...),
			Healthy: healthy[u.ID],
		})
	}
	writeJSON(w, out)
}

func (s *Server) upstreamTest(w http.ResponseWriter, r *http.Request) {
	if s.upstream == nil {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := s.upstream.Test(ctx, r.PathValue("id"))
	if err != nil {
		s.gatewayError(w, "upstream test failed", err)
		return
	}
	writeJSON(w, map[string]any{
		"rcode":      resp.Rcode,
		"latency_us": time.Since(start).Microseconds(),
		"answers":    answerCount(resp),
	})
}

func answerCount(resp *mdns.Msg) int {
	if resp == nil {
		return 0
	}
	return len(resp.Answer)
}
