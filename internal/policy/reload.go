package policy

import "github.com/ersinkoc/sis/internal/config"

func (e *Engine) RegisterReload(r *config.Reloader) {
	if e == nil || r == nil {
		return
	}
	r.Register(func(old, next *config.Config) error {
		return e.ReloadConfig(next)
	})
}
