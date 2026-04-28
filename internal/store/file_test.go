package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestOpenBackend(t *testing.T) {
	s, err := OpenBackend(BackendJSON, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenBackend(BackendSQLite, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenBackend("postgres", t.TempDir()); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("OpenBackend postgres err = %v", err)
	}
}

func TestSQLiteStoreCRUDAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenSQLite(dir)
	if err != nil {
		t.Fatal(err)
	}

	client := &Client{Key: "192.168.1.30", Type: "ip", Group: "default"}
	if err := s.Clients().Upsert(client); err != nil {
		t.Fatal(err)
	}
	if err := s.CustomLists().Add("custom", "sqlite.example"); err != nil {
		t.Fatal(err)
	}
	session := &Session{Token: "tok", Username: "admin", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.Sessions().Upsert(session); err != nil {
		t.Fatal(err)
	}
	if err := s.Stats().Put("1m", "20", &StatsRow{Counters: map[string]uint64{"queries": 20}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfigHistory().Append(&ConfigSnapshot{YAML: "server: {}"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "sis.db")); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSQLite(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	gotClient, err := reopened.Clients().Get(client.Key)
	if err != nil {
		t.Fatal(err)
	}
	if gotClient.Group != "default" {
		t.Fatalf("client group = %q", gotClient.Group)
	}
	sqlite, ok := reopened.(*sqliteStore)
	if !ok {
		t.Fatalf("reopened sqlite store type = %T", reopened)
	}
	var tableGroup string
	if err := sqlite.db.QueryRow(`SELECT client_group FROM clients WHERE key = ?`, client.Key).Scan(&tableGroup); err != nil {
		t.Fatal(err)
	}
	if tableGroup != "default" {
		t.Fatalf("client table group = %q", tableGroup)
	}
	domains, err := reopened.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "sqlite.example" {
		t.Fatalf("domains = %#v", domains)
	}
	var customListRows int
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM custom_lists WHERE list_id = ? AND domain = ?`, "custom", "sqlite.example").Scan(&customListRows); err != nil {
		t.Fatal(err)
	}
	if customListRows != 1 {
		t.Fatalf("custom list table rows = %d", customListRows)
	}
	if err := reopened.CustomLists().Remove("custom", "sqlite.example"); err != nil {
		t.Fatal(err)
	}
	domains, err = reopened.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 0 {
		t.Fatalf("removed domains = %#v", domains)
	}
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM custom_lists WHERE list_id = ? AND domain = ?`, "custom", "sqlite.example").Scan(&customListRows); err != nil {
		t.Fatal(err)
	}
	if customListRows != 0 {
		t.Fatalf("deleted custom list remained in table: %d", customListRows)
	}
	var customListKV int
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM kv WHERE key = ?`, "customlist:custom:sqlite.example").Scan(&customListKV); err != nil {
		t.Fatal(err)
	}
	if customListKV != 0 {
		t.Fatalf("deleted custom list remained in kv: %d", customListKV)
	}
	gotSession, err := reopened.Sessions().Get("tok")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.Username != "admin" {
		t.Fatalf("username = %q", gotSession.Username)
	}
	var tableSessionUser string
	if err := sqlite.db.QueryRow(`SELECT username FROM sessions WHERE token = ?`, "tok").Scan(&tableSessionUser); err != nil {
		t.Fatal(err)
	}
	if tableSessionUser != "admin" {
		t.Fatalf("session table username = %q", tableSessionUser)
	}
	expired := &Session{Token: "expired", Username: "admin", ExpiresAt: time.Now().Add(-time.Hour)}
	if err := reopened.Sessions().Upsert(expired); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Sessions().DeleteExpired(); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Sessions().Get("expired"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session err = %v", err)
	}
	var expiredKV int
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM kv WHERE key = ?`, "session:expired").Scan(&expiredKV); err != nil {
		t.Fatal(err)
	}
	if expiredKV != 0 {
		t.Fatalf("expired session remained in kv: %d", expiredKV)
	}
	if err := reopened.Sessions().Delete("tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Sessions().Get("tok"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session err = %v", err)
	}
	var sessionRows int
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE token = ?`, "tok").Scan(&sessionRows); err != nil {
		t.Fatal(err)
	}
	if sessionRows != 0 {
		t.Fatalf("deleted session remained in table: %d", sessionRows)
	}
	var sessionKV int
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM kv WHERE key = ?`, "session:tok").Scan(&sessionKV); err != nil {
		t.Fatal(err)
	}
	if sessionKV != 0 {
		t.Fatalf("deleted session remained in kv: %d", sessionKV)
	}
	rows, err := reopened.Stats().List("1m")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Counters["queries"] != 20 {
		t.Fatalf("rows = %#v", rows)
	}
	var tableStatsCounters []byte
	if err := sqlite.db.QueryRow(`SELECT counters FROM stats WHERE granularity = ? AND bucket = ?`, "1m", "20").Scan(&tableStatsCounters); err != nil {
		t.Fatal(err)
	}
	var tableStats map[string]uint64
	if err := json.Unmarshal(tableStatsCounters, &tableStats); err != nil {
		t.Fatal(err)
	}
	if tableStats["queries"] != 20 {
		t.Fatalf("stats table counters = %#v", tableStats)
	}
	history, err := reopened.ConfigHistory().List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].YAML == "" {
		t.Fatalf("history = %#v", history)
	}
	var tableHistoryYAML string
	if err := sqlite.db.QueryRow(`SELECT yaml FROM config_history WHERE ts = ?`, sqliteTime(history[0].TS)).Scan(&tableHistoryYAML); err != nil {
		t.Fatal(err)
	}
	if tableHistoryYAML != "server: {}" {
		t.Fatalf("config history table yaml = %q", tableHistoryYAML)
	}
	if err := reopened.Clients().Delete(client.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Clients().Get(client.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted client err = %v", err)
	}
	var tableClients int
	if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM clients WHERE key = ?`, client.Key).Scan(&tableClients); err != nil {
		t.Fatal(err)
	}
	if tableClients != 0 {
		t.Fatalf("deleted client remained in table: %d", tableClients)
	}
}

func TestSQLiteMigrationAddsCollectionColumn(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "sis.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE kv (key TEXT PRIMARY KEY, value BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	rawClient, err := json.Marshal(&Client{Key: "192.0.2.31", Type: "ip", Group: "default"})
	if err != nil {
		t.Fatal(err)
	}
	rawSession, err := json.Marshal(&Session{Token: "tok", Username: "admin", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "clients:192.0.2.31", rawClient); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "session:tok", rawSession); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "customlist:custom:legacy.example", []byte(`true`)); err != nil {
		t.Fatal(err)
	}
	rawStats, err := json.Marshal(&StatsRow{Counters: map[string]uint64{"queries": 9}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "stats:1m:30", rawStats); err != nil {
		t.Fatal(err)
	}
	historyTS := time.Date(2026, 4, 28, 12, 30, 0, 0, time.UTC)
	rawHistory, err := json.Marshal(&ConfigSnapshot{TS: historyTS, YAML: "server:\n  listen: :53\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "confhist:"+historyTS.Format(time.RFC3339Nano), rawHistory); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "store_meta:schema_version", []byte(`1`)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenSQLite(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = sql.Open("sqlite", filepath.Join(dir, "sis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hasCollection, err := sqliteHasColumn(db, "kv", "collection")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCollection {
		t.Fatal("sqlite migration did not add collection column")
	}
	var collection string
	if err := db.QueryRow(`SELECT collection FROM kv WHERE key = ?`, "clients:192.0.2.31").Scan(&collection); err != nil {
		t.Fatal(err)
	}
	if collection != "clients" {
		t.Fatalf("collection = %q", collection)
	}
	var tableGroup string
	if err := db.QueryRow(`SELECT client_group FROM clients WHERE key = ?`, "192.0.2.31").Scan(&tableGroup); err != nil {
		t.Fatal(err)
	}
	if tableGroup != "default" {
		t.Fatalf("client table group = %q", tableGroup)
	}
	var tableSessionUser string
	if err := db.QueryRow(`SELECT username FROM sessions WHERE token = ?`, "tok").Scan(&tableSessionUser); err != nil {
		t.Fatal(err)
	}
	if tableSessionUser != "admin" {
		t.Fatalf("session table username = %q", tableSessionUser)
	}
	var tableCustomList int
	if err := db.QueryRow(`SELECT COUNT(*) FROM custom_lists WHERE list_id = ? AND domain = ?`, "custom", "legacy.example").Scan(&tableCustomList); err != nil {
		t.Fatal(err)
	}
	if tableCustomList != 1 {
		t.Fatalf("custom list table rows = %d", tableCustomList)
	}
	var statsCounters []byte
	if err := db.QueryRow(`SELECT counters FROM stats WHERE granularity = ? AND bucket = ?`, "1m", "30").Scan(&statsCounters); err != nil {
		t.Fatal(err)
	}
	var statsRow map[string]uint64
	if err := json.Unmarshal(statsCounters, &statsRow); err != nil {
		t.Fatal(err)
	}
	if statsRow["queries"] != 9 {
		t.Fatalf("stats table counters = %#v", statsRow)
	}
	var historyYAML string
	if err := db.QueryRow(`SELECT yaml FROM config_history WHERE ts = ?`, sqliteTime(historyTS)).Scan(&historyYAML); err != nil {
		t.Fatal(err)
	}
	if historyYAML == "" || !strings.Contains(historyYAML, "listen") {
		t.Fatalf("config history table yaml = %q", historyYAML)
	}

	result, err := VerifyBackend(BackendSQLite, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != schemaVersion || result.CollectionCounts["clients"] != 1 || result.CollectionCounts["session"] != 1 || result.CollectionCounts["customlist"] != 1 || result.CollectionCounts["stats"] != 1 || result.CollectionCounts["confhist"] != 1 || result.CollectionCounts["store_meta"] != 1 {
		t.Fatalf("verify after migration = %#v", result)
	}
}

func TestSQLiteSchemaUpgradePathsBackfillNormalizedTables(t *testing.T) {
	for version := 1; version <= schemaVersion; version++ {
		t.Run("v"+strconv.Itoa(version), func(t *testing.T) {
			dir := t.TempDir()
			historyTS := seedLegacySQLiteStore(t, dir, version)

			s, err := OpenSQLite(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			sqlite, ok := s.(*sqliteStore)
			if !ok {
				t.Fatalf("sqlite store type = %T", s)
			}

			result, err := VerifyBackend(BackendSQLite, dir)
			if err != nil {
				t.Fatal(err)
			}
			wantCollections := map[string]int{
				"clients":    1,
				"session":    1,
				"customlist": 1,
				"stats":      1,
				"confhist":   1,
				"store_meta": 1,
			}
			if result.SchemaVersion != schemaVersion {
				t.Fatalf("schema version = %d, want %d", result.SchemaVersion, schemaVersion)
			}
			for collection, want := range wantCollections {
				if got := result.CollectionCounts[collection]; got != want {
					t.Fatalf("collection %s count = %d, want %d; result=%#v", collection, got, want, result)
				}
			}

			var clientGroup string
			if err := sqlite.db.QueryRow(`SELECT client_group FROM clients WHERE key = ?`, "192.0.2.80").Scan(&clientGroup); err != nil {
				t.Fatal(err)
			}
			if clientGroup != "default" {
				t.Fatalf("client group = %q", clientGroup)
			}
			var sessionUser string
			if err := sqlite.db.QueryRow(`SELECT username FROM sessions WHERE token = ?`, "legacy").Scan(&sessionUser); err != nil {
				t.Fatal(err)
			}
			if sessionUser != "admin" {
				t.Fatalf("session username = %q", sessionUser)
			}
			var customRows int
			if err := sqlite.db.QueryRow(`SELECT COUNT(*) FROM custom_lists WHERE list_id = ? AND domain = ?`, "custom", "legacy.example").Scan(&customRows); err != nil {
				t.Fatal(err)
			}
			if customRows != 1 {
				t.Fatalf("custom list rows = %d", customRows)
			}
			stats, err := s.Stats().Get("1m", "40")
			if err != nil {
				t.Fatal(err)
			}
			if stats.Counters["queries"] != uint64(version) {
				t.Fatalf("stats queries = %d, want %d", stats.Counters["queries"], version)
			}
			history, err := s.ConfigHistory().List(1)
			if err != nil {
				t.Fatal(err)
			}
			if len(history) != 1 || !history[0].TS.Equal(historyTS) || !strings.Contains(history[0].YAML, "legacy") {
				t.Fatalf("history = %#v, want ts %s with legacy yaml", history, historyTS)
			}
		})
	}
}

func TestSQLiteOpenRepairsMissingCollectionColumn(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "sis.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE kv (key TEXT PRIMARY KEY, value BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "store_meta:schema_version", []byte(`2`)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenSQLite(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = sql.Open("sqlite", filepath.Join(dir, "sis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hasCollection, err := sqliteHasColumn(db, "kv", "collection")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCollection {
		t.Fatal("sqlite open did not repair collection column")
	}
	var collection string
	if err := db.QueryRow(`SELECT collection FROM kv WHERE key = ?`, "store_meta:schema_version").Scan(&collection); err != nil {
		t.Fatal(err)
	}
	if collection != "store_meta" {
		t.Fatalf("collection = %q", collection)
	}
	hasClients, err := sqliteHasTable(db, "clients")
	if err != nil {
		t.Fatal(err)
	}
	if !hasClients {
		t.Fatal("sqlite open did not repair clients table")
	}
	hasSessions, err := sqliteHasTable(db, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	if !hasSessions {
		t.Fatal("sqlite open did not repair sessions table")
	}
	hasCustomLists, err := sqliteHasTable(db, "custom_lists")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCustomLists {
		t.Fatal("sqlite open did not repair custom lists table")
	}
	hasStats, err := sqliteHasTable(db, "stats")
	if err != nil {
		t.Fatal(err)
	}
	if !hasStats {
		t.Fatal("sqlite open did not repair stats table")
	}
	hasConfigHistory, err := sqliteHasTable(db, "config_history")
	if err != nil {
		t.Fatal(err)
	}
	if !hasConfigHistory {
		t.Fatal("sqlite open did not repair config history table")
	}
}

func seedLegacySQLiteStore(t *testing.T, dir string, version int) time.Time {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, "sis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if version == 1 {
		if _, err := db.Exec(`CREATE TABLE kv (key TEXT PRIMARY KEY, value BLOB NOT NULL)`); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := db.Exec(`CREATE TABLE kv (key TEXT PRIMARY KEY, collection TEXT NOT NULL DEFAULT 'unknown', value BLOB NOT NULL)`); err != nil {
			t.Fatal(err)
		}
	}

	insertLegacyKV(t, db, version, "clients:192.0.2.80", &Client{Key: "192.0.2.80", Type: "ip", Group: "default"})
	insertLegacyKV(t, db, version, "session:legacy", &Session{Token: "legacy", Username: "admin", ExpiresAt: time.Now().Add(time.Hour)})
	insertLegacyKV(t, db, version, "customlist:custom:legacy.example", true)
	insertLegacyKV(t, db, version, "stats:1m:40", &StatsRow{Counters: map[string]uint64{"queries": uint64(version)}})
	historyTS := time.Date(2026, 4, 28, 13, version, 0, 0, time.UTC)
	insertLegacyKV(t, db, version, "confhist:"+historyTS.Format(time.RFC3339Nano), &ConfigSnapshot{TS: historyTS, YAML: "legacy: true\n"})
	insertLegacyKV(t, db, version, "store_meta:schema_version", version)
	return historyTS
}

func insertLegacyKV(t *testing.T, db *sql.DB, schemaVersion int, key string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if schemaVersion == 1 {
		if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES(?, ?)`, key, raw); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := db.Exec(`INSERT INTO kv(key, collection, value) VALUES(?, ?, ?)`, key, collectionName(key), raw); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateJSONToSQLiteAndExport(t *testing.T) {
	dir := t.TempDir()
	jsonStore, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonStore.Clients().Upsert(&Client{Key: "192.0.2.44", Type: "ip", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := jsonStore.CustomLists().Add("custom", "migrated.example"); err != nil {
		t.Fatal(err)
	}
	if err := jsonStore.Close(); err != nil {
		t.Fatal(err)
	}

	count, err := MigrateJSONToSQLite(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("migrated zero records")
	}
	if _, err := MigrateJSONToSQLite(dir, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second migration err = %v", err)
	}

	sqliteStore, err := OpenSQLite(dir)
	if err != nil {
		t.Fatal(err)
	}
	client, err := sqliteStore.Clients().Get("192.0.2.44")
	if err != nil {
		t.Fatal(err)
	}
	if client.Group != "default" {
		t.Fatalf("client group = %q", client.Group)
	}
	domains, err := sqliteStore.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "migrated.example" {
		t.Fatalf("domains = %#v", domains)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatal(err)
	}

	exportPath := filepath.Join(t.TempDir(), "export.json")
	exported, err := ExportSQLiteToJSON(dir, exportPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if exported < count {
		t.Fatalf("exported %d records, migrated %d", exported, count)
	}
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "migrated.example") {
		t.Fatalf("export missing migrated domain: %s", raw)
	}
	if _, err := ExportSQLiteToJSON(dir, exportPath, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second export err = %v", err)
	}
}

func TestCompactBackend(t *testing.T) {
	jsonDir := t.TempDir()
	jsonStore, err := Open(jsonDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonStore.Clients().Upsert(&Client{Key: "192.0.2.60", Type: "ip"}); err != nil {
		t.Fatal(err)
	}
	if err := jsonStore.Close(); err != nil {
		t.Fatal(err)
	}
	path, err := CompactBackend(BackendJSON, jsonDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "sis.db.json" {
		t.Fatalf("json compact path = %s", path)
	}

	sqliteDir := t.TempDir()
	sqliteStore, err := OpenSQLite(sqliteDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.Clients().Upsert(&Client{Key: "192.0.2.61", Type: "ip"}); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatal(err)
	}
	path, err = CompactBackend(BackendSQLite, sqliteDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "sis.db" {
		t.Fatalf("sqlite compact path = %s", path)
	}
	if _, err := CompactBackend("postgres", t.TempDir()); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unsupported compact err = %v", err)
	}
}

func TestVerifyBackend(t *testing.T) {
	jsonDir := t.TempDir()
	result, err := VerifyBackend(BackendJSON, jsonDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Backend != BackendJSON || result.Records != 0 {
		t.Fatalf("empty json verify = %#v", result)
	}

	jsonStore, err := Open(jsonDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonStore.Clients().Upsert(&Client{Key: "192.0.2.70", Type: "ip"}); err != nil {
		t.Fatal(err)
	}
	if err := jsonStore.Close(); err != nil {
		t.Fatal(err)
	}
	result, err = VerifyBackend(BackendJSON, jsonDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Records == 0 || result.SchemaVersion != schemaVersion {
		t.Fatalf("json verify = %#v", result)
	}
	if result.CollectionCounts["clients"] != 1 || result.CollectionCounts["store_meta"] != 1 {
		t.Fatalf("json verify collections = %#v", result.CollectionCounts)
	}

	sqliteDir := t.TempDir()
	sqliteStore, err := OpenSQLite(sqliteDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.Clients().Upsert(&Client{Key: "192.0.2.71", Type: "ip"}); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatal(err)
	}
	result, err = VerifyBackend(BackendSQLite, sqliteDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Records == 0 || result.SchemaVersion != schemaVersion {
		t.Fatalf("sqlite verify = %#v", result)
	}
	if result.CollectionCounts["clients"] != 1 || result.CollectionCounts["store_meta"] != 1 {
		t.Fatalf("sqlite verify collections = %#v", result.CollectionCounts)
	}
	names := result.CollectionNames()
	if len(names) != 2 || names[0] != "clients" || names[1] != "store_meta" {
		t.Fatalf("sqlite verify collection names = %#v", names)
	}

	if _, err := VerifyBackend("postgres", t.TempDir()); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unsupported verify err = %v", err)
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
	if err := s.CustomLists().Remove("custom", "example.com"); err != nil {
		t.Fatal(err)
	}
	domains, err = s.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 0 {
		t.Fatalf("removed domains = %#v", domains)
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
	if err := s.Sessions().Delete("tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sessions().Get("tok"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session err = %v", err)
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

func TestStoreSaveUsesRestrictedFinalFileWithoutTempLeftovers(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Clients().Upsert(&Client{Key: "192.0.2.10"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "sis.db.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %o, want 640", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("unexpected temp file %q", entry.Name())
		}
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Clients().Upsert(&Client{Key: "192.0.2.20", Type: "ip", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CustomLists().Add("custom", "persisted.example"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	client, err := reopened.Clients().Get("192.0.2.20")
	if err != nil {
		t.Fatal(err)
	}
	if client.Group != "default" {
		t.Fatalf("client group = %q", client.Group)
	}
	domains, err := reopened.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "persisted.example" {
		t.Fatalf("domains = %#v", domains)
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
