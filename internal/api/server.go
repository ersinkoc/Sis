package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	sisdns "github.com/ersinkoc/sis/internal/dns"
	sislog "github.com/ersinkoc/sis/internal/log"
	"github.com/ersinkoc/sis/internal/policy"
	"github.com/ersinkoc/sis/internal/stats"
	"github.com/ersinkoc/sis/internal/store"
	"github.com/ersinkoc/sis/internal/upstream"
	"github.com/ersinkoc/sis/internal/webui"
)

// Server serves the authenticated management API and embedded WebUI.
type Server struct {
	cfg          *config.Holder
	handler      http.Handler
	server       *http.Server
	log          *slog.Logger
	queryLog     *sislog.Query
	audit        *sislog.Audit
	policy       *policy.Engine
	stats        *stats.Counters
	store        store.Store
	syncer       *policy.Syncer
	upstream     *upstream.Pool
	cache        *sisdns.Cache
	pipeline     *sisdns.Pipeline
	configPath   string
	loginLimiter *rateLimiter
}

// New creates an API server with default dependencies.
func New(cfg *config.Holder, logger *slog.Logger) *Server {
	return NewWithDeps(Options{Config: cfg, Logger: logger})
}

// Options wires runtime dependencies into the API server.
type Options struct {
	Config     *config.Holder
	Logger     *slog.Logger
	QueryLog   *sislog.Query
	Audit      *sislog.Audit
	Policy     *policy.Engine
	Stats      *stats.Counters
	Store      store.Store
	Syncer     *policy.Syncer
	Upstream   *upstream.Pool
	Cache      *sisdns.Cache
	Pipeline   *sisdns.Pipeline
	ConfigPath string
}

// NewWithDeps creates an API server with explicit dependencies.
func NewWithDeps(opts Options) *Server {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		cfg: opts.Config, log: logger, queryLog: opts.QueryLog,
		audit: opts.Audit, policy: opts.Policy, stats: opts.Stats, store: opts.Store,
		syncer: opts.Syncer, upstream: opts.Upstream, cache: opts.Cache,
		pipeline: opts.Pipeline,
		configPath: opts.ConfigPath, loginLimiter: newRateLimiter(5, time.Minute),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("POST /api/v1/auth/setup", s.setup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/me", s.me)
	mux.HandleFunc("GET /api/v1/stats/summary", s.statsSummary)
	mux.HandleFunc("GET /api/v1/stats/timeseries", s.statsTimeseries)
	mux.HandleFunc("GET /api/v1/stats/upstreams", s.statsUpstreams)
	mux.HandleFunc("GET /api/v1/stats/top-domains", s.statsTopDomains)
	mux.HandleFunc("GET /api/v1/stats/top-clients", s.statsTopClients)
	mux.HandleFunc("GET /api/v1/logs/query", s.queryLogList)
	mux.HandleFunc("GET /api/v1/logs/query/stream", s.queryLogStream)
	mux.HandleFunc("GET /api/v1/clients", s.clientsList)
	mux.HandleFunc("GET /api/v1/clients/{key}", s.clientGet)
	mux.HandleFunc("PATCH /api/v1/clients/{key}", s.clientPatch)
	mux.HandleFunc("DELETE /api/v1/clients/{key}", s.clientDelete)
	mux.HandleFunc("GET /api/v1/allowlist", s.allowlistGet)
	mux.HandleFunc("POST /api/v1/allowlist", s.allowlistAdd)
	mux.HandleFunc("DELETE /api/v1/allowlist/{domain}", s.allowlistDelete)
	mux.HandleFunc("GET /api/v1/custom-blocklist", s.customBlocklistGet)
	mux.HandleFunc("POST /api/v1/custom-blocklist", s.customBlocklistAdd)
	mux.HandleFunc("DELETE /api/v1/custom-blocklist/{domain}", s.customBlocklistDelete)
	mux.HandleFunc("GET /api/v1/blocklists", s.blocklistsList)
	mux.HandleFunc("POST /api/v1/blocklists", s.blocklistCreate)
	mux.HandleFunc("PATCH /api/v1/blocklists/{id}", s.blocklistPatch)
	mux.HandleFunc("DELETE /api/v1/blocklists/{id}", s.blocklistDelete)
	mux.HandleFunc("POST /api/v1/blocklists/{id}/sync", s.blocklistSync)
	mux.HandleFunc("GET /api/v1/blocklists/{id}/entries", s.blocklistEntries)
	mux.HandleFunc("GET /api/v1/upstreams", s.upstreamsList)
	mux.HandleFunc("POST /api/v1/upstreams", s.upstreamCreate)
	mux.HandleFunc("PATCH /api/v1/upstreams/{id}", s.upstreamPatch)
	mux.HandleFunc("DELETE /api/v1/upstreams/{id}", s.upstreamDelete)
	mux.HandleFunc("POST /api/v1/upstreams/{id}/test", s.upstreamTest)
	mux.HandleFunc("GET /api/v1/groups", s.groupsList)
	mux.HandleFunc("POST /api/v1/groups", s.groupCreate)
	mux.HandleFunc("GET /api/v1/groups/{name}", s.groupGet)
	mux.HandleFunc("PATCH /api/v1/groups/{name}", s.groupPatch)
	mux.HandleFunc("DELETE /api/v1/groups/{name}", s.groupDelete)
	mux.HandleFunc("GET /api/v1/settings", s.settingsGet)
	mux.HandleFunc("PATCH /api/v1/settings", s.settingsPatch)
	mux.HandleFunc("POST /api/v1/query/test", s.queryTest)
	mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
	mux.HandleFunc("POST /api/v1/system/cache/flush", s.cacheFlush)
	mux.HandleFunc("GET /api/v1/system/config/history", s.configHistory)
	mux.HandleFunc("POST /api/v1/system/config/reload", s.configReload)
	mux.Handle("/", webui.Handler())
	s.handler = s.middleware(mux)
	return s
}

// Handler returns the server's root HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Start listens and serves HTTP or HTTPS until ctx is canceled.
func (s *Server) Start(ctx context.Context) error {
	addr := "0.0.0.0:8080"
	httpCfg := config.HTTPServer{}
	if s.cfg != nil && s.cfg.Get() != nil {
		httpCfg = s.cfg.Get().Server.HTTP
	}
	if httpCfg.Listen != "" {
		addr = httpCfg.Listen
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.server = &http.Server{Handler: s.handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()
	if httpCfg.TLS {
		err = s.server.ServeTLS(ln, httpCfg.CertFile, httpCfg.KeyFile)
	} else {
		err = s.server.Serve(ln)
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}` + "\n"))
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ready":true}` + "\n"))
}

func (s *Server) statsSummary(w http.ResponseWriter, _ *http.Request) {
	if s.stats == nil {
		http.Error(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.stats.Snapshot())
}

func (s *Server) queryLogStream(w http.ResponseWriter, r *http.Request) {
	if s.queryLog == nil {
		http.Error(w, "query log unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	sub := s.queryLog.SubscribeReplay(64, true)
	defer s.queryLog.Unsubscribe(sub)
	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-sub:
			if !ok {
				return
			}
			if _, err := fmt.Fprint(w, "data: "); err != nil {
				return
			}
			if err := enc.Encode(entry); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return recoverMiddleware(s.log)(securityHeaders(requestID(accessLog(s.log, s.authRequired(next)))))
}

func (s *Server) authRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		if s.cfg != nil && s.cfg.Get() != nil && s.cfg.Get().Auth.FirstRun {
			http.Error(w, "setup required", http.StatusPreconditionRequired)
			return
		}
		session, ok := s.sessionFromRequest(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.setSessionCookie(w, r, session)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authExempt(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		return true
	}
	switch r.URL.Path {
	case "/healthz", "/readyz", "/api/v1/auth/setup", "/api/v1/auth/login":
		return true
	default:
		return false
	}
}

func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					logger.Error("http panic", "panic", v, "stack", string(debug.Stack()))
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}
