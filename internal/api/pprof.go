package api

import (
	"net/http"
	"net/http/pprof"
	"strings"
)

func (s *Server) pprofIndex(w http.ResponseWriter, r *http.Request) {
	pprof.Index(w, r)
}

func (s *Server) pprofCmdline(w http.ResponseWriter, r *http.Request) {
	pprof.Cmdline(w, r)
}

func (s *Server) pprofProfile(w http.ResponseWriter, r *http.Request) {
	pprof.Profile(w, r)
}

func (s *Server) pprofSymbol(w http.ResponseWriter, r *http.Request) {
	pprof.Symbol(w, r)
}

func (s *Server) pprofTrace(w http.ResponseWriter, r *http.Request) {
	pprof.Trace(w, r)
}

func (s *Server) pprofNamed(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/system/pprof/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	pprof.Handler(name).ServeHTTP(w, r)
}
