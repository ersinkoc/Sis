package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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

const maxJSONBodySize = 1 << 20

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
	dnsReady     func() bool
	configPath   string
	loginLimiter *rateLimiter
	apiLimiter   *rateLimiter
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
	DNSReady   func() bool
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
		pipeline: opts.Pipeline, dnsReady: opts.DNSReady,
		configPath: opts.ConfigPath, loginLimiter: newRateLimiter(5, time.Minute),
		apiLimiter: newRateLimiter(0, time.Minute),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /metrics", s.metrics)
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
	mux.HandleFunc("GET /api/v1/system/store/verify", s.storeVerify)
	mux.HandleFunc("POST /api/v1/system/cache/flush", s.cacheFlush)
	mux.HandleFunc("GET /api/v1/system/config/history", s.configHistory)
	mux.HandleFunc("POST /api/v1/system/config/reload", s.configReload)
	mux.HandleFunc("GET /api/v1/system/pprof/", s.pprofIndex)
	mux.HandleFunc("GET /api/v1/system/pprof/cmdline", s.pprofCmdline)
	mux.HandleFunc("GET /api/v1/system/pprof/profile", s.pprofProfile)
	mux.HandleFunc("POST /api/v1/system/pprof/symbol", s.pprofSymbol)
	mux.HandleFunc("GET /api/v1/system/pprof/symbol", s.pprofSymbol)
	mux.HandleFunc("GET /api/v1/system/pprof/trace", s.pprofTrace)
	mux.HandleFunc("GET /api/v1/system/pprof/{name}", s.pprofNamed)
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
	if s == nil || s.handler == nil {
		return errors.New("api server handler is required")
	}
	addr := "127.0.0.1:8080"
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
	s.server = newHTTPServer(s.handler)
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

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
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
	checks := map[string]string{}
	ready := true

	var cfg *config.Config
	if s.cfg != nil {
		cfg = s.cfg.Get()
	}
	if cfg == nil {
		ready = false
		checks["config"] = "unavailable"
	} else {
		checks["config"] = "ok"
		if _, err := store.VerifyBackend(cfg.Server.StoreBackend, cfg.Server.DataDir); err != nil {
			ready = false
			checks["store"] = err.Error()
		} else {
			checks["store"] = "ok"
		}
	}

	if s.upstream == nil {
		ready = false
		checks["upstreams"] = "unavailable"
	} else if len(s.upstream.AllIDs()) == 0 {
		ready = false
		checks["upstreams"] = "none configured"
	} else if len(s.upstream.HealthyIDs()) == 0 {
		ready = false
		checks["upstreams"] = "no healthy upstreams"
	} else {
		checks["upstreams"] = "ok"
	}

	if s.pipeline == nil {
		ready = false
		checks["pipeline"] = "unavailable"
	} else {
		checks["pipeline"] = "ok"
	}

	if s.dnsReady == nil {
		checks["dns"] = "not reported"
	} else if !s.dnsReady() {
		ready = false
		checks["dns"] = "listeners not ready"
	} else {
		checks["dns"] = "ok"
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSONStatus(w, status, map[string]any{
		"ready":  ready,
		"checks": checks,
	})
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
	return recoverMiddleware(s.log)(s.securityHeaders(requestID(accessLog(s.log, apiErrorEnvelope(s.csrfGuard(s.authRequired(s.apiRateLimit(next))))))))
}

func (s *Server) apiRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && !s.apiLimiter.allowWith(r, s.apiRateLimitPerMinute(), time.Minute) {
			if s.stats != nil {
				s.stats.IncRateLimited()
			}
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresOriginCheck(r) {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := r.Cookie(s.cookieName()); err != nil {
			next.ServeHTTP(w, r)
			return
		}
		if !requestFromSameOrigin(r) {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
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
		if r.URL.Path != "/api/v1/auth/logout" {
			s.setSessionCookie(w, r, session)
		}
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

func requiresOriginCheck(r *http.Request) bool {
	if r == nil || !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func requestFromSameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return headerMatchesHost(origin, r.Host)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return headerMatchesHost(referer, r.Host)
	}
	return true
}

func headerMatchesHost(raw, host string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
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

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:")
		if s.hstsEnabled(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hstsEnabled(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	return s != nil && s.cfg != nil && s.cfg.Get() != nil && s.cfg.Get().Server.HTTP.TLS
}

func (s *Server) apiRateLimitPerMinute() int {
	if s != nil && s.cfg != nil && s.cfg.Get() != nil {
		return s.cfg.Get().Server.HTTP.RateLimitPerMinute
	}
	return 600
}

func writeJSON(w http.ResponseWriter, value any) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) internalError(w http.ResponseWriter, msg string, err error) {
	if s != nil && s.log != nil && err != nil {
		s.log.Error(msg, "error", err)
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

func (s *Server) gatewayError(w http.ResponseWriter, msg string, err error) {
	if s != nil && s.log != nil && err != nil {
		s.log.Error(msg, "error", err)
	}
	http.Error(w, msg, http.StatusBadGateway)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodySize+1))
	if err != nil {
		return err
	}
	if len(raw) > maxJSONBodySize {
		return fmt.Errorf("JSON body must be <= %d bytes", maxJSONBodySize)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("body must contain a single JSON value")
	}
	return nil
}
