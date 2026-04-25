package store

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("store: not found")
var ErrClosed = errors.New("store: closed")

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

type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}

type StatsRow struct {
	Bucket   string            `json:"bucket"`
	Counters map[string]uint64 `json:"counters"`
}

type ConfigSnapshot struct {
	TS   time.Time `json:"ts"`
	YAML string    `json:"yaml"`
}
