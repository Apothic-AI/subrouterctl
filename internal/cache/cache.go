// Package cache stores public directory responses on disk.
//
// The cache is opt-in and only ever holds responses that are public and
// slow to fetch, such as the marketplace model directory. Account state,
// keys, balances and logs are never cached: they must always be live.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Store is an on-disk cache with a time to live.
type Store struct {
	Dir string
	TTL time.Duration
}

// New builds a store rooted at dir. A TTL of zero disables the cache.
func New(dir string, ttl time.Duration) *Store {
	return &Store{Dir: dir, TTL: ttl}
}

// Enabled reports whether the store will serve anything.
func (s *Store) Enabled() bool { return s != nil && s.TTL > 0 && s.Dir != "" }

// Key derives a filesystem safe key from a request path and its query.
func Key(method, path, query string) string {
	sum := sha256.Sum256([]byte(method + " " + path + "?" + query))
	return hex.EncodeToString(sum[:])[:32]
}

func (s *Store) path(key string) string {
	return filepath.Join(s.Dir, key+".json")
}

// Get returns a cached response when it is still fresh.
func (s *Store) Get(key string) ([]byte, bool) {
	if !s.Enabled() {
		return nil, false
	}
	b, err := os.ReadFile(s.path(key))
	if err != nil {
		return nil, false
	}
	var entry struct {
		StoredAt int64           `json:"stored_at"`
		Body     json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil, false
	}
	age := time.Since(time.Unix(entry.StoredAt, 0))
	if age > s.TTL {
		return nil, false
	}
	return entry.Body, true
}

// Put stores a response with the current timestamp.
func (s *Store) Put(key string, body []byte) error {
	if !s.Enabled() {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	entry := struct {
		StoredAt int64           `json:"stored_at"`
		Body     json.RawMessage `json:"body"`
	}{StoredAt: time.Now().Unix(), Body: body}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(key), b, 0o600)
}

// Clear removes every cached entry.
func (s *Store) Clear() error {
	if s == nil || s.Dir == "" {
		return nil
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(s.Dir, e.Name())); err != nil {
			return fmt.Errorf("remove %s: %w", e.Name(), err)
		}
	}
	return nil
}
