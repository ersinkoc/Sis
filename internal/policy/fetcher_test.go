package policy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetcherHTTPNotModifiedLoadsCache(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprintln(w, "ads.example.com")
	}))
	defer server.Close()

	fetcher := NewFetcher(t.TempDir())
	first, err := fetcher.Fetch(context.Background(), "ads", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if first.FromCache || !first.Domains.Match("ads.example.com") {
		t.Fatalf("unexpected first result: %#v", first)
	}
	second, err := fetcher.Fetch(context.Background(), "ads", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !second.FromCache || !second.NotModified {
		t.Fatalf("expected not-modified cache result: %#v", second)
	}
	if hits != 2 {
		t.Fatalf("hits = %d", hits)
	}
}

func TestFetcherFileURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.txt")
	if err := os.WriteFile(path, []byte("0.0.0.0 file.example.com\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	fetcher := NewFetcher(filepath.Join(dir, "cache"))
	result, err := fetcher.Fetch(context.Background(), "local", "file://"+path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Domains.Match("file.example.com") {
		t.Fatal("expected parsed file domain")
	}
}
