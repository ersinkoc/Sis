package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	"github.com/ersinkoc/sis/internal/store"
)

func TestSeedConfigClientsCreatesAndUpdates(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{Clients: []config.Client{{
		Key: "192.168.1.50", Type: "ip", Name: "TV", Group: "default", Hidden: true,
	}}}
	if err := seedConfigClients(st, cfg); err != nil {
		t.Fatal(err)
	}
	client, err := st.Clients().Get("192.168.1.50")
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "TV" || client.Group != "default" || !client.Hidden {
		t.Fatalf("client = %#v", client)
	}
	firstSeen := client.FirstSeen

	client.LastSeen = time.Now().UTC().Add(time.Hour)
	if err := st.Clients().Upsert(client); err != nil {
		t.Fatal(err)
	}
	cfg.Clients[0].Name = "Living Room TV"
	cfg.Clients[0].Hidden = false
	if err := seedConfigClients(st, cfg); err != nil {
		t.Fatal(err)
	}
	client, err = st.Clients().Get("192.168.1.50")
	if err != nil {
		t.Fatal(err)
	}
	if client.Name != "Living Room TV" || client.Hidden {
		t.Fatalf("client was not updated: %#v", client)
	}
	if !client.FirstSeen.Equal(firstSeen) {
		t.Fatalf("first_seen changed: before=%s after=%s", firstSeen, client.FirstSeen)
	}
}

func TestSeedConfigClientsDefaults(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := seedConfigClients(st, &config.Config{Clients: []config.Client{{Key: "192.168.1.51"}}}); err != nil {
		t.Fatal(err)
	}
	client, err := st.Clients().Get("192.168.1.51")
	if err != nil {
		t.Fatal(err)
	}
	if client.Type != "ip" || client.Group != "default" {
		t.Fatalf("client defaults = %#v", client)
	}
}

func TestUpsertConfigUserTrimsUsernameAndRejectsWeakPassword(t *testing.T) {
	path := writeUserTestConfig(t)
	if err := upsertConfigUser(path, " admin ", "secret123", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Auth.Users) != 1 || cfg.Auth.Users[0].Username != "admin" {
		t.Fatalf("users = %#v", cfg.Auth.Users)
	}
	if err := upsertConfigUser(path, "operator", "short", false); err == nil || !strings.Contains(err.Error(), "at least 8 chars") {
		t.Fatalf("weak password err = %v", err)
	}
}

func TestDumpDebugWritesRestrictedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := dumpDebug(dir); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "dbg", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("debug files = %#v", matches)
	}
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("%s mode = %o, want 640", path, got)
		}
	}
}

func TestRunQueryRejectsInvalidProto(t *testing.T) {
	err := runQuery([]string{"-proto", "bad", "test", "example.com"})
	if err == nil || !strings.Contains(err.Error(), "proto must be udp or tcp") {
		t.Fatalf("invalid proto err = %v", err)
	}
}

func TestRunConfigShowRedactsSecretsByDefault(t *testing.T) {
	path := writeSecretTestConfig(t)
	out := captureStdout(t, func() error {
		return runConfig([]string{"show", "-config", path})
	})
	if strings.Contains(out, "secret-hash") || strings.Contains(out, "secret-salt") {
		t.Fatalf("config show leaked secrets:\n%s", out)
	}
	if !strings.Contains(out, "password_hash: redacted") || !strings.Contains(out, "log_salt: redacted") {
		t.Fatalf("config show did not redact expected fields:\n%s", out)
	}
}

func TestRunConfigShowSecretsFlagIncludesSecrets(t *testing.T) {
	path := writeSecretTestConfig(t)
	out := captureStdout(t, func() error {
		return runConfig([]string{"show", "-config", path, "-secrets"})
	})
	if !strings.Contains(out, "secret-hash") || !strings.Contains(out, "secret-salt") {
		t.Fatalf("config show -secrets did not include secrets:\n%s", out)
	}
}

func TestRunBackupCreateIncludesConfigStoreAndManifest(t *testing.T) {
	path := writeUserTestConfig(t)
	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.Server.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Clients().Upsert(&store.Client{Key: "192.0.2.30", Type: "ip", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "sis-backup.tar.gz")
	if err := runBackup([]string{"create", "-config", path, "-out", out}); err != nil {
		t.Fatal(err)
	}

	files := readBackupFiles(t, out)
	for _, name := range []string{"manifest.json", "sis.yaml", "sis.db.json"} {
		if len(files[name]) == 0 {
			t.Fatalf("backup missing %s; files=%v", name, files)
		}
	}
	var manifest map[string]string
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["config_path"] != path || manifest["data_dir"] != cfg.Server.DataDir {
		t.Fatalf("manifest = %#v", manifest)
	}
	if !strings.Contains(string(files["sis.db.json"]), "192.0.2.30") {
		t.Fatalf("store backup missing client: %s", files["sis.db.json"])
	}
}

func TestRunBackupCreateWithoutStoreFile(t *testing.T) {
	path := writeUserTestConfig(t)
	out := filepath.Join(t.TempDir(), "sis-backup.tar.gz")
	if err := runBackup([]string{"create", "-config", path, "-out", out}); err != nil {
		t.Fatal(err)
	}
	files := readBackupFiles(t, out)
	if len(files["manifest.json"]) == 0 || len(files["sis.yaml"]) == 0 {
		t.Fatalf("backup files = %v", files)
	}
	if _, ok := files["sis.db.json"]; ok {
		t.Fatal("empty data dir should not include sis.db.json")
	}
}

func TestRunBackupVerifyAndRestore(t *testing.T) {
	path := writeUserTestConfig(t)
	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.Server.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CustomLists().Add("custom", "blocked.test"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(t.TempDir(), "sis-backup.tar.gz")
	if err := runBackup([]string{"create", "-config", path, "-out", backupPath}); err != nil {
		t.Fatal(err)
	}
	if err := runBackup([]string{"verify", "-in", backupPath}); err != nil {
		t.Fatal(err)
	}

	restoreDir := t.TempDir()
	restoreConfig := filepath.Join(restoreDir, "sis.yaml")
	restoreData := filepath.Join(restoreDir, "data")
	if err := runBackup([]string{"restore", "-in", backupPath, "-config", restoreConfig, "-data-dir", restoreData}); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(restoreConfig); err != nil || !strings.Contains(string(raw), "cloudflare") {
		t.Fatalf("restored config raw=%q err=%v", raw, err)
	}
	if raw, err := os.ReadFile(filepath.Join(restoreData, "sis.db.json")); err != nil || !strings.Contains(string(raw), "blocked.test") {
		t.Fatalf("restored store raw=%q err=%v", raw, err)
	}
	if err := runBackup([]string{"restore", "-in", backupPath, "-config", restoreConfig, "-data-dir", restoreData}); err == nil {
		t.Fatal("restore overwrote existing files without -force")
	}
	if err := runBackup([]string{"restore", "-in", backupPath, "-config", restoreConfig, "-data-dir", restoreData, "-force"}); err != nil {
		t.Fatal(err)
	}

	partialDir := t.TempDir()
	partialConfig := filepath.Join(partialDir, "sis.yaml")
	partialData := filepath.Join(partialDir, "data")
	if err := os.MkdirAll(partialData, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partialData, "sis.db.json"), []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := runBackup([]string{"restore", "-in", backupPath, "-config", partialConfig, "-data-dir", partialData}); err == nil {
		t.Fatal("partial restore succeeded despite existing store file")
	}
	if _, err := os.Stat(partialConfig); !os.IsNotExist(err) {
		t.Fatalf("restore wrote config before detecting store conflict: %v", err)
	}
}

func TestRunBackupCreateAndRestoreSQLiteStore(t *testing.T) {
	path := writeUserTestConfig(t)
	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.StoreBackend = store.BackendSQLite
	if err := (&config.Loader{Path: path}).Save(cfg); err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenSQLite(cfg.Server.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CustomLists().Add("custom", "sqlite-backup.test"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(t.TempDir(), "sis-sqlite-backup.tar.gz")
	if err := runBackup([]string{"create", "-config", path, "-out", backupPath}); err != nil {
		t.Fatal(err)
	}
	files := readBackupFiles(t, backupPath)
	if !strings.Contains(string(files["sis.db.json"]), "sqlite-backup.test") {
		t.Fatalf("sqlite logical backup missing custom list: %s", files["sis.db.json"])
	}
	var manifest map[string]string
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["store"] != store.BackendSQLite {
		t.Fatalf("manifest store = %q", manifest["store"])
	}

	restoreDir := t.TempDir()
	restoreConfig := filepath.Join(restoreDir, "sis.yaml")
	restoreData := filepath.Join(restoreDir, "data")
	if err := runBackup([]string{"restore", "-in", backupPath, "-config", restoreConfig, "-data-dir", restoreData}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(restoreData, "sis.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(restoreData, "sis.db.json")); !os.IsNotExist(err) {
		t.Fatalf("sqlite restore wrote JSON store file: %v", err)
	}
	restored, err := store.OpenSQLite(restoreData)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	domains, err := restored.CustomLists().List("custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "sqlite-backup.test" {
		t.Fatalf("restored sqlite domains = %#v", domains)
	}
}

func TestRunBackupVerifyRejectsInvalidArchive(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := addBytesToTar(tw, "evil.txt", []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "invalid.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runBackup([]string{"verify", "-in", path}); err == nil {
		t.Fatal("invalid backup verified successfully")
	}
}

func TestRunBackupRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"nonesuch"},
		{"verify"},
		{"create", "extra"},
		{"restore"},
	} {
		if err := runBackup(args); err == nil {
			t.Fatalf("runBackup(%v) succeeded, want error", args)
		}
	}

	if err := runBackup([]string{"create", "-config", filepath.Join(t.TempDir(), "missing.yaml")}); err == nil {
		t.Fatal("missing config succeeded, want error")
	}
}

func TestRunStoreMigrateAndExport(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Clients().Upsert(&store.Client{Key: "192.0.2.55", Type: "ip", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	if err := runStore([]string{"migrate-json-to-sqlite", "-data-dir", dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sis.db")); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(t.TempDir(), "sis.db.json")
	if err := runStore([]string{"export-sqlite-json", "-data-dir", dir, "-out", exportPath}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "192.0.2.55") {
		t.Fatalf("export missing client: %s", raw)
	}
	if err := runStore([]string{"compact", "-data-dir", dir, "-backend", store.BackendSQLite}); err != nil {
		t.Fatal(err)
	}
	if err := runStore([]string{"verify", "-data-dir", dir, "-backend", store.BackendSQLite}); err != nil {
		t.Fatal(err)
	}
}

func TestRunStoreRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"nonesuch"},
		{"migrate-json-to-sqlite"},
		{"export-sqlite-json"},
		{"compact"},
		{"verify"},
	} {
		if err := runStore(args); err == nil {
			t.Fatalf("runStore(%v) succeeded, want error", args)
		}
	}
}

func TestRunBackupCreateUsesDefaultOutputPath(t *testing.T) {
	path := writeUserTestConfig(t)
	workDir := t.TempDir()
	t.Chdir(workDir)

	if err := runBackup([]string{"create", "-config", path}); err != nil {
		t.Fatal(err)
	}

	matches, err := filepath.Glob(filepath.Join(workDir, "sis-backup-*.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("default backup files = %v, want one", matches)
	}
	files := readBackupFiles(t, matches[0])
	if len(files["manifest.json"]) == 0 || len(files["sis.yaml"]) == 0 {
		t.Fatalf("backup files = %v", files)
	}
}

func TestCreateBackupReturnsOutputAndStoreErrors(t *testing.T) {
	path := writeUserTestConfig(t)
	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}

	if err := createBackup(path, cfg.Server.DataDir, cfg.Server.StoreBackend, filepath.Join(t.TempDir(), "missing", "backup.tar.gz"), ""); err == nil {
		t.Fatal("backup in missing output directory succeeded, want error")
	}

	dbDir := filepath.Join(cfg.Server.DataDir, "sis.db.json")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createBackup(path, cfg.Server.DataDir, cfg.Server.StoreBackend, filepath.Join(t.TempDir(), "sis-backup.tar.gz"), ""); err == nil {
		t.Fatal("directory store file succeeded, want error")
	}
}

func TestBackupHelperErrors(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := addJSONToTar(tw, "bad.json", func() {}); err == nil {
		t.Fatal("unmarshalable JSON backup entry succeeded, want error")
	}
	if err := addFileToTar(tw, filepath.Join(t.TempDir(), "missing"), "missing"); err == nil {
		t.Fatal("missing file backup entry succeeded, want error")
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := addBytesToTar(tw, "closed", []byte("x"), 0o600); err == nil {
		t.Fatal("write to closed tar succeeded, want error")
	}
}

func readBackupFiles(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := make(map[string][]byte)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = raw
	}
	return files
}

func writeUserTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sis.yaml")
	cfg := &config.Config{
		Server: config.Server{DataDir: filepath.Join(dir, "data"), TZ: "Local"},
		Cache: config.Cache{
			MinTTL: config.Duration{Duration: time.Minute},
			MaxTTL: config.Duration{Duration: time.Hour},
		},
		Privacy: config.Privacy{LogMode: "full"},
		Upstreams: []config.Upstream{{
			ID: "cloudflare", URL: "https://cloudflare-dns.com/dns-query",
			Bootstrap: []string{"1.1.1.1"},
		}},
		Groups: []config.Group{{Name: "default"}},
		Auth:   config.Auth{FirstRun: true},
	}
	if err := (&config.Loader{Path: path}).Save(cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSecretTestConfig(t *testing.T) string {
	t.Helper()
	path := writeUserTestConfig(t)
	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Privacy.LogMode = "hashed"
	cfg.Privacy.LogSalt = "secret-salt"
	cfg.Auth.FirstRun = false
	cfg.Auth.Users = []config.User{{Username: "admin", PasswordHash: "secret-hash"}}
	if err := (&config.Loader{Path: path}).Save(cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()
	defer r.Close()
	if err := fn(); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
