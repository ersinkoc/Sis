package config

import "sync/atomic"

// Holder stores the active configuration behind an atomic pointer.
type Holder struct {
	cur atomic.Pointer[Config]
}

// NewHolder creates a Holder initialized with c.
func NewHolder(c *Config) *Holder {
	h := &Holder{}
	h.Replace(c)
	return h
}

// Get returns the current configuration pointer.
func (h *Holder) Get() *Config {
	if h == nil {
		return nil
	}
	return h.cur.Load()
}

// Replace atomically swaps the active configuration.
func (h *Holder) Replace(c *Config) {
	if h == nil {
		return
	}
	h.cur.Store(c)
}
