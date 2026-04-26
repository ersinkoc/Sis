package policy

import "github.com/ersinkoc/sis/internal/config"

// RegisterReload registers the engine as a config reload callback.
func (e *Engine) RegisterReload(r *config.Reloader) {
	if e == nil || r == nil {
		return
	}
	r.Register(func(old, next *config.Config) error {
		return e.ReloadConfig(next)
	})
}
