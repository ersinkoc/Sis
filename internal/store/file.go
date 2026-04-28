package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const schemaVersion = 3

type fileStore struct {
	path        string
	mu          sync.RWMutex
	prefixMu    sync.Mutex
	prefixLocks map[string]*sync.Mutex
	data        map[string]json.RawMessage
	closed      bool
}

type jsonKVStore interface {
	getJSON(key string, out any) error
	putJSON(key string, value any) error
	delete(key string) error
	scan(prefix string) map[string]json.RawMessage
}

// Open opens the file-backed store in dataDir.
func Open(dataDir string) (Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	s := &fileStore{
		path:        filepath.Join(dataDir, "sis.db.json"),
		prefixLocks: make(map[string]*sync.Mutex),
		data:        make(map[string]json.RawMessage),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	if err := s.runMigrations(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *fileStore) Clients() ClientStore {
	return &clientStore{s: s}
}

func (s *fileStore) CustomLists() CustomListStore {
	return &customListStore{s: s}
}

func (s *fileStore) Sessions() SessionStore {
	return &sessionStore{s: s}
}

func (s *fileStore) Stats() StatsStore {
	return &statsStore{s: s}
}

func (s *fileStore) ConfigHistory() ConfigHistoryStore {
	return &configHistoryStore{s: s}
}

func (s *fileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.closed = true
	return nil
}

func (s *fileStore) load() error {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &s.data)
}

func (s *fileStore) runMigrations() error {
	var current int
	_ = s.getJSON("store_meta:schema_version", &current)
	if current >= schemaVersion {
		return nil
	}
	for _, migration := range migrations() {
		if migration.Version <= current {
			continue
		}
		if err := migration.Apply(s); err != nil {
			return err
		}
		if err := s.putJSON("store_meta:schema_version", migration.Version); err != nil {
			return err
		}
	}
	return nil
}

func migrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Apply: func(Store) error {
				return nil
			},
		},
		{
			Version: 2,
			Apply: func(s Store) error {
				sqlite, ok := s.(*sqliteStore)
				if !ok {
					return nil
				}
				return sqlite.ensureCollectionColumn()
			},
		},
		{
			Version: 3,
			Apply: func(s Store) error {
				sqlite, ok := s.(*sqliteStore)
				if !ok {
					return nil
				}
				return sqlite.ensureSQLiteSchema()
			},
		},
	}
}

func (s *fileStore) lockPrefix(prefix string) func() {
	s.prefixMu.Lock()
	mu := s.prefixLocks[prefix]
	if mu == nil {
		mu = &sync.Mutex{}
		s.prefixLocks[prefix] = mu
	}
	s.prefixMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func (s *fileStore) getJSON(key string, out any) error {
	s.mu.RLock()
	raw, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	return json.Unmarshal(raw, out)
}

func (s *fileStore) putJSON(key string, value any) error {
	unlock := s.lockPrefix(prefixOf(key))
	defer unlock()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	s.data[key] = raw
	return s.saveLocked()
}

func (s *fileStore) delete(key string) error {
	unlock := s.lockPrefix(prefixOf(key))
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if _, ok := s.data[key]; !ok {
		return ErrNotFound
	}
	delete(s.data, key)
	return s.saveLocked()
}

func (s *fileStore) scan(prefix string) map[string]json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]json.RawMessage)
	for k, v := range s.data {
		if strings.HasPrefix(k, prefix) {
			out[k] = append(json.RawMessage(nil), v...)
		}
	}
	return out
}

func (s *fileStore) saveLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(s.path)+".*.tmp")
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
	if err := os.Rename(tmpPath, s.path); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func prefixOf(key string) string {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i]
	}
	return key
}

type clientStore struct {
	s jsonKVStore
}

func (c *clientStore) Get(key string) (*Client, error) {
	var out Client
	if err := c.s.getJSON("clients:"+key, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *clientStore) List() ([]*Client, error) {
	rows := c.s.scan("clients:")
	out := make([]*Client, 0, len(rows))
	for _, raw := range rows {
		var client Client
		if err := json.Unmarshal(raw, &client); err != nil {
			return nil, err
		}
		out = append(out, &client)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (c *clientStore) Upsert(client *Client) error {
	if client == nil || client.Key == "" {
		return fmt.Errorf("clients.key: required")
	}
	return c.s.putJSON("clients:"+client.Key, client)
}

func (c *clientStore) Delete(key string) error {
	return c.s.delete("clients:" + key)
}

type customListStore struct {
	s jsonKVStore
}

func (c *customListStore) Add(listID, domain string) error {
	if listID == "" || domain == "" {
		return fmt.Errorf("customlist: list id and domain are required")
	}
	return c.s.putJSON("customlist:"+listID+":"+domain, true)
}

func (c *customListStore) Remove(listID, domain string) error {
	return c.s.delete("customlist:" + listID + ":" + domain)
}

func (c *customListStore) List(listID string) ([]string, error) {
	prefix := "customlist:" + listID + ":"
	rows := c.s.scan(prefix)
	out := make([]string, 0, len(rows))
	for key := range rows {
		out = append(out, strings.TrimPrefix(key, prefix))
	}
	sort.Strings(out)
	return out, nil
}

type sessionStore struct {
	s jsonKVStore
}

func (ss *sessionStore) Get(token string) (*Session, error) {
	var out Session
	if err := ss.s.getJSON("session:"+token, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (ss *sessionStore) Upsert(session *Session) error {
	if session == nil || session.Token == "" {
		return fmt.Errorf("session.token: required")
	}
	return ss.s.putJSON("session:"+session.Token, session)
}

func (ss *sessionStore) Delete(token string) error {
	return ss.s.delete("session:" + token)
}

func (ss *sessionStore) DeleteExpired() error {
	now := time.Now()
	rows := ss.s.scan("session:")
	for key, raw := range rows {
		var session Session
		if err := json.Unmarshal(raw, &session); err != nil {
			return err
		}
		if !session.ExpiresAt.IsZero() && session.ExpiresAt.Before(now) {
			if err := ss.s.delete(key); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}
	return nil
}

type statsStore struct {
	s jsonKVStore
}

func (st *statsStore) Put(granularity, bucket string, row *StatsRow) error {
	if granularity == "" || bucket == "" || row == nil {
		return fmt.Errorf("stats: granularity, bucket, and row are required")
	}
	stored := *row
	stored.Bucket = bucket
	return st.s.putJSON("stats:"+granularity+":"+bucket, &stored)
}

func (st *statsStore) Get(granularity, bucket string) (*StatsRow, error) {
	var out StatsRow
	if err := st.s.getJSON("stats:"+granularity+":"+bucket, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (st *statsStore) List(granularity string) ([]*StatsRow, error) {
	rows := st.s.scan("stats:" + granularity + ":")
	out := make([]*StatsRow, 0, len(rows))
	for _, raw := range rows {
		var row StatsRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, err
		}
		out = append(out, &row)
	}
	sort.Slice(out, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(out[i].Bucket, 10, 64)
		right, rightErr := strconv.ParseInt(out[j].Bucket, 10, 64)
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		return out[i].Bucket < out[j].Bucket
	})
	return out, nil
}

type configHistoryStore struct {
	s jsonKVStore
}

func (ch *configHistoryStore) Append(snapshot *ConfigSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("confhist: snapshot is required")
	}
	stored := *snapshot
	if stored.TS.IsZero() {
		stored.TS = time.Now().UTC()
	}
	return ch.s.putJSON("confhist:"+stored.TS.Format(time.RFC3339Nano), &stored)
}

func (ch *configHistoryStore) List(limit int) ([]*ConfigSnapshot, error) {
	rows := ch.s.scan("confhist:")
	out := make([]*ConfigSnapshot, 0, len(rows))
	for _, raw := range rows {
		var snapshot ConfigSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return nil, err
		}
		out = append(out, &snapshot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS.After(out[j].TS) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
