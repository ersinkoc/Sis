package dns

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	sislog "github.com/ersinkoc/sis/internal/log"
	"github.com/ersinkoc/sis/internal/policy"
	"github.com/ersinkoc/sis/internal/stats"
	"github.com/ersinkoc/sis/internal/upstream"
	mdns "github.com/miekg/dns"
)

func TestSpec19DNSAcceptanceCorePath(t *testing.T) {
	var primaryHits atomic.Int64
	primary := fakeDoH(t, http.StatusOK, net.ParseIP("203.0.113.10"), &primaryHits)
	defer primary.Close()

	cfg := acceptanceConfig(t, []config.Upstream{{
		ID: "primary", URL: primary.URL, Bootstrap: []string{"127.0.0.1"}, Timeout: config.Duration{Duration: time.Second},
	}})
	counters := stats.New()
	queryLog, err := sislog.OpenQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer queryLog.Close()
	engine, err := policy.NewEngine(cfg, policy.StaticClientResolver{})
	if err != nil {
		t.Fatal(err)
	}
	ads := policy.NewDomains()
	if !ads.Add("ads.example.") {
		t.Fatal("failed to add ads.example")
	}
	engine.ReplaceList("ads", ads)

	server := startAcceptanceServer(t, cfg, counters, queryLog, engine, upstream.NewPool(cfg.Upstreams))
	udpResp := exchangeAcceptanceDNS(t, "udp", server.udpAddr(), "clean.example.", mdns.TypeA)
	assertAAnswer(t, udpResp, "203.0.113.10")

	blocked := exchangeAcceptanceDNS(t, "tcp", server.tcpAddr(), "ads.example.", mdns.TypeA)
	assertAAnswer(t, blocked, "0.0.0.0")
	if snap := counters.Snapshot(); snap.QueryTotal != 2 || snap.BlockedTotal != 1 {
		t.Fatalf("unexpected counters after clean/block queries: %#v", snap)
	}
	if primaryHits.Load() != 1 {
		t.Fatalf("primary upstream hits = %d, want 1", primaryHits.Load())
	}

	cfg.Allowlist.Domains = []string{"ads.example."}
	if err := engine.ReloadConfig(cfg); err != nil {
		t.Fatal(err)
	}
	allowed := exchangeAcceptanceDNS(t, "udp", server.udpAddr(), "ads.example.", mdns.TypeA)
	assertAAnswer(t, allowed, "203.0.113.10")
	if entries := queryLog.Recent(sislog.Filter{QName: "ads.example", Limit: 5}); len(entries) == 0 || entries[len(entries)-1].Blocked {
		t.Fatalf("allowlisted query was not logged as allowed: %#v", entries)
	}
}

func TestSpec19DNSAcceptanceFailoverAndCacheHit(t *testing.T) {
	var failingHits atomic.Int64
	failing := fakeDoH(t, http.StatusInternalServerError, nil, &failingHits)
	defer failing.Close()
	var backupHits atomic.Int64
	backup := fakeDoH(t, http.StatusOK, net.ParseIP("198.51.100.25"), &backupHits)
	defer backup.Close()

	cfg := acceptanceConfig(t, []config.Upstream{
		{ID: "primary", URL: failing.URL, Bootstrap: []string{"127.0.0.1"}, Timeout: config.Duration{Duration: time.Second}},
		{ID: "backup", URL: backup.URL, Bootstrap: []string{"127.0.0.1"}, Timeout: config.Duration{Duration: time.Second}},
	})
	counters := stats.New()
	queryLog, err := sislog.OpenQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer queryLog.Close()

	pool := upstream.NewPool(cfg.Upstreams)
	server := startAcceptanceServer(t, cfg, counters, queryLog, nil, pool)
	for _, domain := range []string{"fail-one.example.", "fail-two.example.", "fail-three.example."} {
		resp := exchangeAcceptanceDNS(t, "udp", server.udpAddr(), domain, mdns.TypeA)
		assertAAnswer(t, resp, "198.51.100.25")
	}
	if failingHits.Load() != 3 || backupHits.Load() != 3 {
		t.Fatalf("upstream hits = primary %d backup %d, want 3/3", failingHits.Load(), backupHits.Load())
	}
	if pool.IsHealthy("primary") {
		t.Fatal("primary upstream should be unhealthy after repeated 5xx failures")
	}

	backupBeforeCache := backupHits.Load()
	resp := exchangeAcceptanceDNS(t, "udp", server.udpAddr(), "fail-three.example.", mdns.TypeA)
	assertAAnswer(t, resp, "198.51.100.25")
	if backupHits.Load() != backupBeforeCache {
		t.Fatalf("cached query reached upstream: before=%d after=%d", backupBeforeCache, backupHits.Load())
	}
	if entries := queryLog.Recent(sislog.Filter{QName: "fail-three.example", Limit: 5}); len(entries) == 0 || !entries[len(entries)-1].CacheHit {
		t.Fatalf("cached query was not logged as cache hit: %#v", entries)
	}
	if snap := counters.Snapshot(); snap.CacheHit != 1 || snap.Upstreams["primary"].Errors != 3 || !snap.Upstreams["backup"].Healthy {
		t.Fatalf("unexpected stats after failover/cache: %#v", snap)
	}
}

func TestSpec19DNSAcceptanceScheduleActiveInactive(t *testing.T) {
	var upstreamHits atomic.Int64
	doh := fakeDoH(t, http.StatusOK, net.ParseIP("203.0.113.30"), &upstreamHits)
	defer doh.Close()

	cfg := acceptanceConfig(t, []config.Upstream{{
		ID: "primary", URL: doh.URL, Bootstrap: []string{"127.0.0.1"}, Timeout: config.Duration{Duration: time.Second},
	}})
	cfg.Groups = []config.Group{
		{Name: "default"},
		{
			Name: "kids",
			Schedules: []config.Schedule{{
				Name: "bedtime", Days: []string{"all"}, From: "00:00", To: "00:00", Block: []string{"social"},
			}},
		},
		{
			Name: "after-school",
			Schedules: []config.Schedule{{
				Name: "inactive", Days: []string{tomorrowToken()}, From: "00:00", To: "00:00", Block: []string{"social"},
			}},
		},
	}
	counters := stats.New()
	queryLog, err := sislog.OpenQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer queryLog.Close()
	engine, err := policy.NewEngine(cfg, policy.StaticClientResolver{"127.0.0.1": "kids"})
	if err != nil {
		t.Fatal(err)
	}
	social := policy.NewDomains()
	if !social.Add("tiktok.com.") {
		t.Fatal("failed to add tiktok.com")
	}
	engine.ReplaceList("social", social)

	server := startAcceptanceServer(t, cfg, counters, queryLog, engine, upstream.NewPool(cfg.Upstreams))
	blocked := exchangeAcceptanceDNS(t, "udp", server.udpAddr(), "tiktok.com.", mdns.TypeA)
	assertAAnswer(t, blocked, "0.0.0.0")
	entries := queryLog.Recent(sislog.Filter{QName: "tiktok.com", Limit: 5})
	if len(entries) == 0 || !entries[len(entries)-1].Blocked ||
		entries[len(entries)-1].BlockReason != "schedule:bedtime" || entries[len(entries)-1].BlockList != "social" {
		t.Fatalf("active schedule query log = %#v", entries)
	}

	engine, err = policy.NewEngine(cfg, policy.StaticClientResolver{"127.0.0.1": "after-school"})
	if err != nil {
		t.Fatal(err)
	}
	engine.ReplaceList("social", social)
	secondServer := startAcceptanceServer(t, cfg, counters, queryLog, engine, upstream.NewPool(cfg.Upstreams))
	allowed := exchangeAcceptanceDNS(t, "udp", secondServer.udpAddr(), "tiktok.com.", mdns.TypeA)
	assertAAnswer(t, allowed, "203.0.113.30")
	entries = queryLog.Recent(sislog.Filter{QName: "tiktok.com", Limit: 5})
	if len(entries) == 0 || entries[len(entries)-1].Blocked {
		t.Fatalf("inactive schedule query log = %#v", entries)
	}
}

func TestSpec19DNSAcceptancePrivacyModeHashesClientIdentity(t *testing.T) {
	var upstreamHits atomic.Int64
	doh := fakeDoH(t, http.StatusOK, net.ParseIP("203.0.113.40"), &upstreamHits)
	defer doh.Close()

	cfg := acceptanceConfig(t, []config.Upstream{{
		ID: "primary", URL: doh.URL, Bootstrap: []string{"127.0.0.1"}, Timeout: config.Duration{Duration: time.Second},
	}})
	cfg.Privacy.LogMode = "hashed"
	cfg.Privacy.LogSalt = "acceptance-salt"
	counters := stats.New()
	queryLog, err := sislog.OpenQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer queryLog.Close()

	server := startAcceptanceServer(t, cfg, counters, queryLog, nil, upstream.NewPool(cfg.Upstreams))
	resp := exchangeAcceptanceDNS(t, "udp", server.udpAddr(), "privacy.example.", mdns.TypeA)
	assertAAnswer(t, resp, "203.0.113.40")
	entries := queryLog.Recent(sislog.Filter{QName: "privacy.example", Limit: 5})
	if len(entries) == 0 {
		t.Fatal("expected privacy query log entry")
	}
	entry := entries[len(entries)-1]
	if entry.ClientKey == "" || entry.ClientKey == "127.0.0.1" {
		t.Fatalf("client key was not hashed: %#v", entry)
	}
	if entry.ClientIP == "" || entry.ClientIP == "127.0.0.1" {
		t.Fatalf("client IP was not hashed: %#v", entry)
	}
}

type acceptanceServer struct {
	server *Server
}

func startAcceptanceServer(t *testing.T, cfg *config.Config, counters *stats.Counters, queryLog *sislog.Query, engine *policy.Engine, pool *upstream.Pool) *acceptanceServer {
	t.Helper()
	holder := config.NewHolder(cfg)
	pipeline := NewPipelineWithDeps(PipelineOptions{
		Config: holder, Cache: NewCache(CacheOptions{}), Policy: engine,
		Upstream: pool, Log: queryLog, Stats: counters,
	})
	server := NewServer(holder, pipeline)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("shutdown dns server: %v", err)
		}
	})
	return &acceptanceServer{server: server}
}

func (s *acceptanceServer) udpAddr() string {
	if s == nil || s.server == nil || len(s.server.udpConns) == 0 {
		return ""
	}
	return s.server.udpConns[0].LocalAddr().String()
}

func (s *acceptanceServer) tcpAddr() string {
	if s == nil || s.server == nil || len(s.server.tcpLns) == 0 {
		return ""
	}
	return s.server.tcpLns[0].Addr().String()
}

func acceptanceConfig(t *testing.T, upstreams []config.Upstream) *config.Config {
	t.Helper()
	return &config.Config{
		Server: config.Server{
			DNS:     config.DNSServer{Listen: []string{"127.0.0.1:0"}, UDPSize: 1232},
			DataDir: t.TempDir(), TZ: "Local",
		},
		Cache: config.Cache{
			MinTTL: config.Duration{Duration: time.Minute},
			MaxTTL: config.Duration{Duration: time.Hour},
		},
		Privacy: config.Privacy{StripECS: true, BlockLocalPTR: true, LogMode: "full"},
		Logging: config.Logging{QueryLog: false},
		Block: config.Block{
			ResponseA: "0.0.0.0", ResponseAAAA: "::", ResponseTTL: config.Duration{Duration: time.Minute},
		},
		Upstreams:  upstreams,
		Blocklists: []config.Blocklist{{ID: "ads", URL: "file:///tmp/ads.txt", Enabled: true}},
		Groups:     []config.Group{{Name: "default", Blocklists: []string{"ads"}}},
		Auth:       config.Auth{FirstRun: true, CookieName: "sis_session"},
	}
}

func fakeDoH(t *testing.T, status int, answer net.IP, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if status < 200 || status >= 300 {
			http.Error(w, "upstream failed", status)
			return
		}
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read dns message: %v", err)
			http.Error(w, "bad dns", http.StatusBadRequest)
			return
		}
		var req mdns.Msg
		if err := req.Unpack(body); err != nil {
			t.Errorf("unpack upstream request: %v", err)
			http.Error(w, "bad dns", http.StatusBadRequest)
			return
		}
		resp := new(mdns.Msg)
		resp.SetReply(&req)
		resp.RecursionAvailable = true
		if len(req.Question) > 0 && req.Question[0].Qtype == mdns.TypeA && answer != nil {
			resp.Answer = append(resp.Answer, &mdns.A{
				Hdr: rrHeader(req.Question[0].Name, mdns.TypeA, 60),
				A:   answer.To4(),
			})
		}
		wire, err := resp.Pack()
		if err != nil {
			t.Errorf("pack upstream response: %v", err)
			http.Error(w, "bad response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wire)
	}))
}

func exchangeAcceptanceDNS(t *testing.T, network, addr, name string, qtype uint16) *mdns.Msg {
	t.Helper()
	msg := new(mdns.Msg)
	msg.SetQuestion(name, qtype)
	client := &mdns.Client{Net: network, Timeout: 2 * time.Second}
	resp, _, err := client.Exchange(msg, addr)
	if err != nil {
		t.Fatalf("dns exchange %s %s: %v", network, name, err)
	}
	if resp == nil {
		t.Fatalf("nil dns response for %s", name)
	}
	return resp
}

func assertAAnswer(t *testing.T, msg *mdns.Msg, want string) {
	t.Helper()
	if msg.Rcode != mdns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", mdns.RcodeToString[msg.Rcode])
	}
	for _, rr := range msg.Answer {
		if a, ok := rr.(*mdns.A); ok && a.A.String() == want {
			return
		}
	}
	t.Fatalf("A answer %s not found in %#v", want, msg.Answer)
}

func tomorrowToken() string {
	switch time.Now().AddDate(0, 0, 1).Weekday() {
	case time.Sunday:
		return "sun"
	case time.Monday:
		return "mon"
	case time.Tuesday:
		return "tue"
	case time.Wednesday:
		return "wed"
	case time.Thursday:
		return "thu"
	case time.Friday:
		return "fri"
	default:
		return "sat"
	}
}
