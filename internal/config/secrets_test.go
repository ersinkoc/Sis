package config

import "testing"

func TestEnsureLogSalt(t *testing.T) {
	cfg := &Config{Privacy: Privacy{LogMode: "hashed"}}
	changed, err := EnsureLogSalt(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || cfg.Privacy.LogSalt == "" {
		t.Fatalf("expected generated salt, changed=%v salt=%q", changed, cfg.Privacy.LogSalt)
	}
	salt := cfg.Privacy.LogSalt
	changed, err = EnsureLogSalt(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if changed || cfg.Privacy.LogSalt != salt {
		t.Fatalf("salt should be stable, changed=%v salt=%q", changed, cfg.Privacy.LogSalt)
	}
}

func TestEnsureLogSaltOnlyHashedMode(t *testing.T) {
	cfg := &Config{Privacy: Privacy{LogMode: "full"}}
	changed, err := EnsureLogSalt(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if changed || cfg.Privacy.LogSalt != "" {
		t.Fatalf("unexpected salt for full mode: changed=%v salt=%q", changed, cfg.Privacy.LogSalt)
	}
}

func TestRedactedCopyHidesSecretsWithoutMutatingSource(t *testing.T) {
	cfg := &Config{
		Privacy: Privacy{LogSalt: "salt"},
		Auth:    Auth{Users: []User{{Username: "admin", PasswordHash: "hash"}}},
	}
	redacted := RedactedCopy(cfg)
	if redacted.Auth.Users[0].PasswordHash != "redacted" || redacted.Privacy.LogSalt != "redacted" {
		t.Fatalf("redacted copy = %#v", redacted)
	}
	if cfg.Auth.Users[0].PasswordHash != "hash" || cfg.Privacy.LogSalt != "salt" {
		t.Fatalf("source mutated = %#v", cfg)
	}
}
