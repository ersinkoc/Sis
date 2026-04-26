package log

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Rotator writes to a file with size-based rotation, retention, and optional gzip.
type Rotator struct {
	path       string
	maxBytes   int64
	retainDays int
	gzip       bool
	cur        *os.File
	curSize    int64
}

// NewRotator opens path and configures size and retention policies.
func NewRotator(path string, maxBytes int64, retainDays int, gzipRotated bool) (*Rotator, error) {
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024
	}
	if retainDays <= 0 {
		retainDays = 7
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Rotator{
		path: path, maxBytes: maxBytes, retainDays: retainDays,
		gzip: gzipRotated, cur: f, curSize: info.Size(),
	}, nil
}

// Write appends p, rotating first when the configured size would be exceeded.
func (r *Rotator) Write(p []byte) (int, error) {
	if r.curSize > 0 && r.curSize+int64(len(p)) > r.maxBytes {
		if err := r.Rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.cur.Write(p)
	r.curSize += int64(n)
	return n, err
}

// Rotate closes the active file, renames it with a timestamp, and opens a new file.
func (r *Rotator) Rotate() error {
	if r.cur == nil {
		return nil
	}
	if err := r.cur.Close(); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	rotated := fmt.Sprintf("%s.%s", r.path, stamp)
	if err := os.Rename(r.path, rotated); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	r.cur = f
	r.curSize = 0
	if r.gzip {
		go gzipFile(rotated)
	}
	go r.EvictOld()
	return nil
}

// EvictOld removes rotated log files older than the retention window.
func (r *Rotator) EvictOld() {
	cutoff := time.Now().Add(-time.Duration(r.retainDays) * 24 * time.Hour)
	entries, err := os.ReadDir(filepath.Dir(r.path))
	if err != nil {
		return
	}
	base := filepath.Base(r.path) + "."
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		full := filepath.Join(filepath.Dir(r.path), name)
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(full)
		}
	}
}

// Close closes the active file.
func (r *Rotator) Close() error {
	if r.cur == nil {
		return nil
	}
	err := r.cur.Close()
	r.cur = nil
	return err
}

func gzipFile(path string) {
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer in.Close()
	outPath := path + ".gz"
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return
	}
	gz := gzip.NewWriter(out)
	_, copyErr := io.Copy(gz, in)
	closeErr := gz.Close()
	fileErr := out.Close()
	if copyErr == nil && closeErr == nil && fileErr == nil {
		_ = os.Remove(path)
	}
}
