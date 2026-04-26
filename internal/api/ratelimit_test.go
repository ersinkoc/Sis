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

func TestRateLimiterDisabledWithNonPositiveLimitOrWindow(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	for _, limiter := range []*rateLimiter{
		newRateLimiter(0, time.Minute),
		newRateLimiter(1, 0),
	} {
		for i := 0; i < 3; i++ {
			if !limiter.allow(req) {
				t.Fatalf("limiter should be disabled: %#v", limiter)
			}
		}
	}
}
