package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	sisdns "github.com/ersinkoc/sis/internal/dns"
	sislog "github.com/ersinkoc/sis/internal/log"
	"github.com/ersinkoc/sis/internal/policy"
	"github.com/ersinkoc/sis/internal/stats"
	"github.com/ersinkoc/sis/internal/store"
	"github.com/ersinkoc/sis/internal/upstream"
)

func TestHealthz(t *testing.T) {
	s := New(testHolder(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
}

func TestAccessLogIncludesRequestID(t *testing.T) {
	var logs bytes.Buffer
	s := New(testHolder(), slog.New(slog.NewTextHandler(&logs, nil)))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "request-log-test")
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := logs.String(); !strings.Contains(got, "request_id=request-log-test") {
		t.Fatalf("access log missing request id: %s", got)
	}
}

func TestMetricsExposesPrometheusCounters(t *testing.T) {
	counters := stats.New()
	counters.IncQuery()
	counters.IncCacheHit()
	counters.IncBlocked()
	counters.ObserveLatency(3 * time.Millisecond)
	upstream := counters.Upstream("cloudflare")
	upstream.IncRequest()
	upstream.IncError()
	upstream.MarkUnhealthy()
	upstream.ObserveLatency(12 * time.Millisecond)
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stats:  counters,
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("content type = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE sis_dns_queries_total counter",
		"sis_dns_queries_total 1",
		"sis_dns_cache_hits_total 1",
		"sis_dns_blocked_queries_total 1",
		"sis_dns_latency_observations_total 1",
		`sis_upstream_requests_total{upstream="cloudflare"} 1`,
		`sis_upstream_errors_total{upstream="cloudflare"} 1`,
		`sis_upstream_healthy{upstream="cloudflare"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
}

func TestMetricsEscapesUpstreamLabels(t *testing.T) {
	counters := stats.New()
	counters.Upstream("bad\"\\\nlabel").IncRequest()
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stats:  counters,
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `sis_upstream_requests_total{upstream="bad\"\\\nlabel"} 1`) {
		t.Fatalf("upstream label was not escaped:\n%s", rec.Body.String())
	}
}

func TestMetricsWithoutStatsReturnsUnavailable(t *testing.T) {
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPprofRoutesRequireAuthentication(t *testing.T) {
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/pprof/goroutine?debug=1", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPprofNamedProfile(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/pprof/goroutine?debug=1", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("content type = %q", got)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("goroutine profile:")) {
		t.Fatalf("unexpected pprof body: %s", rec.Body.String())
	}
}

func TestReadyzChecksRuntimeDependencies(t *testing.T) {
	holder := validAPIConfig(t)
	st, err := store.Open(holder.Get().Server.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	pool := upstream.NewPool(holder.Get().Upstreams)
	pipeline := sisdns.NewPipelineWithDeps(sisdns.PipelineOptions{
		Config:   holder,
		Upstream: pool,
	})
	s := NewWithDeps(Options{
		Config:   holder,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		Upstream: pool,
		Pipeline: pipeline,
		DNSReady: func() bool { return true },
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ready  bool              `json:"ready"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Ready || out.Checks["store"] != "ok" || out.Checks["upstreams"] != "ok" || out.Checks["pipeline"] != "ok" || out.Checks["dns"] != "ok" {
		t.Fatalf("readyz response = %#v", out)
	}
}

func TestReadyzReturnsUnavailableWhenDNSListenersNotReady(t *testing.T) {
	holder := validAPIConfig(t)
	st, err := store.Open(holder.Get().Server.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	pool := upstream.NewPool(holder.Get().Upstreams)
	pipeline := sisdns.NewPipelineWithDeps(sisdns.PipelineOptions{
		Config:   holder,
		Upstream: pool,
	})
	s := NewWithDeps(Options{
		Config:   holder,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		Upstream: pool,
		Pipeline: pipeline,
		DNSReady: func() bool { return false },
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ready  bool              `json:"ready"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Ready || out.Checks["dns"] != "listeners not ready" {
		t.Fatalf("readyz response = %#v", out)
	}
}

func TestReadyzReturnsUnavailableWhenDependenciesMissing(t *testing.T) {
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ready  bool              `json:"ready"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Ready || out.Checks["upstreams"] != "unavailable" || out.Checks["pipeline"] != "unavailable" {
		t.Fatalf("readyz response = %#v", out)
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := New(testHolder(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing content security policy")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestSecurityHeadersIncludeHSTSWhenTLSConfigured(t *testing.T) {
	holder := testHolder()
	cfg := *holder.Get()
	cfg.Server.HTTP.TLS = true
	holder.Replace(&cfg)
	s := New(holder, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("missing strict transport security")
	}
}

func TestAPIErrorEnvelopeIncludesRequestID(t *testing.T) {
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/summary", nil)
	req.Header.Set("X-Request-ID", "request-123")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type = %q", got)
	}
	var out struct {
		Error     string `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error != "unauthorized" || out.RequestID != "request-123" {
		t.Fatalf("error envelope = %#v", out)
	}
}

func TestAPIRateLimitProtectedRoutes(t *testing.T) {
	holder := validAPIConfig(t)
	cfg := *holder.Get()
	cfg.Server.HTTP.RateLimitPerMinute = 1
	holder.Replace(&cfg)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stats:  stats.New(),
		Store:  st,
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/stats/summary", nil)
	firstReq.RemoteAddr = "192.0.2.44:1234"
	addSessionCookie(t, st, firstReq)
	firstRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status = %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/stats/summary", nil)
	secondReq.RemoteAddr = "192.0.2.44:1234"
	addSessionCookie(t, st, secondReq)
	secondRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if got := s.stats.Snapshot().RateLimitedTotal; got != 1 {
		t.Fatalf("rate limited total = %d", got)
	}
}

func TestCSRFMiddlewareRejectsCrossOriginCookieMutation(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "http://sis.local/api/v1/allowlist", bytes.NewBufferString(`{"domain":"allowed.example.com"}`))
	req.Header.Set("Origin", "http://evil.example")
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCSRFMiddlewareAllowsSameOriginCookieMutation(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "http://sis.local/api/v1/allowlist", bytes.NewBufferString(`{"domain":"allowed.example.com"}`))
	req.Header.Set("Origin", "http://sis.local")
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTPServerTimeouts(t *testing.T) {
	server := newHTTPServer(http.NewServeMux())
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("read header timeout = %s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 15*time.Second {
		t.Fatalf("read timeout = %s", server.ReadTimeout)
	}
	if server.WriteTimeout != 30*time.Second {
		t.Fatalf("write timeout = %s", server.WriteTimeout)
	}
	if server.IdleTimeout != 120*time.Second {
		t.Fatalf("idle timeout = %s", server.IdleTimeout)
	}
	if server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("max header bytes = %d", server.MaxHeaderBytes)
	}
}

func TestAPIStartRequiresHandler(t *testing.T) {
	if err := (&Server{}).Start(context.Background()); err == nil {
		t.Fatal("expected missing handler error")
	}
}

func TestWebUIRootIsPublic(t *testing.T) {
	s := New(testHolder(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestWebUISPAFallback(t *testing.T) {
	s := New(testHolder(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/clients/known-device", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestStatsSummary(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	counters := stats.New()
	counters.IncQuery()
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stats:  counters,
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/summary", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() == "" {
		t.Fatal("empty body")
	}
}

func TestAuthenticatedRequestExtendsSession(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	counters := stats.New()
	holder := validAPIConfig(t)
	holder.Get().Auth.SessionTTL = config.Duration{Duration: 2 * time.Hour}
	expires := time.Now().Add(5 * time.Minute)
	if err := st.Sessions().Upsert(&store.Session{Token: "sliding-token", Username: "admin", ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stats:  counters,
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/summary", nil)
	req.AddCookie(&http.Cookie{Name: "sis_session", Value: "sliding-token"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value != "sliding-token" || !cookies[0].Expires.After(expires.Add(time.Hour)) {
		t.Fatalf("expected refreshed session cookie, got %#v", cookies)
	}
	session, err := st.Sessions().Get("sliding-token")
	if err != nil {
		t.Fatal(err)
	}
	if !session.ExpiresAt.After(expires.Add(time.Hour)) {
		t.Fatalf("session expiry was not extended enough: before=%s after=%s", expires, session.ExpiresAt)
	}
}

func TestClientPatch(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Clients().Upsert(&store.Client{Key: "192.168.1.10", Type: "ip", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	holder := testHolder()
	holder.Get().Groups = append(holder.Get().Groups, config.Group{Name: "iot"})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/clients/192.168.1.10", bytes.NewBufferString(`{"name":"TV","group":"iot","hidden":true}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	client, err := st.Clients().Get("192.168.1.10")
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "TV" || client.Group != "iot" || !client.Hidden {
		t.Fatalf("client = %#v", client)
	}
}

func TestClientPatchRejectsUnknownGroup(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Clients().Upsert(&store.Client{Key: "192.168.1.10", Type: "ip", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/clients/192.168.1.10", bytes.NewBufferString(`{"group":"missing"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestClientsListGetAndDelete(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	client := &store.Client{Key: "192.168.1.20", Type: "ip", Name: "Laptop", Group: "default"}
	if err := st.Clients().Upsert(client); err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	addSessionCookie(t, st, listReq)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !bytes.Contains(listRec.Body.Bytes(), []byte("Laptop")) {
		t.Fatalf("list status = %d body=%s", listRec.Code, listRec.Body.String())
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/clients/192.168.1.20", nil)
	addSessionCookie(t, st, getReq)
	getRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK || !bytes.Contains(getRec.Body.Bytes(), []byte("192.168.1.20")) {
		t.Fatalf("get status = %d body=%s", getRec.Code, getRec.Body.String())
	}
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/clients/192.168.1.20", nil)
	addSessionCookie(t, st, delReq)
	delRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", delRec.Code, delRec.Body.String())
	}
	if _, err := st.Clients().Get("192.168.1.20"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("client should be deleted, err=%v", err)
	}
}

func TestClientGetAndDeleteMissing(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/clients/missing", nil)
		addSessionCookie(t, st, req)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d body=%s", method, rec.Code, rec.Body.String())
		}
	}
}

func TestAPIRequiresAuth(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
		Stats:  stats.New(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/summary", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPIRequiresSetupDuringFirstRun(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Auth.FirstRun = true
	holder.Get().Auth.Users = nil
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
		Stats:  stats.New(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/summary", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAllowlistAdd(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/allowlist", bytes.NewBufferString(`{"domain":"allowed.example.com"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	domains, err := st.CustomLists().List("custom-allow")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "allowed.example.com" {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestAllowlistAddRejectsInvalidDomain(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: testHolder(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/allowlist", bytes.NewBufferString(`{"domain":"bad domain"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAllowlistDeleteMissing(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/allowlist/missing.example.com", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCustomBlocklistAdd(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/custom-blocklist", bytes.NewBufferString(`{"domain":"blocked.example.com"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	domains, err := st.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "blocked.example.com" {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestCustomBlocklistAddNormalizesDomain(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/custom-blocklist", bytes.NewBufferString(`{"domain":"  Blocked.Example.COM.  "}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	domains, err := st.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "blocked.example.com" {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestCustomBlocklistDeleteNormalizesDomain(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CustomLists().Add("custom", "blocked.example.com"); err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/custom-blocklist/Blocked.Example.COM.", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	domains, err := st.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 0 {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestSetupAndLogin(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := config.NewHolder(&config.Config{Auth: config.Auth{FirstRun: true, CookieName: "sis_session"}})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", bytes.NewBufferString(`{"username":" admin ","password":"secret123"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status = %d body=%s", rec.Code, rec.Body.String())
	}
	if holder.Get().Auth.FirstRun {
		t.Fatal("first_run should be false after setup")
	}
	if holder.Get().Auth.Users[0].Username != "admin" {
		t.Fatalf("username = %q", holder.Get().Auth.Users[0].Username)
	}
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":" admin ","password":"secret123"}`))
	loginRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	if len(loginRec.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
}

func TestSetupPersistsConfigAndSessionAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	configPath := filepath.Join(dir, "sis.yaml")
	cfg := *validAPIConfig(t).Get()
	cfg.Server.DataDir = dataDir
	cfg.Auth.FirstRun = true
	cfg.Auth.Users = nil
	cfg.Auth.CookieName = "sis_session"
	holder := config.NewHolder(&cfg)
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config:     holder,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:      st,
		ConfigPath: configPath,
	})
	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", bytes.NewBufferString(`{"username":" admin ","password":"secret123"}`))
	setupRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup status = %d body=%s", setupRec.Code, setupRec.Body.String())
	}
	cookies := setupRec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value == "" {
		t.Fatalf("setup cookie = %#v", cookies)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := (&config.Loader{Path: configPath}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.FirstRun || len(loaded.Auth.Users) != 1 || loaded.Auth.Users[0].Username != "admin" ||
		loaded.Auth.Users[0].PasswordHash == "" || loaded.Auth.Users[0].PasswordHash == "secret123" {
		t.Fatalf("loaded auth config = %#v", loaded.Auth)
	}
	reopened, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restarted := NewWithDeps(Options{
		Config: config.NewHolder(loaded),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  reopened,
	})
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.AddCookie(cookies[0])
	meRec := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", meRec.Code, meRec.Body.String())
	}
	if !bytes.Contains(meRec.Body.Bytes(), []byte(`"username":"admin"`)) {
		t.Fatalf("me body = %s", meRec.Body.String())
	}
}

func TestSystemInfoIncludesStoreBackend(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	cfg := *holder.Get()
	cfg.Server.StoreBackend = store.BackendSQLite
	holder.Replace(&cfg)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/info", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var info map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info["store_backend"] != store.BackendSQLite {
		t.Fatalf("store_backend = %#v", info["store_backend"])
	}
}

func TestSystemStoreVerify(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Clients().Upsert(&store.Client{Key: "192.0.2.88", Type: "ip"}); err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	cfg := *holder.Get()
	cfg.Server.DataDir = dataDir
	cfg.Server.StoreBackend = store.BackendJSON
	holder.Replace(&cfg)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/store/verify", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		OK    bool `json:"ok"`
		Store struct {
			Backend          string         `json:"backend"`
			Records          int            `json:"records"`
			CollectionCounts map[string]int `json:"collection_counts"`
		} `json:"store"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.Store.Backend != store.BackendJSON || out.Store.Records == 0 {
		t.Fatalf("store verify response = %#v", out)
	}
	if out.Store.CollectionCounts["clients"] == 0 || out.Store.CollectionCounts["store_meta"] == 0 {
		t.Fatalf("store verify collection counts = %#v", out.Store.CollectionCounts)
	}
}

func TestSetupRejectsCompletedAndWeakPassword(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	holder := config.NewHolder(&config.Config{
		Auth: config.Auth{
			FirstRun: false, CookieName: "sis_session",
			Users: []config.User{{Username: "admin", PasswordHash: hash}},
		},
	})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	doneReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", bytes.NewBufferString(`{"username":"admin","password":"secret123"}`))
	doneRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(doneRec, doneReq)
	if doneRec.Code != http.StatusConflict {
		t.Fatalf("completed setup status = %d body=%s", doneRec.Code, doneRec.Body.String())
	}

	firstRun := config.NewHolder(&config.Config{Auth: config.Auth{FirstRun: true, CookieName: "sis_session"}})
	firstRunServer := NewWithDeps(Options{
		Config: firstRun,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	weakReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", bytes.NewBufferString(`{"username":"admin","password":"short"}`))
	weakRec := httptest.NewRecorder()
	firstRunServer.Handler().ServeHTTP(weakRec, weakReq)
	if weakRec.Code != http.StatusBadRequest {
		t.Fatalf("weak setup status = %d body=%s", weakRec.Code, weakRec.Body.String())
	}
}

func TestLoginRejectsBadCredentialsAndMalformedJSON(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	holder := config.NewHolder(&config.Config{
		Auth: config.Auth{
			FirstRun: false, CookieName: "sis_session",
			Users: []config.User{{Username: "admin", PasswordHash: hash}},
		},
	})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"wrong-password"}`))
	badRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d body=%s", badRec.Code, badRec.Body.String())
	}
	malformedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{`))
	malformedRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(malformedRec, malformedReq)
	if malformedRec.Code != http.StatusBadRequest {
		t.Fatalf("malformed login status = %d body=%s", malformedRec.Code, malformedRec.Body.String())
	}
	unknownReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"secret123","role":"root"}`))
	unknownRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(unknownRec, unknownReq)
	if unknownRec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field login status = %d body=%s", unknownRec.Code, unknownRec.Body.String())
	}
	trailingReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"secret123"} {}`))
	trailingRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(trailingRec, trailingReq)
	if trailingRec.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON login status = %d body=%s", trailingRec.Code, trailingRec.Body.String())
	}
	largeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(bytes.Repeat([]byte(" "), maxJSONBodySize+1)))
	largeRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(largeRec, largeReq)
	if largeRec.Code != http.StatusBadRequest {
		t.Fatalf("large body login status = %d body=%s", largeRec.Code, largeRec.Body.String())
	}
}

func TestLoginFailsWhenSessionCannotBePersisted(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	holder := config.NewHolder(&config.Config{
		Auth: config.Auth{
			FirstRun: false, CookieName: "sis_session",
			Users: []config.User{{Username: "admin", PasswordHash: hash}},
		},
	})
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"secret123"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "store: closed") {
		t.Fatalf("login leaked internal error: %s", rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("unexpected cookies: %#v", rec.Result().Cookies())
	}
}

func TestLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	session := &store.Session{Token: "logout-token", Username: "admin", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.Sessions().Upsert(session); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "sis_session", Value: session.Token})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := st.Sessions().Get(session.Token); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("session should be deleted, err=%v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge != -1 || cookies[0].Value != "" {
		t.Fatalf("expected clearing cookie, got %#v", cookies)
	}
}

func TestLoginCookieSecureWhenTLSConfigured(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	holder := config.NewHolder(&config.Config{
		Server: config.Server{HTTP: config.HTTPServer{TLS: true}},
		Auth: config.Auth{
			FirstRun: false, CookieName: "sis_session",
			Users: []config.User{{Username: "admin", PasswordHash: hash}},
		},
	})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"secret123"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected secure cookie, got %#v", cookies)
	}
}

func TestLoginCookieSecureWhenConfiguredForProxy(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	holder := config.NewHolder(&config.Config{
		Auth: config.Auth{
			FirstRun: false, CookieName: "sis_session", SecureCookie: true,
			Users: []config.User{{Username: "admin", PasswordHash: hash}},
		},
	})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"secret123"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected secure cookie, got %#v", cookies)
	}
}

func TestNewTokenUsesURLSafeRandomBytes(t *testing.T) {
	token, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || strings.ContainsAny(token, "+/=") {
		t.Fatalf("unexpected token %q", token)
	}
}

func TestGroupsCreate(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", bytes.NewBufferString(`{"name":" iot ","blocklists":["ads"],"allowlist":[],"schedules":[]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := findConfigGroup(holder.Get().Groups, "iot"); !ok {
		t.Fatalf("group was not added: %#v", holder.Get().Groups)
	}
}

func TestGroupsCreateRejectsBlankName(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", bytes.NewBufferString(`{"name":"   ","blocklists":["ads"],"allowlist":[],"schedules":[]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGroupPatchUpdatesPolicyFields(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Blocklists = append(holder.Get().Blocklists, config.Blocklist{
		ID: "malware", URL: "file:///tmp/malware.txt", Enabled: true,
	})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	body := `{"name":"default","blocklists":["ads","malware"],"allowlist":["safe.example"],"schedules":[]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/default", bytes.NewBufferString(body))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	group, ok := findConfigGroup(holder.Get().Groups, "default")
	if !ok {
		t.Fatal("default group missing")
	}
	if strings.Join(group.Blocklists, ",") != "ads,malware" ||
		strings.Join(group.Allowlist, ",") != "safe.example" {
		t.Fatalf("group = %#v", group)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"blocklists":["ads","malware"]`)) {
		t.Fatalf("response should use JSON tags: %s", rec.Body.String())
	}
}

func TestGroupPatchPreservesOmittedFields(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Groups[0].Schedules = []config.Schedule{{
		Name: "school", Days: []string{"mon"}, From: "08:00", To: "16:00", Block: []string{"ads"},
	}}
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/default", bytes.NewBufferString(`{"allowlist":["safe.example"]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	group, ok := findConfigGroup(holder.Get().Groups, "default")
	if !ok {
		t.Fatal("default group missing")
	}
	if strings.Join(group.Blocklists, ",") != "ads" || strings.Join(group.Allowlist, ",") != "safe.example" ||
		len(group.Schedules) != 1 || group.Schedules[0].Name != "school" {
		t.Fatalf("group = %#v", group)
	}
}

func TestGroupPatchDefaultRenameFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/default", bytes.NewBufferString(`{"name":"renamed","blocklists":["ads"],"allowlist":[],"schedules":[]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGroupPatchDuplicateNameFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Groups = append(holder.Get().Groups, config.Group{Name: "iot"})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/iot", bytes.NewBufferString(`{"name":"default","blocklists":[],"allowlist":[],"schedules":[]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGroupPatchUnknownBlocklistFailsValidation(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/default", bytes.NewBufferString(`{"name":"default","blocklists":["missing"],"allowlist":[],"schedules":[]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGroupSchedulePatchAffectsQueryTest(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Groups = []config.Group{{Name: "default"}}
	holder.Get().Blocklists[0].Enabled = true
	engine, err := policy.NewEngine(holder.Get(), policy.StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	ads := policy.NewDomains()
	if !ads.Add("ads.example.com.") {
		t.Fatal("failed to add ads.example.com")
	}
	engine.ReplaceList("ads", ads)
	pipeline := sisdns.NewPipelineWithDeps(sisdns.PipelineOptions{
		Config: holder,
		Cache:  sisdns.NewCache(sisdns.CacheOptions{}),
		Policy: engine,
		Stats:  stats.New(),
	})
	s := NewWithDeps(Options{
		Config:   holder,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		Policy:   engine,
		Pipeline: pipeline,
	})

	body := `{"schedules":[{"name":"bedtime","days":["all"],"from":"00:00","to":"00:00","block":["ads"]}]}`
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/default", bytes.NewBufferString(body))
	addSessionCookie(t, st, patchReq)
	patchRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", patchRec.Code, patchRec.Body.String())
	}

	queryReq := httptest.NewRequest(http.MethodPost, "/api/v1/query/test", bytes.NewBufferString(`{"domain":"ads.example.com","type":"A","client_ip":"192.0.2.55"}`))
	addSessionCookie(t, st, queryReq)
	queryRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(queryRec, queryReq)
	if queryRec.Code != http.StatusOK {
		t.Fatalf("query status = %d body=%s", queryRec.Code, queryRec.Body.String())
	}
	if !bytes.Contains(queryRec.Body.Bytes(), []byte(`"source":"synthetic"`)) ||
		!bytes.Contains(queryRec.Body.Bytes(), []byte("0.0.0.0")) {
		t.Fatalf("query body = %s", queryRec.Body.String())
	}
}

func TestSettingsPatch(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", bytes.NewBufferString(`{"privacy":{"strip_ecs":false,"block_local_ptr":true,"log_mode":"hashed"}}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if holder.Get().Privacy.LogMode != "hashed" {
		t.Fatalf("privacy = %#v", holder.Get().Privacy)
	}
	if holder.Get().Privacy.LogSalt == "" {
		t.Fatal("expected generated log salt")
	}
	history, err := st.ConfigHistory().List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || !bytes.Contains([]byte(history[0].YAML), []byte("log_mode: hashed")) {
		t.Fatalf("expected config history snapshot, got %#v", history)
	}
	if bytes.Contains([]byte(history[0].YAML), []byte(holder.Get().Privacy.LogSalt)) {
		t.Fatalf("stored history leaked log salt: %s", history[0].YAML)
	}
	historyReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/config/history", nil)
	addSessionCookie(t, st, historyReq)
	historyRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history status = %d body=%s", historyRec.Code, historyRec.Body.String())
	}
	if bytes.Contains(historyRec.Body.Bytes(), []byte("password_hash: unused")) ||
		bytes.Contains(historyRec.Body.Bytes(), []byte(holder.Get().Privacy.LogSalt)) {
		t.Fatalf("history leaked secrets: %s", historyRec.Body.String())
	}
}

func TestSettingsGetUsesSnakeCaseJSON(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	cache, ok := payload["cache"].(map[string]any)
	if !ok {
		t.Fatalf("cache missing: %#v", payload)
	}
	if _, ok := cache["max_entries"]; !ok {
		t.Fatalf("expected max_entries JSON key, got %s", rec.Body.String())
	}
	if _, ok := cache["MaxEntries"]; ok {
		t.Fatalf("unexpected PascalCase JSON key: %s", rec.Body.String())
	}
}

func TestUpstreamCreate(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewBufferString(`{"id":" quad9 ","name":"Quad9","url":"https://dns.quad9.net/dns-query","bootstrap":["9.9.9.9"],"timeout":"3s"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamIndex(holder.Get().Upstreams, "quad9") < 0 {
		t.Fatalf("upstream was not added: %#v", holder.Get().Upstreams)
	}
}

func TestUpstreamCreateRejectsBlankID(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewBufferString(`{"id":"   ","url":"https://dns.quad9.net/dns-query","bootstrap":["9.9.9.9"]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpstreamPatch(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	body := `{"id":"cloudflare","name":"Cloudflare","url":"https://one.one.one.one/dns-query","bootstrap":["1.0.0.1"],"timeout":"5s"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/upstreams/cloudflare", bytes.NewBufferString(body))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	upstream := holder.Get().Upstreams[upstreamIndex(holder.Get().Upstreams, "cloudflare")]
	if upstream.Name != "Cloudflare" || upstream.URL != "https://one.one.one.one/dns-query" ||
		strings.Join(upstream.Bootstrap, ",") != "1.0.0.1" || upstream.Timeout.Duration != 5*time.Second {
		t.Fatalf("upstream = %#v", upstream)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"bootstrap":["1.0.0.1"]`)) {
		t.Fatalf("response should use JSON tags: %s", rec.Body.String())
	}
}

func TestUpstreamPatchPreservesOmittedFields(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Upstreams[0].Name = "Cloudflare"
	holder.Get().Upstreams[0].Timeout = config.Duration{Duration: 3 * time.Second}
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/upstreams/cloudflare", bytes.NewBufferString(`{"name":"Cloudflare DNS"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	upstream := holder.Get().Upstreams[upstreamIndex(holder.Get().Upstreams, "cloudflare")]
	if upstream.Name != "Cloudflare DNS" || upstream.URL != "https://cloudflare-dns.com/dns-query" ||
		strings.Join(upstream.Bootstrap, ",") != "1.1.1.1" || upstream.Timeout.Duration != 3*time.Second {
		t.Fatalf("upstream = %#v", upstream)
	}
}

func TestUpstreamPatchDuplicateIDFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Upstreams = append(holder.Get().Upstreams, config.Upstream{
		ID: "quad9", URL: "https://dns.quad9.net/dns-query", Bootstrap: []string{"9.9.9.9"},
	})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/upstreams/cloudflare", bytes.NewBufferString(`{"id":"quad9","url":"https://dns.quad9.net/dns-query","bootstrap":["9.9.9.9"]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpstreamPatchInvalidURLFailsValidation(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/upstreams/cloudflare", bytes.NewBufferString(`{"id":"cloudflare","url":"http://not-doh.example/dns-query","bootstrap":["1.1.1.1"]}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpstreamTestHidesRuntimeErrorDetails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config:   validAPIConfig(t),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		Upstream: upstream.NewPool(nil),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams/missing/test", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("missing")) {
		t.Fatalf("runtime detail leaked: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("upstream test failed")) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestBlocklistDeleteReferencedFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/blocklists/ads", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBlocklistCreateTrimsID(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocklists", bytes.NewBufferString(`{"id":" malware ","url":"file:///tmp/malware.txt","enabled":true}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if blocklistIndex(holder.Get().Blocklists, "malware") < 0 {
		t.Fatalf("blocklist was not added: %#v", holder.Get().Blocklists)
	}
}

func TestBlocklistCreateRejectsBlankID(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocklists", bytes.NewBufferString(`{"id":"   ","url":"file:///tmp/malware.txt","enabled":true}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBlocklistPatch(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	body := `{"id":"ads","name":"Ads","url":"file:///tmp/updated.txt","enabled":false,"refresh_interval":"12h"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/blocklists/ads", bytes.NewBufferString(body))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	blocklist := holder.Get().Blocklists[blocklistIndex(holder.Get().Blocklists, "ads")]
	if blocklist.Name != "Ads" || blocklist.URL != "file:///tmp/updated.txt" ||
		blocklist.Enabled || blocklist.RefreshInterval.Duration != 12*time.Hour {
		t.Fatalf("blocklist = %#v", blocklist)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"refresh_interval":"12h0m0s"`)) {
		t.Fatalf("response should use JSON tags: %s", rec.Body.String())
	}
}

func TestBlocklistPatchPreservesOmittedFields(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Blocklists[0].Name = "Ads"
	holder.Get().Blocklists[0].Enabled = true
	holder.Get().Blocklists[0].RefreshInterval = config.Duration{Duration: 24 * time.Hour}
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/blocklists/ads", bytes.NewBufferString(`{"name":"Advertising"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	blocklist := holder.Get().Blocklists[blocklistIndex(holder.Get().Blocklists, "ads")]
	if blocklist.Name != "Advertising" || blocklist.URL != "file:///tmp/ads.txt" ||
		!blocklist.Enabled || blocklist.RefreshInterval.Duration != 24*time.Hour {
		t.Fatalf("blocklist = %#v", blocklist)
	}
}

func TestBlocklistPatchDuplicateIDFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Blocklists = append(holder.Get().Blocklists, config.Blocklist{
		ID: "malware", URL: "file:///tmp/malware.txt", Enabled: true,
	})
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/blocklists/ads", bytes.NewBufferString(`{"id":"malware","url":"file:///tmp/malware.txt","enabled":true,"refresh_interval":"24h"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBlocklistPatchBadRefreshIntervalFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/blocklists/ads", bytes.NewBufferString(`{"id":"ads","url":"file:///tmp/ads.txt","enabled":true,"refresh_interval":"not-a-duration"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBlocklistSyncEndpointUpdatesPolicyEntries(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "ads.txt")
	if err := os.WriteFile(listPath, []byte("ads.example.com\ntracker.example.net\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	holder.Get().Blocklists[0].URL = "file://" + listPath
	holder.Get().Blocklists[0].Enabled = true
	engine, err := policy.NewEngine(holder.Get(), policy.StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	syncer := policy.NewSyncer(holder, policy.NewFetcher(filepath.Join(dir, "cache"), 0), engine, nil)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
		Policy: engine,
		Syncer: syncer,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocklists/ads/sync", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ID       string `json:"id"`
		Accepted int    `json:"accepted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ID != "ads" || payload.Accepted != 2 {
		t.Fatalf("sync payload = %#v", payload)
	}
	decision := engine.For(policy.Identity{Key: "client"}).Evaluate("ads.example.com.", 1, time.Now())
	if !decision.Blocked || decision.List != "ads" {
		t.Fatalf("decision = %#v", decision)
	}

	entriesReq := httptest.NewRequest(http.MethodGet, "/api/v1/blocklists/ads/entries?q=tracker&limit=10", nil)
	addSessionCookie(t, st, entriesReq)
	entriesRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(entriesRec, entriesReq)
	if entriesRec.Code != http.StatusOK {
		t.Fatalf("entries status = %d body=%s", entriesRec.Code, entriesRec.Body.String())
	}
	if !bytes.Contains(entriesRec.Body.Bytes(), []byte("tracker.example.net")) {
		t.Fatalf("entries body = %s", entriesRec.Body.String())
	}
}

func TestQueryLogList(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	queryLog, err := sislog.OpenQuery(&config.Config{
		Server:  config.Server{DataDir: t.TempDir()},
		Privacy: config.Privacy{LogMode: "full"},
		Logging: config.Logging{QueryLog: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := queryLog.Write(&sislog.Entry{ClientKey: "c1", QName: "blocked.example.", Blocked: true}); err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config:   validAPIConfig(t),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		QueryLog: queryLog,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?blocked=true&qname=blocked", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("blocked.example.")) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestQueryLogListInvalidLimit(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	queryLog, err := sislog.OpenQuery(&config.Config{
		Server:  config.Server{DataDir: t.TempDir()},
		Privacy: config.Privacy{LogMode: "full"},
		Logging: config.Logging{QueryLog: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config:   validAPIConfig(t),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		QueryLog: queryLog,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?limit=-1", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsTimeseries(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Stats().Put("1m", "1", &store.StatsRow{Counters: map[string]uint64{"queries": 2}}); err != nil {
		t.Fatal(err)
	}
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/timeseries?bucket=1m", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"queries":2`)) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestStatsTimeseriesInvalidBucket(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/timeseries?bucket=bad", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsTopDomainsInvalidBlocked(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
		Stats:  stats.New(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/top-domains?blocked=maybe", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsTopDomains(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	counters := stats.New()
	counters.AddDomain("example.com.", false)
	s := NewWithDeps(Options{
		Config: validAPIConfig(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
		Stats:  counters,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/top-domains", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("example.com.")) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestQueryTest(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	counters := stats.New()
	pipeline := sisdns.NewPipelineWithDeps(sisdns.PipelineOptions{
		Config: holder,
		Cache:  sisdns.NewCache(sisdns.CacheOptions{}),
		Stats:  counters,
	})
	s := NewWithDeps(Options{
		Config:   holder,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		Stats:    counters,
		Pipeline: pipeline,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query/test", bytes.NewBufferString(`{"domain":"localhost","type":"A","client_ip":"192.168.1.50"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"source":"local"`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte("127.0.0.1")) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestQueryTestRejectsInvalidDomain(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	pipeline := sisdns.NewPipelineWithDeps(sisdns.PipelineOptions{
		Config: holder,
		Cache:  sisdns.NewCache(sisdns.CacheOptions{}),
	})
	s := NewWithDeps(Options{
		Config:   holder,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    st,
		Pipeline: pipeline,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query/test", bytes.NewBufferString(`{"domain":"bad domain","type":"A"}`))
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBlocklistEntries(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	holder := validAPIConfig(t)
	engine, err := policy.NewEngine(holder.Get(), policy.StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	domains, _, err := policy.ParseBlocklist(strings.NewReader("ads.example.com\ntracker.example.net\n"))
	if err != nil {
		t.Fatal(err)
	}
	engine.ReplaceList("ads", domains)
	s := NewWithDeps(Options{
		Config: holder,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  st,
		Policy: engine,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocklists/ads/entries?q=example.com&limit=10", nil)
	addSessionCookie(t, st, req)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("ads.example.com")) ||
		bytes.Contains(rec.Body.Bytes(), []byte("tracker.example.net")) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func testHolder() *config.Holder {
	return validAPIConfig(nil)
}

func addSessionCookie(t *testing.T, st store.Store, req *http.Request) {
	t.Helper()
	session := &store.Session{Token: "test-token", Username: "admin", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.Sessions().Upsert(session); err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "sis_session", Value: session.Token})
}

func validAPIConfig(t *testing.T) *config.Holder {
	dataDir := "./data"
	if t != nil {
		dataDir = t.TempDir()
	}
	return config.NewHolder(&config.Config{
		Server: config.Server{DataDir: dataDir, TZ: "Local"},
		Cache: config.Cache{
			MinTTL: config.Duration{Duration: time.Minute},
			MaxTTL: config.Duration{Duration: time.Hour},
		},
		Privacy: config.Privacy{StripECS: true, BlockLocalPTR: true, LogMode: "full"},
		Upstreams: []config.Upstream{{
			ID: "cloudflare", URL: "https://cloudflare-dns.com/dns-query",
			Bootstrap: []string{"1.1.1.1"},
		}},
		Blocklists: []config.Blocklist{{ID: "ads", URL: "file:///tmp/ads.txt"}},
		Groups:     []config.Group{{Name: "default", Blocklists: []string{"ads"}}},
		Auth: config.Auth{
			FirstRun:   false,
			Users:      []config.User{{Username: "admin", PasswordHash: "unused"}},
			CookieName: "sis_session",
		},
	})
}

func findConfigGroup(groups []config.Group, name string) (config.Group, bool) {
	for _, group := range groups {
		if group.Name == name {
			return group, true
		}
	}
	return config.Group{}, false
}
