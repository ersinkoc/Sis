package config

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type ReloadCallback func(old, new *Config) error

type Reloader struct {
	loader    *Loader
	holder    *Holder
	callbacks []ReloadCallback
	mu        sync.Mutex
}

func NewReloader(loader *Loader, holder *Holder) *Reloader {
	return &Reloader{loader: loader, holder: holder}
}

func (r *Reloader) Register(cb ReloadCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, cb)
}

func (r *Reloader) Reload() error {
	next, err := r.loader.Load()
	if err != nil {
		return err
	}
	old := r.holder.Get()
	r.mu.Lock()
	callbacks := append([]ReloadCallback(nil), r.callbacks...)
	r.mu.Unlock()
	for _, cb := range callbacks {
		if err := cb(old, next); err != nil {
			return err
		}
	}
	r.holder.Replace(next)
	return nil
}

func (r *Reloader) WatchSIGHUP(ctx context.Context, logger *slog.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			if err := r.Reload(); err != nil {
				if logger != nil {
					logger.Error("config reload failed", "error", err)
				}
				continue
			}
			if logger != nil {
				logger.Info("config reloaded")
			}
		}
	}
}
