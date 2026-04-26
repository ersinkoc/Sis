package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

func TestProbeUnhealthyRecoversClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mdns.Msg
		if err := req.Unpack(mustReadAll(t, r)); err != nil {
			t.Fatal(err)
		}
		resp := new(mdns.Msg)
		resp.SetReply(&req)
		wire, err := resp.Pack()
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wire)
	}))
	defer server.Close()

	pool := NewPool([]config.Upstream{{ID: "test", URL: server.URL}})
	pool.markFailure("test")
	pool.markFailure("test")
	pool.markFailure("test")
	if len(pool.HealthyIDs()) != 0 {
		t.Fatalf("expected unhealthy upstream, got healthy ids %#v", pool.HealthyIDs())
	}
	pool.ProbeUnhealthy(context.Background())
	if got := pool.HealthyIDs(); len(got) != 1 || got[0] != "test" {
		t.Fatalf("expected recovered upstream, got %#v", got)
	}
}

func TestPoolIDsAndHealth(t *testing.T) {
	pool := NewPool([]config.Upstream{{ID: "one"}, {ID: "two"}})
	if got := pool.AllIDs(); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("all ids = %#v", got)
	}
	if !pool.IsHealthy("one") {
		t.Fatal("one should start healthy")
	}
	pool.markFailure("one")
	pool.markFailure("one")
	pool.markFailure("one")
	if pool.IsHealthy("one") {
		t.Fatal("one should be unhealthy after failures")
	}
}

func TestForwardReportsAttempts(t *testing.T) {
	var firstCalls atomic.Int64
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls.Add(1)
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mdns.Msg
		if err := req.Unpack(mustReadAll(t, r)); err != nil {
			t.Fatal(err)
		}
		resp := new(mdns.Msg)
		resp.SetReply(&req)
		wire, err := resp.Pack()
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wire)
	}))
	defer good.Close()

	pool := NewPool([]config.Upstream{{ID: "bad", URL: bad.URL}, {ID: "good", URL: good.URL}})
	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)
	resp, id, attempts, err := pool.Forward(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || id != "good" {
		t.Fatalf("response id = %q resp=%#v", id, resp)
	}
	if len(attempts) != 2 || attempts[0].ID != "bad" || attempts[0].OK || attempts[1].ID != "good" || !attempts[1].OK {
		t.Fatalf("attempts = %#v", attempts)
	}
	if firstCalls.Load() != 1 {
		t.Fatalf("bad upstream calls = %d", firstCalls.Load())
	}
}

func TestPoolRejectsNilInputs(t *testing.T) {
	var pool *Pool
	if _, _, _, err := pool.Forward(context.Background(), new(mdns.Msg)); err == nil {
		t.Fatal("expected nil pool error")
	}
	if _, err := pool.Test(context.Background(), "missing"); err == nil {
		t.Fatal("expected nil pool test error")
	}
	if got := pool.AllIDs(); got != nil {
		t.Fatalf("all ids = %#v", got)
	}
	if got := pool.HealthyIDs(); got != nil {
		t.Fatalf("healthy ids = %#v", got)
	}
	if pool.IsHealthy("missing") {
		t.Fatal("nil pool should not report healthy upstreams")
	}

	pool = NewPool(nil)
	if _, _, _, err := pool.Forward(context.Background(), nil); err == nil {
		t.Fatal("expected nil message error")
	}
}
