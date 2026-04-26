package config

import (
	"crypto/rand"
	"encoding/base64"
)

// EnsureLogSalt generates a persistent salt when hashed query logging is enabled.
func EnsureLogSalt(c *Config) (bool, error) {
	if c == nil || c.Privacy.LogMode != "hashed" || c.Privacy.LogSalt != "" {
		return false, nil
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return false, err
	}
	c.Privacy.LogSalt = base64.RawStdEncoding.EncodeToString(raw[:])
	return true, nil
}
