package store

type Store interface {
	Clients() ClientStore
	CustomLists() CustomListStore
	Sessions() SessionStore
	Stats() StatsStore
	ConfigHistory() ConfigHistoryStore
	Close() error
}

type ClientStore interface {
	Get(key string) (*Client, error)
	List() ([]*Client, error)
	Upsert(*Client) error
	Delete(key string) error
}

type CustomListStore interface {
	Add(listID, domain string) error
	Remove(listID, domain string) error
	List(listID string) ([]string, error)
}

type SessionStore interface {
	Get(token string) (*Session, error)
	Upsert(*Session) error
	Delete(token string) error
	DeleteExpired() error
}

type StatsStore interface {
	Put(granularity, bucket string, row *StatsRow) error
	Get(granularity, bucket string) (*StatsRow, error)
	List(granularity string) ([]*StatsRow, error)
}

type ConfigHistoryStore interface {
	Append(*ConfigSnapshot) error
	List(limit int) ([]*ConfigSnapshot, error)
}

type Migration struct {
	Version int
	Apply   func(Store) error
}
