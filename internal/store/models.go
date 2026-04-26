package store

import (
	"errors"
	"time"
)

// ErrNotFound reports that a requested store record does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrClosed reports that a write was attempted after the store was closed.
var ErrClosed = errors.New("store: closed")

// Client is the durable metadata Sis tracks for a DNS client.
type Client struct {
	Key       string    `json:"key"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Group     string    `json:"group"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	LastIP    string    `json:"last_ip"`
	Hidden    bool      `json:"hidden"`
}

// Session is a server-side authentication session referenced by cookie token.
type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StatsRow stores aggregated counters for a single time bucket.
type StatsRow struct {
	Bucket   string            `json:"bucket"`
	Counters map[string]uint64 `json:"counters"`
}

// ConfigSnapshot stores a YAML rendering of a runtime configuration revision.
type ConfigSnapshot struct {
	TS   time.Time `json:"ts"`
	YAML string    `json:"yaml"`
}
