package dns

import (
	"net"
	"sort"
	"sync"
	"time"
)

const defaultRateLimiterMaxBuckets = 10000

type RateLimiter struct {
	mu         sync.Mutex
	qps        float64
	burst      float64
	maxBuckets int
	buckets    map[string]*rateBucket
	now        func() time.Time
	nextPrune  time.Time
}

type rateBucket struct {
	tokens float64
	seen   time.Time
}

func NewRateLimiter(qps, burst int) *RateLimiter {
	if qps <= 0 || burst <= 0 {
		return nil
	}
	return &RateLimiter{
		qps:        qpsToFloat(qps),
		burst:      qpsToFloat(burst),
		maxBuckets: defaultRateLimiterMaxBuckets,
		buckets:    make(map[string]*rateBucket),
		now:        time.Now,
	}
}

func qpsToFloat(v int) float64 {
	return float64(v)
}

func (l *RateLimiter) Allow(ip net.IP) bool {
	if l == nil {
		return true
	}
	key := "unknown"
	if ip != nil {
		key = ip.String()
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.nextPrune.IsZero() || now.After(l.nextPrune) {
		l.pruneLocked(now)
		l.nextPrune = now.Add(time.Minute)
	}
	bucket := l.buckets[key]
	if bucket == nil {
		bucket = &rateBucket{tokens: l.burst, seen: now}
		l.buckets[key] = bucket
		l.trimLocked(key)
	}
	elapsed := now.Sub(bucket.seen).Seconds()
	bucket.seen = now
	bucket.tokens += elapsed * l.qps
	if bucket.tokens > l.burst {
		bucket.tokens = l.burst
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (l *RateLimiter) pruneLocked(now time.Time) {
	for key, bucket := range l.buckets {
		if now.Sub(bucket.seen) > 5*time.Minute {
			delete(l.buckets, key)
		}
	}
	l.trimLocked("")
}

func (l *RateLimiter) trimLocked(exempt string) {
	if l.maxBuckets <= 0 || len(l.buckets) <= l.maxBuckets {
		return
	}
	keys := make([]string, 0, len(l.buckets))
	for key := range l.buckets {
		if key != exempt {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return l.buckets[keys[i]].seen.Before(l.buckets[keys[j]].seen)
	})
	for _, key := range keys {
		if len(l.buckets) <= l.maxBuckets {
			return
		}
		delete(l.buckets, key)
	}
}
