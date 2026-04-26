package store

// Store groups the durable repositories used by the Sis runtime.
type Store interface {
	Clients() ClientStore
	CustomLists() CustomListStore
	Sessions() SessionStore
	Stats() StatsStore
	ConfigHistory() ConfigHistoryStore
	Close() error
}

// ClientStore persists discovered and configured client metadata.
type ClientStore interface {
	Get(key string) (*Client, error)
	List() ([]*Client, error)
	Upsert(*Client) error
	Delete(key string) error
}

// CustomListStore stores user-managed allow/block list entries.
type CustomListStore interface {
	Add(listID, domain string) error
	Remove(listID, domain string) error
	List(listID string) ([]string, error)
}

// SessionStore persists server-side HTTP authentication sessions.
type SessionStore interface {
	Get(token string) (*Session, error)
	Upsert(*Session) error
	Delete(token string) error
	DeleteExpired() error
}

// StatsStore stores aggregated counter rows by granularity and bucket.
type StatsStore interface {
	Put(granularity, bucket string, row *StatsRow) error
	Get(granularity, bucket string) (*StatsRow, error)
	List(granularity string) ([]*StatsRow, error)
}

// ConfigHistoryStore stores snapshots written after config mutations and reloads.
type ConfigHistoryStore interface {
	Append(*ConfigSnapshot) error
	List(limit int) ([]*ConfigSnapshot, error)
}

// Migration describes a store schema migration.
type Migration struct {
	Version int
	Apply   func(Store) error
}
