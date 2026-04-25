package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterPrunesExpiredEntries(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	limiter.entries["expired"] = rateEntry{count: 1, reset: time.Now().Add(-time.Minute)}
	limiter.nextPrune = time.Now().Add(-time.Second)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	if !limiter.allow(req) {
		t.Fatal("first request should be allowed")
	}
	if _, ok := limiter.entries["expired"]; ok {
		t.Fatal("expired entry was not pruned")
	}
}
