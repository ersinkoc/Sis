package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// CompactBackend rewrites or vacuums the configured store backend in dataDir.
func CompactBackend(backend, dataDir string) (string, error) {
	switch backend {
	case "", BackendJSON:
		path := filepath.Join(dataDir, "sis.db.json")
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		s, err := Open(dataDir)
		if err != nil {
			return "", err
		}
		if err := s.Close(); err != nil {
			return "", err
		}
		return path, nil
	case BackendSQLite:
		path := filepath.Join(dataDir, "sis.db")
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		s, err := OpenSQLite(dataDir)
		if err != nil {
			return "", err
		}
		sqlite, ok := s.(*sqliteStore)
		if !ok {
			_ = s.Close()
			return "", fmt.Errorf("sqlite compaction opened unexpected store type %T", s)
		}
		if err := sqlite.compact(); err != nil {
			_ = s.Close()
			return "", err
		}
		if err := s.Close(); err != nil {
			return "", err
		}
		return path, nil
	default:
		return "", fmt.Errorf("store backend %q is not supported; supported values: json, sqlite", backend)
	}
}
