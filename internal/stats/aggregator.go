package stats

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/ersinkoc/sis/internal/store"
)

type Aggregator struct {
	counters *Counters
	store    store.StatsStore
	now      func() time.Time
	last     Snapshot
	hasLast  bool
}

func NewAggregator(counters *Counters, statsStore store.StatsStore) *Aggregator {
	return &Aggregator{counters: counters, store: statsStore, now: time.Now}
}

func (a *Aggregator) Run(ctx context.Context) {
	if a == nil || a.counters == nil || a.store == nil {
		return
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

func (a *Aggregator) Flush() error {
	snap := a.counters.Snapshot()
	bucket := a.now().UTC().Truncate(time.Minute).Unix()
	queries := snap.QueryTotal
	cacheHit := snap.CacheHit
	cacheMiss := snap.CacheMiss
	blocked := snap.BlockedTotal
	if a.hasLast {
		queries -= a.last.QueryTotal
		cacheHit -= a.last.CacheHit
		cacheMiss -= a.last.CacheMiss
		blocked -= a.last.BlockedTotal
	}
	a.last = snap
	a.hasLast = true
	row := &store.StatsRow{Counters: map[string]uint64{
		"queries":    queries,
		"cache_hit":  cacheHit,
		"cache_miss": cacheMiss,
		"blocked":    blocked,
	}}
	if err := a.store.Put("1m", strconv.FormatInt(bucket, 10), row); err != nil {
		return err
	}
	if err := a.addRollup("1h", a.now().UTC().Truncate(time.Hour).Unix(), row.Counters); err != nil {
		return err
	}
	return a.addRollup("1d", a.now().UTC().Truncate(24*time.Hour).Unix(), row.Counters)
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
