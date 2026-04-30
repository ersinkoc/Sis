package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

func BenchmarkDoHClientForward(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mdns.Msg
		body, err := io.ReadAll(r.Body)
		if err != nil {
			b.Fatal(err)
		}
		if err := req.Unpack(body); err != nil {
			b.Fatal(err)
		}
		resp := new(mdns.Msg)
		resp.SetReply(&req)
		resp.RecursionAvailable = true
		wire, err := resp.Pack()
		if err != nil {
			b.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(wire)
	}))
	defer server.Close()
	client := NewDoHClient(config.Upstream{ID: "bench", URL: server.URL})
	msg := new(mdns.Msg)
	msg.SetQuestion("example.com.", mdns.TypeA)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.Id = uint16(i)
		resp, err := client.Forward(context.Background(), msg)
		if err != nil {
			b.Fatal(err)
		}
		if resp.Id != msg.Id {
			b.Fatalf("response id = %d, want %d", resp.Id, msg.Id)
		}
	}
}
