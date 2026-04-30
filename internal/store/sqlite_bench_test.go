package store

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkSQLiteClientUpsert(b *testing.B) {
	s := benchmarkSQLiteStore(b)
	defer s.Close()
	clients := s.Clients()
	now := time.Now().UTC()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := clients.Upsert(&Client{
			Key:       fmt.Sprintf("192.0.2.%d", i%254+1),
			Type:      "ip",
			Name:      "bench-client",
			Group:     "default",
			FirstSeen: now,
			LastSeen:  now,
			LastIP:    fmt.Sprintf("192.0.2.%d", i%254+1),
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSQLiteClientGet(b *testing.B) {
	s := benchmarkSQLiteStore(b)
	defer s.Close()
	clients := s.Clients()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("192.0.2.%d", i)
		if err := clients.Upsert(&Client{Key: key, Type: "ip", Group: "default", LastIP: key}); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := clients.Get(fmt.Sprintf("192.0.2.%d", i%1000)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSQLiteSessionUpsert(b *testing.B) {
	s := benchmarkSQLiteStore(b)
	defer s.Close()
	sessions := s.Sessions()
	expires := time.Now().UTC().Add(time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sessions.Upsert(&Session{
			Token:     fmt.Sprintf("token-%08d", i),
			Username:  "admin",
			ExpiresAt: expires,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSQLiteStatsPutGet(b *testing.B) {
	s := benchmarkSQLiteStore(b)
	defer s.Close()
	stats := s.Stats()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket := fmt.Sprintf("%d", i%1000)
		if err := stats.Put("1m", bucket, &StatsRow{Counters: map[string]uint64{
			"query_total":   uint64(i),
			"blocked_total": uint64(i / 10),
		}}); err != nil {
			b.Fatal(err)
		}
		if _, err := stats.Get("1m", bucket); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSQLiteConfigHistoryAppendList(b *testing.B) {
	s := benchmarkSQLiteStore(b)
	defer s.Close()
	history := s.ConfigHistory()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := history.Append(&ConfigSnapshot{
			TS:   time.Now().UTC(),
			YAML: fmt.Sprintf("server:\n  data_dir: /tmp/sis-%d\n", i),
		}); err != nil {
			b.Fatal(err)
		}
		if _, err := history.List(10); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkSQLiteStore(b *testing.B) Store {
	b.Helper()
	s, err := OpenSQLite(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	return s
}
