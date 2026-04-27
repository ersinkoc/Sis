package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateJSONToSQLite imports sis.db.json from dataDir into a SQLite sis.db file.
func MigrateJSONToSQLite(dataDir string, force bool) (int, error) {
	jsonPath := filepath.Join(dataDir, "sis.db.json")
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return 0, err
	}
	rows, err := decodeJSONRows(raw)
	if err != nil {
		return 0, fmt.Errorf("decode %s: %w", jsonPath, err)
	}

	sqlitePath := filepath.Join(dataDir, "sis.db")
	if err := prepareSQLiteMigrationTarget(sqlitePath, force); err != nil {
		return 0, err
	}
	dst, err := OpenSQLite(dataDir)
	if err != nil {
		return 0, err
	}
	defer dst.Close()

	sqlite, ok := dst.(*sqliteStore)
	if !ok {
		return 0, fmt.Errorf("sqlite migration opened unexpected store type %T", dst)
	}
	for key, value := range rows {
		if err := sqlite.putRawJSON(key, value); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// ExportSQLiteToJSON writes a sis.db-compatible JSON export from the SQLite store.
func ExportSQLiteToJSON(dataDir, outPath string, force bool) (int, error) {
	src, err := OpenSQLite(dataDir)
	if err != nil {
		return 0, err
	}
	defer src.Close()

	sqlite, ok := src.(*sqliteStore)
	if !ok {
		return 0, fmt.Errorf("sqlite export opened unexpected store type %T", src)
	}
	rows := sqlite.scan("")
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return 0, err
	}
	raw = append(raw, '\n')
	if err := writeExportFileAtomic(outPath, raw, force); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func decodeJSONRows(raw []byte) (map[string]json.RawMessage, error) {
	rows := map[string]json.RawMessage{}
	if len(raw) == 0 {
		return rows, nil
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	for key, value := range rows {
		if key == "" {
			return nil, fmt.Errorf("empty key")
		}
		if !json.Valid(value) {
			return nil, fmt.Errorf("invalid JSON value for key %q", key)
		}
	}
	return rows, nil
}

func prepareSQLiteMigrationTarget(path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; pass -force to overwrite", path)
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	for _, path := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func writeExportFileAtomic(path string, raw []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; pass -force to overwrite", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
