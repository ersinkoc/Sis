package stats

import (
	"sync/atomic"
	"time"
)

var histogramBounds = [...]time.Duration{
	time.Microsecond,
	2 * time.Microsecond,
	4 * time.Microsecond,
	8 * time.Microsecond,
	16 * time.Microsecond,
	32 * time.Microsecond,
	64 * time.Microsecond,
	128 * time.Microsecond,
	256 * time.Microsecond,
	512 * time.Microsecond,
	time.Millisecond,
	2 * time.Millisecond,
	4 * time.Millisecond,
	8 * time.Millisecond,
	16 * time.Millisecond,
	32 * time.Millisecond,
	64 * time.Millisecond,
	128 * time.Millisecond,
	256 * time.Millisecond,
	512 * time.Millisecond,
	time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	10 * time.Second,
}

// Histogram records latency samples in fixed duration buckets.
type Histogram struct {
	buckets [len(histogramBounds) + 1]atomic.Uint64
	total   atomic.Uint64
}

// HistogramSnapshot exposes approximate latency quantiles.
type HistogramSnapshot struct {
	Count uint64        `json:"count"`
	P50   time.Duration `json:"p50"`
	P95   time.Duration `json:"p95"`
	P99   time.Duration `json:"p99"`
}

// NewHistogram creates an empty latency histogram.
func NewHistogram() *Histogram {
	return &Histogram{}
}

// Observe records one latency sample.
func (h *Histogram) Observe(d time.Duration) {
	idx := len(histogramBounds)
	for i, bound := range histogramBounds {
		if d <= bound {
			idx = i
			break
		}
	}
	h.buckets[idx].Add(1)
	h.total.Add(1)
}

// Snapshot returns current sample count and approximate quantiles.
func (h *Histogram) Snapshot() HistogramSnapshot {
	count := h.total.Load()
	return HistogramSnapshot{
		Count: count,
		P50:   h.quantile(count, 0.50),
		P95:   h.quantile(count, 0.95),
		P99:   h.quantile(count, 0.99),
	}
}

func (h *Histogram) quantile(count uint64, q float64) time.Duration {
	if count == 0 {
		return 0
	}
	target := uint64(float64(count) * q)
	if target == 0 {
		target = 1
	}
	var seen uint64
	for i := range h.buckets {
		seen += h.buckets[i].Load()
		if seen >= target {
			if i >= len(histogramBounds) {
				return histogramBounds[len(histogramBounds)-1]
			}
			return histogramBounds[i]
		}
	}
	return histogramBounds[len(histogramBounds)-1]
}
