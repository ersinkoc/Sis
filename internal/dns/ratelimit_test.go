package dns

import (
	"net"
	"testing"
	"time"
)

func TestRateLimiterAllowsBurstAndRefill(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewRateLimiter(2, 2)
	limiter.now = func() time.Time { return now }
	ip := net.ParseIP("192.0.2.10")

	if !limiter.Allow(ip) || !limiter.Allow(ip) {
		t.Fatal("burst should allow first two requests")
	}
	if limiter.Allow(ip) {
		t.Fatal("third request should be limited")
	}
	now = now.Add(time.Second)
	if !limiter.Allow(ip) || !limiter.Allow(ip) {
		t.Fatal("refill should allow two more requests")
	}
}

func TestRateLimiterPrunesIdleBuckets(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewRateLimiter(1, 1)
	limiter.now = func() time.Time { return now }
	limiter.buckets["old"] = &rateBucket{tokens: 0, seen: now.Add(-10 * time.Minute)}
	limiter.nextPrune = now.Add(-time.Second)

	if !limiter.Allow(net.ParseIP("192.0.2.11")) {
		t.Fatal("request should be allowed")
	}
	if _, ok := limiter.buckets["old"]; ok {
		t.Fatal("old bucket was not pruned")
	}
}

func TestRateLimiterCapsBucketCount(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewRateLimiter(10, 10)
	limiter.now = func() time.Time { return now }
	limiter.maxBuckets = 2
	limiter.buckets["oldest"] = &rateBucket{tokens: 1, seen: now.Add(-3 * time.Minute)}
	limiter.buckets["newer"] = &rateBucket{tokens: 1, seen: now.Add(-time.Minute)}

	if !limiter.Allow(net.ParseIP("192.0.2.12")) {
		t.Fatal("request should be allowed")
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2", len(limiter.buckets))
	}
	if _, ok := limiter.buckets["oldest"]; ok {
		t.Fatal("oldest bucket was not evicted")
	}
	if _, ok := limiter.buckets["192.0.2.12"]; !ok {
		t.Fatal("current bucket was evicted")
	}
}
