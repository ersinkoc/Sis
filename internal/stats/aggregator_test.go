package stats

import (
	"errors"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/store"
)

func TestAggregatorFlush(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	counters := New()
	counters.IncQuery()
	counters.IncBlocked()
	agg := NewAggregator(counters, st.Stats())
	agg.now = func() time.Time {
		return time.Unix(120, 0).UTC()
	}
	if err := agg.Flush(); err != nil {
		t.Fatal(err)
	}
	row, err := st.Stats().Get("1m", "120")
	if err != nil {
		t.Fatal(err)
	}
	if row.Counters["queries"] != 1 || row.Counters["blocked"] != 1 {
		t.Fatalf("row = %#v", row)
	}
	hour, err := st.Stats().Get("1h", "0")
	if err != nil {
		t.Fatal(err)
	}
	day, err := st.Stats().Get("1d", "0")
	if err != nil {
		t.Fatal(err)
	}
	if hour.Counters["queries"] != 1 || day.Counters["blocked"] != 1 {
		t.Fatalf("unexpected rollups: hour=%#v day=%#v", hour, day)
	}
	counters.IncQuery()
	if err := agg.Flush(); err != nil {
		t.Fatal(err)
	}
	row, err = st.Stats().Get("1m", "120")
	if err != nil {
		t.Fatal(err)
	}
	if row.Counters["queries"] != 1 || row.Counters["blocked"] != 0 {
		t.Fatalf("second row should be a delta: %#v", row)
	}
	hour, err = st.Stats().Get("1h", "0")
	if err != nil {
		t.Fatal(err)
	}
	if hour.Counters["queries"] != 2 || hour.Counters["blocked"] != 1 {
		t.Fatalf("hour rollup did not accumulate deltas: %#v", hour)
	}
}

func TestAggregatorFlushKeepsDeltaAfterStoreError(t *testing.T) {
	counters := New()
	counters.IncQuery()
	st := &flakyStatsStore{failPut: true, rows: make(map[string]*store.StatsRow)}
	agg := NewAggregator(counters, st)
	agg.now = func() time.Time { return time.Unix(60, 0).UTC() }
	if err := agg.Flush(); err == nil {
		t.Fatal("expected flush error")
	}
	st.failPut = false
	if err := agg.Flush(); err != nil {
		t.Fatal(err)
	}
	row, err := st.Get("1m", "60")
	if err != nil {
		t.Fatal(err)
	}
	if row.Counters["queries"] != 1 {
		t.Fatalf("queries = %d, want retry to preserve delta", row.Counters["queries"])
	}
}

func TestAggregatorFlushUsesSingleTimestampForRollups(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	counters := New()
	counters.IncQuery()
	agg := NewAggregator(counters, st.Stats())
	calls := 0
	agg.now = func() time.Time {
		calls++
		if calls > 1 {
			return time.Unix(3600, 0).UTC()
		}
		return time.Unix(3599, 0).UTC()
	}
	if err := agg.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Stats().Get("1h", "0"); err != nil {
		t.Fatalf("expected hour bucket from first timestamp: %v", err)
	}
	if _, err := st.Stats().Get("1h", "3600"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unexpected later hour bucket err = %v", err)
	}
}

type flakyStatsStore struct {
	failPut bool
	rows    map[string]*store.StatsRow
}

func (s *flakyStatsStore) Put(granularity, bucket string, row *store.StatsRow) error {
	if s.failPut {
		return errors.New("put failed")
	}
	cp := &store.StatsRow{Bucket: bucket, Counters: make(map[string]uint64)}
	for key, value := range row.Counters {
		cp.Counters[key] = value
	}
	s.rows[granularity+":"+bucket] = cp
	return nil
}

func (s *flakyStatsStore) Get(granularity, bucket string) (*store.StatsRow, error) {
	row := s.rows[granularity+":"+bucket]
	if row == nil {
		return nil, store.ErrNotFound
	}
	return row, nil
}

func (s *flakyStatsStore) List(string) ([]*store.StatsRow, error) {
	return nil, nil
}
