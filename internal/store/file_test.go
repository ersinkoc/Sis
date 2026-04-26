package store

import (
	"errors"
	"testing"
	"time"
)

func TestStoreCRUD(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	client := &Client{Key: "192.168.1.10", Type: "ip", Group: "default"}
	if err := s.Clients().Upsert(client); err != nil {
		t.Fatal(err)
	}
	got, err := s.Clients().Get(client.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Group != "default" {
		t.Fatalf("group = %q", got.Group)
	}
	clients, err := s.Clients().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 {
		t.Fatalf("clients len = %d", len(clients))
	}
	if err := s.Clients().Delete(client.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Clients().Get(client.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get deleted err = %v", err)
	}
}

func TestCustomListSessionStatsAndConfigHistory(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.CustomLists().Add("custom", "example.com"); err != nil {
		t.Fatal(err)
	}
	domains, err := s.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "example.com" {
		t.Fatalf("domains = %#v", domains)
	}

	session := &Session{Token: "tok", Username: "admin", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.Sessions().Upsert(session); err != nil {
		t.Fatal(err)
	}
	gotSession, err := s.Sessions().Get("tok")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.Username != "admin" {
		t.Fatalf("username = %q", gotSession.Username)
	}

	row := &StatsRow{Counters: map[string]uint64{"queries": 42}}
	if err := s.Stats().Put("1m", "123", row); err != nil {
		t.Fatal(err)
	}
	gotRow, err := s.Stats().Get("1m", "123")
	if err != nil {
		t.Fatal(err)
	}
	if gotRow.Counters["queries"] != 42 {
		t.Fatalf("queries = %d", gotRow.Counters["queries"])
	}
	if err := s.Stats().Put("1m", "20", &StatsRow{Counters: map[string]uint64{"queries": 20}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Stats().Put("1m", "100", &StatsRow{Counters: map[string]uint64{"queries": 100}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Stats().List("1m")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Bucket != "20" || rows[1].Bucket != "100" || rows[2].Bucket != "123" {
		t.Fatalf("rows not numerically sorted: %#v", rows)
	}

	if err := s.ConfigHistory().Append(&ConfigSnapshot{YAML: "server: {}"}); err != nil {
		t.Fatal(err)
	}
	history, err := s.ConfigHistory().List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].YAML == "" {
		t.Fatalf("history = %#v", history)
	}
}

func TestStatsPutDoesNotMutateInputRow(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	row := &StatsRow{Bucket: "caller-owned", Counters: map[string]uint64{"queries": 7}}
	if err := s.Stats().Put("1m", "123", row); err != nil {
		t.Fatal(err)
	}
	if row.Bucket != "caller-owned" {
		t.Fatalf("input row bucket mutated to %q", row.Bucket)
	}
	got, err := s.Stats().Get("1m", "123")
	if err != nil {
		t.Fatal(err)
	}
	if got.Bucket != "123" {
		t.Fatalf("stored bucket = %q", got.Bucket)
	}
}

func TestConfigHistoryAppendDoesNotMutateInputSnapshot(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	snapshot := &ConfigSnapshot{YAML: "server: {}"}
	if err := s.ConfigHistory().Append(snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.TS.IsZero() {
		t.Fatalf("input snapshot timestamp mutated to %s", snapshot.TS)
	}
	history, err := s.ConfigHistory().List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].TS.IsZero() {
		t.Fatalf("history = %#v", history)
	}
}

func TestSessionDeleteExpired(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	expired := &Session{Token: "expired", Username: "admin", ExpiresAt: time.Now().Add(-time.Minute)}
	active := &Session{Token: "active", Username: "admin", ExpiresAt: time.Now().Add(time.Minute)}
	if err := s.Sessions().Upsert(expired); err != nil {
		t.Fatal(err)
	}
	if err := s.Sessions().Upsert(active); err != nil {
		t.Fatal(err)
	}
	if err := s.Sessions().DeleteExpired(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sessions().Get("expired"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session err = %v", err)
	}
	if _, err := s.Sessions().Get("active"); err != nil {
		t.Fatalf("active session err = %v", err)
	}
}

func TestStoreWritesAfterCloseFail(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Clients().Upsert(&Client{Key: "192.168.1.10"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("upsert after close err = %v", err)
	}
	if err := s.CustomLists().Add("custom", "example.com"); !errors.Is(err, ErrClosed) {
		t.Fatalf("custom list add after close err = %v", err)
	}
}
