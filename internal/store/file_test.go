package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	domains, err := reopened.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "sqlite.example" {
		t.Fatalf("domains = %#v", domains)
	}
	gotSession, err := reopened.Sessions().Get("tok")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.Username != "admin" {
		t.Fatalf("username = %q", gotSession.Username)
	}
	rows, err := reopened.Stats().List("1m")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Counters["queries"] != 20 {
		t.Fatalf("rows = %#v", rows)
	}
	history, err := reopened.ConfigHistory().List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].YAML == "" {
		t.Fatalf("history = %#v", history)
	}
	if err := reopened.Clients().Delete(client.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Clients().Get(client.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted client err = %v", err)
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
	if _, err := db.Exec(`INSERT INTO kv(key, value) VALUES (?, ?)`, "clients:192.0.2.31", rawClient); err != nil {
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

	result, err := VerifyBackend(BackendSQLite, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != schemaVersion || result.CollectionCounts["clients"] != 1 || result.CollectionCounts["store_meta"] != 1 {
		t.Fatalf("verify after migration = %#v", result)
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
