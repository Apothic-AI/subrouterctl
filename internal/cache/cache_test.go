package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDisabledStoreNeverServes(t *testing.T) {
	s := New(t.TempDir(), 0)
	if s.Enabled() {
		t.Error("a store with a zero TTL must be disabled")
	}
	if _, ok := s.Get("abc"); ok {
		t.Error("a disabled store must not serve anything")
	}
	if err := s.Put("abc", []byte(`{"x":1}`)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 0 {
		t.Errorf("a disabled store wrote %d files", len(entries))
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := New(t.TempDir(), time.Minute)
	body := []byte(`{"data":[{"id":1}]}`)
	if err := s.Put("k", body); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := s.Get("k")
	if !ok {
		t.Fatal("Get missed a fresh entry")
	}
	if string(got) != string(body) {
		t.Errorf("Get = %s, want %s", got, body)
	}
}

func TestStoreExpires(t *testing.T) {
	s := New(t.TempDir(), time.Nanosecond)
	if err := s.Put("k", []byte(`{}`)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, ok := s.Get("k"); ok {
		t.Error("an expired entry must not be served")
	}
}

func TestStoreClear(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, time.Minute)
	for _, k := range []string{"a", "b", "c"} {
		if err := s.Put(k, []byte(`{}`)); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("Clear left %d files", len(entries))
	}
}

func TestClearOnAMissingDirectory(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "does-not-exist"), time.Minute)
	if err := s.Clear(); err != nil {
		t.Errorf("Clear on a missing directory: %v", err)
	}
}

func TestKeyIsStableAndDistinct(t *testing.T) {
	a := Key("GET", "/api/marketplace/models", "page=1")
	b := Key("GET", "/api/marketplace/models", "page=1")
	c := Key("GET", "/api/marketplace/models", "page=2")
	d := Key("GET", "/api/marketplace/providers", "page=1")
	if a != b {
		t.Error("the same request must produce the same key")
	}
	if a == c || a == d {
		t.Error("different requests must produce different keys")
	}
	if len(a) != 32 {
		t.Errorf("key length = %d, want 32", len(a))
	}
}
