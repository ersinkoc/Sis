package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxBlocklistBytes = 32 << 20

// Fetcher downloads blocklists and maintains an on-disk cache.
type Fetcher struct {
	Client   *http.Client
	CacheDir string
}

// FetchResult is the parsed output of one blocklist fetch.
type FetchResult struct {
	ID        string
	Domains   *Domains
	Stats     ParseStats
	FromCache bool
	NotModified bool
	FetchedAt time.Time
}

type cacheMeta struct {
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// NewFetcher creates a blocklist fetcher using cacheDir for content and metadata.
func NewFetcher(cacheDir string) *Fetcher {
	return &Fetcher{
		Client:   &http.Client{Timeout: 30 * time.Second},
		CacheDir: cacheDir,
	}
}

// Fetch retrieves and parses a blocklist, falling back to cached content on errors.
func (f *Fetcher) Fetch(ctx context.Context, id, rawURL string) (*FetchResult, error) {
	if id == "" {
		return nil, fmt.Errorf("blocklist id is required")
	}
	if f.Client == nil {
		f.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if f.CacheDir == "" {
		f.CacheDir = "."
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		if cached, cacheErr := f.loadCache(id); cacheErr == nil {
			return cached, nil
		}
		return nil, err
	}
	if parsed.Scheme == "file" {
		return f.fetchFile(id, parsed)
	}
	result, err := f.fetchHTTP(ctx, id, rawURL)
	if err == nil {
		return result, nil
	}
	if cached, cacheErr := f.loadCache(id); cacheErr == nil {
		return cached, nil
	}
	return nil, err
}

func (f *Fetcher) fetchHTTP(ctx context.Context, id, rawURL string) (*FetchResult, error) {
	meta, _ := f.loadMeta(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if meta.ETag != "" {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	if meta.LastModified != "" {
		req.Header.Set("If-Modified-Since", meta.LastModified)
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		cached, err := f.loadCache(id)
		if err != nil {
			return nil, err
		}
		cached.NotModified = true
		return cached, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: HTTP %d", rawURL, resp.StatusCode)
	}
	raw, err := readLimited(resp.Body, maxBlocklistBytes)
	if err != nil {
		return nil, err
	}
	result, err := f.parseAndCache(id, raw, cacheMeta{
		ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"),
		FetchedAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (f *Fetcher) fetchFile(id string, parsed *url.URL) (*FetchResult, error) {
	path := parsed.Path
	if path == "" {
		path = strings.TrimPrefix(parsed.Opaque, "file://")
	}
	file, err := os.Open(path)
	if err != nil {
		if cached, cacheErr := f.loadCache(id); cacheErr == nil {
			return cached, nil
		}
		return nil, err
	}
	defer file.Close()
	raw, err := readLimited(file, maxBlocklistBytes)
	if err != nil {
		if cached, cacheErr := f.loadCache(id); cacheErr == nil {
			return cached, nil
		}
		return nil, err
	}
	return f.parseAndCache(id, raw, cacheMeta{FetchedAt: time.Now().UTC()})
}

func (f *Fetcher) parseAndCache(id string, raw []byte, meta cacheMeta) (*FetchResult, error) {
	if err := os.MkdirAll(f.CacheDir, 0o755); err != nil {
		return nil, err
	}
	domains, stats, err := ParseBlocklist(strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(f.cachePath(id), raw, 0o640); err != nil {
		return nil, err
	}
	metaRaw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(f.metaPath(id), metaRaw, 0o640); err != nil {
		return nil, err
	}
	return &FetchResult{ID: id, Domains: domains, Stats: stats, FetchedAt: meta.FetchedAt}, nil
}

func (f *Fetcher) loadCache(id string) (*FetchResult, error) {
	raw, err := os.ReadFile(f.cachePath(id))
	if err != nil {
		return nil, err
	}
	domains, stats, err := ParseBlocklist(strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	meta, _ := f.loadMeta(id)
	return &FetchResult{ID: id, Domains: domains, Stats: stats, FromCache: true, FetchedAt: meta.FetchedAt}, nil
}

func (f *Fetcher) loadMeta(id string) (cacheMeta, error) {
	var meta cacheMeta
	raw, err := os.ReadFile(f.metaPath(id))
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(raw, &meta)
}

func (f *Fetcher) cachePath(id string) string {
	return filepath.Join(f.CacheDir, safeID(id)+".txt")
}

func (f *Fetcher) metaPath(id string) string {
	return filepath.Join(f.CacheDir, safeID(id)+".json")
}

func safeID(id string) string {
	id = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, id)
	if id == "" || id == "." || id == ".." {
		return "list"
	}
	return id
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("blocklist exceeds %d bytes", limit)
	}
	return raw, nil
}
