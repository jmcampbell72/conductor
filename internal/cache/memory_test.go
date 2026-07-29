// Tests for the in-memory cache: set/get, TTL expiry, hit/miss stats, and
// gob-snapshot persistence.
//
// Author: Justin Campbell
package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"conductor/internal/api"
)

func makeResp(content string) *api.ChatCompletionResponse {
	return &api.ChatCompletionResponse{
		ID:    "test-id",
		Model: "gpt-4o-mini",
		Choices: []api.ChatCompletionChoice{
			{Message: api.Message{Role: "assistant", Content: content}},
		},
	}
}

func TestMemory_SetGet(t *testing.T) {
	m := NewMemory()
	resp := makeResp("hello")
	m.Set("k1", resp, time.Hour)

	got, ok := m.Get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Choices[0].Message.Content != "hello" {
		t.Fatalf("got %q, want %q", got.Choices[0].Message.Content, "hello")
	}
}

func TestMemory_Miss(t *testing.T) {
	m := NewMemory()
	_, ok := m.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestMemory_Expiry(t *testing.T) {
	m := NewMemory()
	m.Set("k1", makeResp("expires"), time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	_, ok := m.Get("k1")
	if ok {
		t.Fatal("expected expired entry to be a miss")
	}
}

func TestMemory_Stats(t *testing.T) {
	m := NewMemory()
	m.Set("k1", makeResp("a"), time.Hour)

	m.Get("k1") // hit
	m.Get("k1") // hit
	m.Get("k2") // miss

	s := m.Stats()
	if s.Hits != 2 {
		t.Errorf("hits: got %d, want 2", s.Hits)
	}
	if s.Misses != 1 {
		t.Errorf("misses: got %d, want 1", s.Misses)
	}
	if s.Entries != 1 {
		t.Errorf("entries: got %d, want 1", s.Entries)
	}
}

func TestMemory_Persist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.gob")

	m1 := NewMemory()
	m1.Set("k1", makeResp("persisted"), time.Hour)
	if err := m1.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	m2 := NewMemory()
	if err := m2.Load(path); err != nil {
		t.Fatalf("load: %v", err)
	}

	got, ok := m2.Get("k1")
	if !ok {
		t.Fatal("expected cache hit after load")
	}
	if got.Choices[0].Message.Content != "persisted" {
		t.Fatalf("got %q, want %q", got.Choices[0].Message.Content, "persisted")
	}
}

func TestMemory_LoadMissingFile(t *testing.T) {
	m := NewMemory()
	if err := m.Load("/nonexistent/path/cache.gob"); err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
}

func TestMemory_SaveAtomicity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.gob")

	m := NewMemory()
	m.Set("k1", makeResp("atomic"), time.Hour)
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}

	// No tmp file should remain after a successful save.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "cache.gob" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}
