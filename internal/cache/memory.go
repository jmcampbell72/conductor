package cache

import (
	"context"
	"encoding/gob"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"conductor/internal/api"
)

func init() {
	// Register concrete types used inside api so gob can encode them.
	gob.Register(api.ChatCompletionResponse{})
}

type entry struct {
	Response  api.ChatCompletionResponse
	ExpiresAt time.Time
}

// Memory is a thread-safe in-memory cache with TTL eviction and optional
// file-backed persistence via gob snapshots.
type Memory struct {
	mu      sync.RWMutex
	entries map[string]entry
	hits    atomic.Int64
	misses  atomic.Int64
}

func NewMemory() *Memory {
	return &Memory{entries: make(map[string]entry)}
}

func (m *Memory) Get(key string) (*api.ChatCompletionResponse, bool) {
	m.mu.RLock()
	e, ok := m.entries[key]
	m.mu.RUnlock()

	if !ok || time.Now().After(e.ExpiresAt) {
		m.misses.Add(1)
		return nil, false
	}
	m.hits.Add(1)
	resp := e.Response
	return &resp, true
}

func (m *Memory) Set(key string, resp *api.ChatCompletionResponse, ttl time.Duration) {
	m.mu.Lock()
	m.entries[key] = entry{Response: *resp, ExpiresAt: time.Now().Add(ttl)}
	m.mu.Unlock()
}

func (m *Memory) Stats() Stats {
	m.mu.RLock()
	size := int64(len(m.entries))
	m.mu.RUnlock()
	hits := m.hits.Load()
	misses := m.misses.Load()
	total := hits + misses
	var ratio float64
	if total > 0 {
		ratio = float64(hits) / float64(total)
	}
	return Stats{
		Hits:     hits,
		Misses:   misses,
		Entries:  size,
		HitRatio: ratio,
	}
}

// StartEviction sweeps expired entries on interval until ctx is cancelled.
func (m *Memory) StartEviction(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				m.evict()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (m *Memory) evict() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.entries {
		if now.After(e.ExpiresAt) {
			delete(m.entries, k)
		}
	}
}

// StartPersist saves a snapshot to path on interval and once more on shutdown.
func (m *Memory) StartPersist(ctx context.Context, path string, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := m.Save(path); err != nil {
					slog.Warn("cache persist failed", "err", err)
				}
			case <-ctx.Done():
				if err := m.Save(path); err != nil {
					slog.Warn("cache final persist failed", "err", err)
				}
				return
			}
		}
	}()
}

type snapshot struct {
	Entries map[string]entry
}

// Save writes non-expired entries to path using an atomic rename so a crash
// during the write never leaves a corrupt file.
func (m *Memory) Save(path string) error {
	now := time.Now()
	m.mu.RLock()
	snap := snapshot{Entries: make(map[string]entry, len(m.entries))}
	for k, e := range m.entries {
		if !now.After(e.ExpiresAt) {
			snap.Entries[k] = e
		}
	}
	m.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cache-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if err := gob.NewEncoder(tmp).Encode(snap); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Load reads entries from path, skipping any that have expired.
// A missing file is silently ignored.
func (m *Memory) Load(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	var snap snapshot
	if err := gob.NewDecoder(f).Decode(&snap); err != nil {
		return err
	}

	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range snap.Entries {
		if !now.After(e.ExpiresAt) {
			m.entries[k] = e
		}
	}
	return nil
}
