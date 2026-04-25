package policy

import (
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

func TestScheduleActiveAtSameDay(t *testing.T) {
	s := mustSchedule(t, config.Schedule{
		Name: "school", Days: []string{"weekday"}, From: "08:00", To: "15:00",
		Block: []string{"social"},
	})
	loc := time.FixedZone("test", 0)
	if !s.ActiveAt(time.Date(2026, 4, 20, 9, 0, 0, 0, loc), loc) {
		t.Fatal("expected Monday 09:00 active")
	}
	if s.ActiveAt(time.Date(2026, 4, 20, 16, 0, 0, 0, loc), loc) {
		t.Fatal("expected Monday 16:00 inactive")
	}
	if s.ActiveAt(time.Date(2026, 4, 19, 9, 0, 0, 0, loc), loc) {
		t.Fatal("expected Sunday inactive")
	}
}

func TestScheduleActiveAtCrossMidnight(t *testing.T) {
	s := mustSchedule(t, config.Schedule{
		Name: "bedtime", Days: []string{"fri"}, From: "22:00", To: "07:00",
		Block: []string{"video"},
	})
	loc := time.FixedZone("test", 0)
	if !s.ActiveAt(time.Date(2026, 4, 24, 23, 0, 0, 0, loc), loc) {
		t.Fatal("expected Friday 23:00 active")
	}
	if !s.ActiveAt(time.Date(2026, 4, 25, 6, 30, 0, 0, loc), loc) {
		t.Fatal("expected Saturday 06:30 active from Friday window")
	}
	if s.ActiveAt(time.Date(2026, 4, 25, 8, 0, 0, 0, loc), loc) {
		t.Fatal("expected Saturday 08:00 inactive")
	}
}

func mustSchedule(t *testing.T, raw config.Schedule) CompiledSchedule {
	t.Helper()
	s, err := CompileSchedule(raw)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
