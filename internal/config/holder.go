package config

import "sync/atomic"

type Holder struct {
	cur atomic.Pointer[Config]
}

func NewHolder(c *Config) *Holder {
	h := &Holder{}
	h.Replace(c)
	return h
}

func (h *Holder) Get() *Config {
	return h.cur.Load()
}

func (h *Holder) Replace(c *Config) {
	h.cur.Store(c)
}
