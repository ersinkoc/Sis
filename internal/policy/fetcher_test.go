package policy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestFetcherRejectsNilAndMissingInputs(t *testing.T) {
	var fetcher *Fetcher
	if _, err := fetcher.Fetch(context.Background(), "ads", "https://example.test/list.txt"); err == nil {
		t.Fatal("expected nil fetcher error")
	}
	fetcher = NewFetcher(t.TempDir())
	if _, err := fetcher.Fetch(context.Background(), "", "https://example.test/list.txt"); err == nil {
		t.Fatal("expected missing id error")
	}
}

func TestFetcherAcceptsNilContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.txt")
	if err := os.WriteFile(path, []byte("nil-context.example.com\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	fetcher := NewFetcher(filepath.Join(dir, "cache"))
	result, err := fetcher.Fetch(nil, "local", "file://"+path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Domains.Match("nil-context.example.com") {
		t.Fatal("expected parsed nil-context domain")
	}
}

func TestFetcherCacheFilesUseRestrictedModeWithoutTempLeftovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.txt")
	if err := os.WriteFile(path, []byte("0.0.0.0 file.example.com\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	fetcher := NewFetcher(filepath.Join(dir, "cache"))
	if _, err := fetcher.Fetch(context.Background(), "local", "file://"+path); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fetcher.cachePath("local"), fetcher.metaPath("local")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("%s mode = %o, want 640", path, got)
		}
	}
	entries, err := os.ReadDir(fetcher.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("unexpected temp file %q", entry.Name())
		}
	}
}

func TestFetcherRejectsOversizedHTTPBlocklist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, maxBlocklistBytes+1))
	}))
	defer server.Close()

	fetcher := NewFetcher(t.TempDir())
	if _, err := fetcher.Fetch(context.Background(), "ads", server.URL); err == nil {
		t.Fatal("expected oversized blocklist error")
	}
}

func TestFetcherRejectsOversizedFileBlocklist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.txt")
	if err := os.WriteFile(path, make([]byte, maxBlocklistBytes+1), 0o640); err != nil {
		t.Fatal(err)
	}
	fetcher := NewFetcher(filepath.Join(dir, "cache"))
	if _, err := fetcher.Fetch(context.Background(), "local", "file://"+path); err == nil {
		t.Fatal("expected oversized file blocklist error")
	}
}
