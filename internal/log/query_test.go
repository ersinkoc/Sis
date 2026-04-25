package log

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ersinkoc/sis/internal/config"
)

func TestQueryWritePrivacyHashedAndSubscribe(t *testing.T) {
	cfg := testConfig(t)
	cfg.Privacy.LogMode = "hashed"
	cfg.Privacy.LogSalt = "test-salt"
	q, err := OpenQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	sub := q.Subscribe(1)
	defer q.Unsubscribe(sub)

	err = q.Write(&Entry{
		ClientKey: "aa:bb:cc:dd:ee:ff", ClientName: "phone", ClientIP: "192.168.1.10",
		QName: "example.com.", QType: "A", QClass: "IN", RCode: "NOERROR", Proto: "udp",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := <-sub
	if got.ClientKey == "aa:bb:cc:dd:ee:ff" || got.ClientKey == "" {
		t.Fatalf("client key was not hashed: %q", got.ClientKey)
	}
	if got.ClientIP == "192.168.1.10" || got.ClientIP == "" {
		t.Fatalf("client IP was not hashed: %q", got.ClientIP)
	}
	if got.ClientName != "phone" {
		t.Fatalf("client name = %q", got.ClientName)
	}
}

func TestQueryReconfigurePrivacy(t *testing.T) {
	cfg := testConfig(t)
	cfg.Logging.QueryLog = false
	q, err := OpenQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	sub := q.Subscribe(2)
	defer q.Unsubscribe(sub)
	if err := q.Write(&Entry{ClientKey: "client-1", ClientIP: "192.168.1.10", QName: "before.example."}); err != nil {
		t.Fatal(err)
	}
	before := <-sub
	if before.ClientKey != "client-1" {
		t.Fatalf("unexpected pre-reconfigure key: %q", before.ClientKey)
	}
	cfg.Privacy.LogMode = "hashed"
	cfg.Privacy.LogSalt = "test-salt"
	if err := q.Reconfigure(cfg); err != nil {
		t.Fatal(err)
	}
	if err := q.Write(&Entry{ClientKey: "client-1", ClientIP: "192.168.1.10", QName: "after.example."}); err != nil {
		t.Fatal(err)
	}
	after := <-sub
	if after.ClientKey == "client-1" || after.ClientKey == "" {
		t.Fatalf("client key was not hashed after reconfigure: %q", after.ClientKey)
	}
}

func TestQueryReconfigureEnablesFileLogging(t *testing.T) {
	cfg := testConfig(t)
	cfg.Logging.QueryLog = false
	q, err := OpenQuery(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if err := q.Write(&Entry{QName: "before.example.", QType: "A", QClass: "IN", RCode: "NOERROR"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Server.DataDir, "logs", "sis-query.log")); !os.IsNotExist(err) {
		t.Fatalf("query log should not exist before enable, err=%v", err)
	}
	cfg.Logging.QueryLog = true
	if err := q.Reconfigure(cfg); err != nil {
		t.Fatal(err)
	}
	if err := q.Write(&Entry{QName: "after.example.", QType: "A", QClass: "IN", RCode: "NOERROR"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Server.DataDir, "logs", "sis-query.log")); err != nil {
		t.Fatalf("query log should exist after enable: %v", err)
	}
}

func TestQueryRotationCreatesRotatedFile(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRotator(filepath.Join(dir, "sis-query.log"), 64, 7, false)
	if err != nil {
		t.Fatal(err)
	}
	q := &Query{enabled: true, mode: "full", rotator: r, enc: json.NewEncoder(r), fanout: newFanout(0)}
	defer q.Close()
	for i := 0; i < 10; i++ {
		if err := q.Write(&Entry{QName: "example.com.", QType: "A", QClass: "IN", RCode: "NOERROR"}); err != nil {
			t.Fatal(err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(dir, "sis-query.log.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected rotated file")
	}
}

func TestAuditSeparateFile(t *testing.T) {
	cfg := testConfig(t)
	a, err := OpenAudit(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Auditf("group.update", "kids", map[string]string{"old": "1"}, map[string]string{"new": "2"}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(cfg.Server.DataDir, "logs", "sis-audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !bufio.NewScanner(f).Scan() {
		t.Fatal("expected audit entry")
	}
	if _, err := os.Stat(filepath.Join(cfg.Server.DataDir, "logs", "sis-query.log")); !os.IsNotExist(err) {
		t.Fatalf("query log should not be created by audit write, stat err=%v", err)
	}
}

func TestAuditReconfigureEnablesFileLogging(t *testing.T) {
	cfg := testConfig(t)
	cfg.Logging.AuditLog = false
	a, err := OpenAudit(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Auditf("before", "target", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Server.DataDir, "logs", "sis-audit.log")); !os.IsNotExist(err) {
		t.Fatalf("audit log should not exist before enable, err=%v", err)
	}
	cfg.Logging.AuditLog = true
	if err := a.Reconfigure(cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.Auditf("after", "target", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Server.DataDir, "logs", "sis-audit.log")); err != nil {
		t.Fatalf("audit log should exist after enable: %v", err)
	}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Server:  config.Server{DataDir: t.TempDir()},
		Privacy: config.Privacy{LogMode: "full"},
		Logging: config.Logging{
			QueryLog: true, AuditLog: true, RotateSizeMB: 1, RetentionDays: 7,
		},
	}
}
