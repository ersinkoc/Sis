package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db     *sql.DB
	mu     sync.RWMutex
	closed bool
}

// OpenSQLite opens the SQLite-backed store in dataDir.
func OpenSQLite(dataDir string) (Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "sis.db")
	_, statErr := os.Stat(path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &sqliteStore{db: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if os.IsNotExist(statErr) {
		_ = os.Chmod(path, 0o640)
	}
	if err := s.runMigrations(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *sqliteStore) init() error {
	for _, stmt := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`CREATE TABLE IF NOT EXISTS kv (
			key TEXT PRIMARY KEY,
			value BLOB NOT NULL
		)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteStore) runMigrations() error {
	var current int
	_ = s.getJSON("store_meta:schema_version", &current)
	if current >= schemaVersion {
		return nil
	}
	for _, migration := range migrations() {
		if migration.Version <= current {
			continue
		}
		if migration.Apply != nil {
			if err := migration.Apply(s); err != nil {
				return err
			}
		}
		if err := s.putJSON("store_meta:schema_version", migration.Version); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteStore) Clients() ClientStore {
	return &clientStore{s: s}
}

func (s *sqliteStore) CustomLists() CustomListStore {
	return &customListStore{s: s}
}

func (s *sqliteStore) Sessions() SessionStore {
	return &sessionStore{s: s}
}

func (s *sqliteStore) Stats() StatsStore {
	return &statsStore{s: s}
}

func (s *sqliteStore) ConfigHistory() ConfigHistoryStore {
	return &configHistoryStore{s: s}
}

func (s *sqliteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

func (s *sqliteStore) getJSON(key string, out any) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}

	var raw []byte
	if err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	return json.Unmarshal(raw, out)
}

func (s *sqliteStore) putJSON(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.putRawJSON(key, raw)
}

func (s *sqliteStore) putRawJSON(key string, raw json.RawMessage) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}

	_, err := s.db.Exec(
		`INSERT INTO kv(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key,
		raw,
	)
	return err
}

func (s *sqliteStore) delete(key string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}

	res, err := s.db.Exec(`DELETE FROM kv WHERE key = ?`, key)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *sqliteStore) scan(prefix string) map[string]json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return map[string]json.RawMessage{}
	}

	rows, err := s.db.Query(`SELECT key, value FROM kv WHERE key >= ? AND key < ?`, prefix, prefix+"\xff")
	if err != nil {
		return map[string]json.RawMessage{}
	}
	defer rows.Close()

	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return map[string]json.RawMessage{}
		}
		out[key] = append(json.RawMessage(nil), raw...)
	}
	return out
}
