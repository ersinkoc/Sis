package stats

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/ersinkoc/sis/internal/store"
)

// Aggregator periodically flushes live counter deltas into persistent rollups.
type Aggregator struct {
	counters *Counters
	store    store.StatsStore
	now      func() time.Time
	last     Snapshot
	hasLast  bool
}

// NewAggregator creates an Aggregator for counters and statsStore.
func NewAggregator(counters *Counters, statsStore store.StatsStore) *Aggregator {
	return &Aggregator{counters: counters, store: statsStore, now: time.Now}
}

// Run flushes stats once per minute until ctx is canceled.
func (a *Aggregator) Run(ctx context.Context) {
	if a == nil || a.counters == nil || a.store == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = a.Flush()
		}
	}
}

// Flush writes one minute bucket and updates hourly and daily rollups.
func (a *Aggregator) Flush() error {
	if a == nil || a.counters == nil || a.store == nil {
		return errors.New("stats aggregator is not configured")
	}
	snap := a.counters.Snapshot()
	if a.now == nil {
		a.now = time.Now
	}
	now := a.now().UTC()
	bucket := now.Truncate(time.Minute).Unix()
	queries := snap.QueryTotal
	cacheHit := snap.CacheHit
	cacheMiss := snap.CacheMiss
	blocked := snap.BlockedTotal
	rateLimited := snap.RateLimitedTotal
	if a.hasLast {
		queries = deltaCounter(snap.QueryTotal, a.last.QueryTotal)
		cacheHit = deltaCounter(snap.CacheHit, a.last.CacheHit)
		cacheMiss = deltaCounter(snap.CacheMiss, a.last.CacheMiss)
		blocked = deltaCounter(snap.BlockedTotal, a.last.BlockedTotal)
		rateLimited = deltaCounter(snap.RateLimitedTotal, a.last.RateLimitedTotal)
	}
	row := &store.StatsRow{Counters: map[string]uint64{
		"queries":      queries,
		"cache_hit":    cacheHit,
		"cache_miss":   cacheMiss,
		"blocked":      blocked,
		"rate_limited": rateLimited,
	}}
	if err := a.store.Put("1m", strconv.FormatInt(bucket, 10), row); err != nil {
		return err
	}
	if err := a.addRollup("1h", now.Truncate(time.Hour).Unix(), row.Counters); err != nil {
		return err
	}
	if err := a.addRollup("1d", now.Truncate(24*time.Hour).Unix(), row.Counters); err != nil {
		return err
	}
	a.last = snap
	a.hasLast = true
	return nil
}

func deltaCounter(current, previous uint64) uint64 {
	if current < previous {
		return current
	}
	return current - previous
}

func (a *Aggregator) addRollup(granularity string, bucket int64, delta map[string]uint64) error {
	key := strconv.FormatInt(bucket, 10)
	row, err := a.store.Get(granularity, key)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		row = &store.StatsRow{Counters: map[string]uint64{}}
	}
	if row.Counters == nil {
		row.Counters = map[string]uint64{}
	}
	for name, value := range delta {
		row.Counters[name] += value
	}
	return a.store.Put(granularity, key, row)
}
