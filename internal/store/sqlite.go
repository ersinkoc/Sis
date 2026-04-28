package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
			collection TEXT NOT NULL DEFAULT 'unknown',
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
		return s.ensureCollectionColumn()
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

func (s *sqliteStore) ensureCollectionColumn() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	hasCollection, err := sqliteHasColumn(s.db, "kv", "collection")
	if err != nil {
		return err
	}
	if !hasCollection {
		if _, err := s.db.Exec(`ALTER TABLE kv ADD COLUMN collection TEXT NOT NULL DEFAULT 'unknown'`); err != nil {
			return err
		}
	}
	rows, err := s.db.Query(`SELECT key FROM kv WHERE collection = 'unknown' OR collection = ''`)
	if err != nil {
		return err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := tx.Exec(`UPDATE kv SET collection = ? WHERE key = ?`, collectionName(key), key); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_kv_collection ON kv(collection)`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func sqliteHasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
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

func (s *sqliteStore) compact() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	for _, stmt := range []string{
		`PRAGMA wal_checkpoint(TRUNCATE)`,
		`VACUUM`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
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
		`INSERT INTO kv(key, collection, value) VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET collection = excluded.collection, value = excluded.value`,
		key,
		collectionName(key),
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
