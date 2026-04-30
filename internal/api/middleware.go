package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type requestIDKey struct{}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur", time.Since(start), "request_id", requestIDFromContext(r.Context()))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func apiErrorEnvelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/api/v1/logs/query/stream" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &bufferedResponse{header: make(http.Header), status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status < http.StatusBadRequest || !isPlainText(rec.header.Get("Content-Type")) {
			copyHeader(w.Header(), rec.header)
			w.WriteHeader(rec.status)
			_, _ = w.Write(rec.body.Bytes())
			return
		}
		copyHeader(w.Header(), rec.header)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Del("Content-Length")
		w.WriteHeader(rec.status)
		msg := strings.TrimSpace(rec.body.String())
		if msg == "" {
			msg = http.StatusText(rec.status)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":      msg,
			"request_id": requestIDFromContext(r.Context()),
		})
	})
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *bufferedResponse) Header() http.Header {
	return r.header
}

func (r *bufferedResponse) WriteHeader(status int) {
	r.status = status
}

func (r *bufferedResponse) Write(data []byte) (int, error) {
	return r.body.Write(data)
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isPlainText(contentType string) bool {
	return contentType == "" || strings.HasPrefix(contentType, "text/plain")
}

func requestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
