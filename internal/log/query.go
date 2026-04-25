package log

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
)

type Filter struct {
	Client  string
	QName   string
	Blocked *bool
	Limit   int
}

type Query struct {
	mu      sync.Mutex
	rotator *Rotator
	enc     *json.Encoder
	enabled bool
	mode    string
	salt    []byte
	fanout  *fanout
}

func OpenQuery(c *config.Config) (*Query, error) {
	q := &Query{
		fanout: newFanout(256),
	}
	if err := q.Reconfigure(c); err != nil {
		return nil, err
	}
	return q, nil
}

func (q *Query) Reconfigure(c *config.Config) error {
	if q == nil || c == nil {
		return nil
	}
	mode := c.Privacy.LogMode
	if mode == "" {
		mode = "full"
	}
	salt := []byte(c.Privacy.LogSalt)
	if mode == "hashed" && len(salt) == 0 {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return err
		}
	}
	var nextRotator *Rotator
	var nextEncoder *json.Encoder
	if c.Logging.QueryLog {
		maxBytes := int64(c.Logging.RotateSizeMB) * 1024 * 1024
		rotator, err := NewRotator(filepath.Join(c.Server.DataDir, "logs", "sis-query.log"), maxBytes, c.Logging.RetentionDays, c.Logging.Gzip)
		if err != nil {
			return err
		}
		nextRotator = rotator
		nextEncoder = json.NewEncoder(rotator)
	}
	q.mu.Lock()
	oldRotator := q.rotator
	q.enabled = c.Logging.QueryLog
	q.mode = mode
	q.salt = salt
	q.rotator = nextRotator
	q.enc = nextEncoder
	q.mu.Unlock()
	if oldRotator != nil {
		_ = oldRotator.Close()
	}
	return nil
}

func (q *Query) Write(e *Entry) error {
	if q == nil || e == nil {
		return nil
	}
	entry := e.clone()
	if entry.TS.IsZero() {
		entry.TS = time.Now().UTC()
	}
	q.applyPrivacy(&entry)
	q.fanout.publish(entry)
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.enabled || q.enc == nil {
		return nil
	}
	return q.enc.Encode(&entry)
}

func (q *Query) Subscribe(size int) Subscription {
	return q.SubscribeReplay(size, false)
}

func (q *Query) SubscribeReplay(size int, replay bool) Subscription {
	if q == nil || q.fanout == nil {
		ch := make(Subscription)
		close(ch)
		return ch
	}
	return q.fanout.subscribe(size, replay)
}

func (q *Query) Unsubscribe(sub Subscription) {
	if q == nil || q.fanout == nil {
		return
	}
	q.fanout.unsubscribe(sub)
}

func (q *Query) Recent(filter Filter) []Entry {
	if q == nil || q.fanout == nil {
		return nil
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	entries := q.fanout.snapshot()
	out := make([]Entry, 0, filter.Limit)
	for i := len(entries) - 1; i >= 0 && len(out) < filter.Limit; i-- {
		entry := entries[i]
		if filter.Client != "" && entry.ClientKey != filter.Client && entry.ClientIP != filter.Client && entry.ClientName != filter.Client {
			continue
		}
		if filter.QName != "" && !containsFold(entry.QName, filter.QName) {
			continue
		}
		if filter.Blocked != nil && entry.Blocked != *filter.Blocked {
			continue
		}
		out = append(out, entry)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (q *Query) Rotate() error {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.rotator == nil {
		return nil
	}
	return q.rotator.Rotate()
}

func (q *Query) Close() error {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.rotator == nil {
		return nil
	}
	return q.rotator.Close()
}

func (q *Query) applyPrivacy(e *Entry) {
	q.mu.Lock()
	mode := q.mode
	salt := append([]byte(nil), q.salt...)
	q.mu.Unlock()
	switch mode {
	case "hashed":
		e.ClientKey = hashValue(salt, e.ClientKey)
		e.ClientIP = hashValue(salt, e.ClientIP)
	case "anonymous":
		e.ClientKey = ""
		e.ClientName = ""
		e.ClientGroup = ""
		e.ClientIP = ""
	}
}

func hashValue(salt []byte, value string) string {
	if value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func containsFold(s, substr string) bool {
	if substr == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
