package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	sisdns "github.com/ersinkoc/sis/internal/dns"
	mdns "github.com/miekg/dns"
)

type queryTestRequest struct {
	Domain   string `json:"domain"`
	Type     string `json:"type"`
	ClientIP string `json:"client_ip"`
	Proto    string `json:"proto"`
}

type queryTestResponse struct {
	Domain    string   `json:"domain"`
	Type      string   `json:"type"`
	RCode     string   `json:"rcode"`
	Source    string   `json:"source"`
	LatencyUS int64    `json:"latency_us"`
	Answers   []string `json:"answers"`
}

func (s *Server) queryTest(w http.ResponseWriter, r *http.Request) {
	if s.pipeline == nil {
		http.Error(w, "query pipeline unavailable", http.StatusServiceUnavailable)
		return
	}
	var req queryTestRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}
	qtypeName := strings.ToUpper(strings.TrimSpace(req.Type))
	if qtypeName == "" {
		qtypeName = "A"
	}
	qtype, ok := mdns.StringToType[qtypeName]
	if !ok {
		http.Error(w, "unknown query type", http.StatusBadRequest)
		return
	}
	srcIP := net.ParseIP(req.ClientIP)
	if srcIP == nil {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			srcIP = net.ParseIP(host)
		}
	}
	if srcIP == nil {
		srcIP = net.IPv4(127, 0, 0, 1)
	}
	proto := strings.ToLower(strings.TrimSpace(req.Proto))
	if proto == "" {
		proto = "api"
	}
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(req.Domain), qtype)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp := s.pipeline.Handle(ctx, &sisdns.Request{
		Msg: msg, SrcIP: srcIP, Proto: proto, StartedAt: time.Now(),
	})
	if resp == nil || resp.Msg == nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, queryTestResponse{
		Domain: mdns.Fqdn(req.Domain), Type: qtypeName,
		RCode: mdns.RcodeToString[resp.Msg.Rcode], Source: resp.Source,
		LatencyUS: resp.Latency.Microseconds(), Answers: queryAnswers(resp.Msg),
	})
}

func queryAnswers(msg *mdns.Msg) []string {
	if msg == nil || len(msg.Answer) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(msg.Answer))
	for _, rr := range msg.Answer {
		out = append(out, rr.String())
	}
	return out
}
