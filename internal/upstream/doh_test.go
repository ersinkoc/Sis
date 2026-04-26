package upstream

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

func TestDoHClientForward(t *testing.T) {
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

	client := NewDoHClient(config.Upstream{ID: "test", URL: server.URL})
	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)
	resp, err := client.Forward(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Id != msg.Id || resp.Rcode != mdns.RcodeSuccess {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestDoHClientBootstrapDial(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "doh.test" {
			t.Fatalf("host = %q", r.Host)
		}
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
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client := NewDoHClient(config.Upstream{
		ID: "bootstrap", URL: "https://doh.test:" + port + "/dns-query",
		Bootstrap: []string{"127.0.0.1"},
	})
	transport := client.client.Transport.(*http.Transport)
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "doh.test",
		MinVersion:         tls.VersionTLS12,
	}

	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)
	resp, err := client.Forward(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Id != msg.Id {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestDoHClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(make([]byte, maxDNSMessageSize+1))
	}))
	defer server.Close()

	client := NewDoHClient(config.Upstream{ID: "test", URL: server.URL})
	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)
	if _, err := client.Forward(context.Background(), msg); err == nil {
		t.Fatal("expected oversized response error")
	}
}

func TestDoHClientRejectsUnexpectedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not dns"))
	}))
	defer server.Close()

	client := NewDoHClient(config.Upstream{ID: "test", URL: server.URL})
	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)
	if _, err := client.Forward(context.Background(), msg); err == nil {
		t.Fatal("expected content type error")
	}
}

func TestDNSMessageContentTypeAllowsParameters(t *testing.T) {
	if !isDNSMessageContent("application/dns-message; charset=binary") {
		t.Fatal("expected content type with parameters to be accepted")
	}
}

func TestDoHHost(t *testing.T) {
	if got := dohHost("https://cloudflare-dns.com/dns-query"); got != "cloudflare-dns.com" {
		t.Fatalf("host = %q", got)
	}
	if got := dohHost("://bad"); got != "" {
		t.Fatalf("bad host = %q", got)
	}
}

func mustReadAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	defer r.Body.Close()
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf
}
