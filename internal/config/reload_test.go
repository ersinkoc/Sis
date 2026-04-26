package config

import (
	"strings"
	"testing"
)

func TestReloaderReloadRequiresDependencies(t *testing.T) {
	err := (*Reloader)(nil).Reload()
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("nil reloader err = %v", err)
	}
	err = (&Reloader{}).Reload()
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("empty reloader err = %v", err)
	}
}
