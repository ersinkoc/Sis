package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// VerifyResult summarizes a durable store verification.
type VerifyResult struct {
	Backend          string         `json:"backend"`
	Path             string         `json:"path"`
	Records          int            `json:"records"`
	SchemaVersion    int            `json:"schema_version"`
	CollectionCounts map[string]int `json:"collection_counts,omitempty"`
}

// VerifyBackend validates that the configured store backend can be read.
func VerifyBackend(backend, dataDir string) (*VerifyResult, error) {
	switch backend {
	case "", BackendJSON:
		return verifyJSON(dataDir)
	case BackendSQLite:
		return verifySQLite(dataDir)
	default:
		return nil, fmt.Errorf("store backend %q is not supported; supported values: json, sqlite", backend)
	}
}

func verifyJSON(dataDir string) (*VerifyResult, error) {
	path := filepath.Join(dataDir, "sis.db.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &VerifyResult{Backend: BackendJSON, Path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return &VerifyResult{Backend: BackendJSON, Path: path}, nil
	}
	var rows map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	var version int
	_ = json.Unmarshal(rows["store_meta:schema_version"], &version)
	return &VerifyResult{
		Backend:          BackendJSON,
		Path:             path,
		Records:          len(rows),
		SchemaVersion:    version,
		CollectionCounts: collectionCounts(rows),
	}, nil
}

func verifySQLite(dataDir string) (*VerifyResult, error) {
	path := filepath.Join(dataDir, "sis.db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var quickCheck string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&quickCheck); err != nil {
		return nil, err
	}
	if quickCheck != "ok" {
		return nil, fmt.Errorf("sqlite quick_check failed: %s", quickCheck)
	}

	var records int
	if err := db.QueryRow(`SELECT COUNT(*) FROM kv`).Scan(&records); err != nil {
		return nil, err
	}
	var version int
	var rawVersion []byte
	if err := db.QueryRow(`SELECT value FROM kv WHERE key = ?`, "store_meta:schema_version").Scan(&rawVersion); err == nil {
		_ = json.Unmarshal(rawVersion, &version)
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	counts, err := sqliteCollectionCounts(db)
	if err != nil {
		return nil, err
	}
	return &VerifyResult{
		Backend:          BackendSQLite,
		Path:             path,
		Records:          records,
		SchemaVersion:    version,
		CollectionCounts: counts,
	}, nil
}

func collectionCounts(rows map[string]json.RawMessage) map[string]int {
	counts := make(map[string]int)
	for key := range rows {
		counts[collectionName(key)]++
	}
	return counts
}

func sqliteCollectionCounts(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`SELECT key FROM kv`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		counts[collectionName(key)]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func collectionName(key string) string {
	if i := strings.IndexByte(key, ':'); i > 0 {
		return key[:i]
	}
	return "unknown"
}

// CollectionNames returns sorted collection names present in a verification result.
func (r *VerifyResult) CollectionNames() []string {
	if r == nil || len(r.CollectionCounts) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.CollectionCounts))
	for name := range r.CollectionCounts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
