package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
		return s.ensureSQLiteSchema()
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

func (s *sqliteStore) ensureSQLiteSchema() error {
	if err := s.ensureCollectionColumn(); err != nil {
		return err
	}
	return s.ensureClientTable()
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

func (s *sqliteStore) ensureClientTable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if err := s.createClientTableLocked(); err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT key, value FROM kv WHERE collection = 'clients' OR key >= ? AND key < ?`, "clients:", "clients:\xff")
	if err != nil {
		return err
	}
	type pendingClient struct {
		key    string
		client Client
	}
	var pending []pendingClient
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		var client Client
		if err := json.Unmarshal(raw, &client); err != nil {
			_ = rows.Close()
			return err
		}
		if client.Key == "" {
			client.Key = strings.TrimPrefix(key, "clients:")
		}
		pending = append(pending, pendingClient{key: key, client: client})
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
	for _, row := range pending {
		if err := upsertClientSQL(tx, &row.client); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) createClientTableLocked() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS clients (
			key TEXT PRIMARY KEY,
			client_type TEXT NOT NULL,
			name TEXT NOT NULL,
			client_group TEXT NOT NULL,
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			last_ip TEXT NOT NULL,
			hidden INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_group ON clients(client_group)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_last_seen ON clients(last_seen)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
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

func sqliteHasTable(db *sql.DB, table string) (bool, error) {
	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *sqliteStore) Clients() ClientStore {
	return &sqliteClientStore{s: s}
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

	if collectionName(key) == "clients" {
		var client Client
		if err := json.Unmarshal(raw, &client); err != nil {
			return err
		}
		if client.Key == "" {
			client.Key = strings.TrimPrefix(key, "clients:")
		}
		return s.putClientLocked(key, &client, raw)
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

	if strings.HasPrefix(key, "clients:") {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		clientKey := strings.TrimPrefix(key, "clients:")
		clientRes, err := tx.Exec(`DELETE FROM clients WHERE key = ?`, clientKey)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		kvRes, err := tx.Exec(`DELETE FROM kv WHERE key = ?`, key)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		clientAffected, err := clientRes.RowsAffected()
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		kvAffected, err := kvRes.RowsAffected()
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if clientAffected == 0 && kvAffected == 0 {
			_ = tx.Rollback()
			return ErrNotFound
		}
		return tx.Commit()
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

type sqliteClientStore struct {
	s *sqliteStore
}

func (c *sqliteClientStore) Get(key string) (*Client, error) {
	c.s.mu.RLock()
	defer c.s.mu.RUnlock()
	if c.s.closed {
		return nil, ErrClosed
	}
	row := c.s.db.QueryRow(`SELECT key, client_type, name, client_group, first_seen, last_seen, last_ip, hidden FROM clients WHERE key = ?`, key)
	client, err := scanSQLiteClient(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (c *sqliteClientStore) List() ([]*Client, error) {
	c.s.mu.RLock()
	defer c.s.mu.RUnlock()
	if c.s.closed {
		return nil, ErrClosed
	}
	rows, err := c.s.db.Query(`SELECT key, client_type, name, client_group, first_seen, last_seen, last_ip, hidden FROM clients ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Client{}
	for rows.Next() {
		client, err := scanSQLiteClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, client)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sqliteClientStore) Upsert(client *Client) error {
	if client == nil || client.Key == "" {
		return fmt.Errorf("clients.key: required")
	}
	raw, err := json.Marshal(client)
	if err != nil {
		return err
	}
	c.s.mu.RLock()
	defer c.s.mu.RUnlock()
	if c.s.closed {
		return ErrClosed
	}
	return c.s.putClientLocked("clients:"+client.Key, client, raw)
}

func (c *sqliteClientStore) Delete(key string) error {
	return c.s.delete("clients:" + key)
}

func (s *sqliteStore) putClientLocked(key string, client *Client, raw json.RawMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := upsertClientSQL(tx, client); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO kv(key, collection, value) VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET collection = excluded.collection, value = excluded.value`,
		key,
		collectionName(key),
		raw,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func upsertClientSQL(tx *sql.Tx, client *Client) error {
	_, err := tx.Exec(
		`INSERT INTO clients(key, client_type, name, client_group, first_seen, last_seen, last_ip, hidden)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			client_type = excluded.client_type,
			name = excluded.name,
			client_group = excluded.client_group,
			first_seen = excluded.first_seen,
			last_seen = excluded.last_seen,
			last_ip = excluded.last_ip,
			hidden = excluded.hidden`,
		client.Key,
		client.Type,
		client.Name,
		client.Group,
		sqliteTime(client.FirstSeen),
		sqliteTime(client.LastSeen),
		client.LastIP,
		boolInt(client.Hidden),
	)
	return err
}

type sqliteScanner interface {
	Scan(dest ...any) error
}

func scanSQLiteClient(row sqliteScanner) (*Client, error) {
	var client Client
	var firstSeen, lastSeen string
	var hidden int
	if err := row.Scan(&client.Key, &client.Type, &client.Name, &client.Group, &firstSeen, &lastSeen, &client.LastIP, &hidden); err != nil {
		return nil, err
	}
	var err error
	client.FirstSeen, err = parseSQLiteTime(firstSeen)
	if err != nil {
		return nil, err
	}
	client.LastSeen, err = parseSQLiteTime(lastSeen)
	if err != nil {
		return nil, err
	}
	client.Hidden = hidden != 0
	return &client, nil
}

func sqliteTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, raw)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
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
