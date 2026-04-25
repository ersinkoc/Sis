package stats

import (
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
