// Redis-backed cache Store implementation. Values are JSON-encoded and stored
// with a 24-hour TTL; cache write failures are non-fatal best-effort.
//
// Author: Justin Campbell
package cache

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"conductor/internal/api"
	"conductor/internal/redis"
)

// RedisStore implements Store using Redis as the backing cache.
// Values are JSON-encoded so they are human-readable in Redis and
// require no gob type registration.
type RedisStore struct {
	client *redis.Client
	hits   atomic.Int64
	misses atomic.Int64
}

// NewRedis returns a RedisStore that connects to addr (e.g. "localhost:6379").
// The connection is established lazily on first use.
func NewRedis(addr string) *RedisStore {
	return &RedisStore{client: redis.New(addr)}
}

func (r *RedisStore) Get(key string) (*api.ChatCompletionResponse, bool) {
	data, err := r.client.Get(key)
	if err != nil {
		if !errors.Is(err, redis.ErrNil) {
			// Log or swallow transient errors; fall through to miss.
			_ = err
		}
		r.misses.Add(1)
		return nil, false
	}

	var resp api.ChatCompletionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		r.misses.Add(1)
		return nil, false
	}

	r.hits.Add(1)
	return &resp, true
}

func (r *RedisStore) Set(key string, resp *api.ChatCompletionResponse, ttl time.Duration) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = r.client.Set(key, data, ttl) // best-effort; cache write failures are non-fatal
}

func (r *RedisStore) Stats() Stats {
	hits := r.hits.Load()
	misses := r.misses.Load()
	total := hits + misses
	var ratio float64
	if total > 0 {
		ratio = float64(hits) / float64(total)
	}
	return Stats{
		Hits:     hits,
		Misses:   misses,
		HitRatio: ratio,
		// Entries count would require a DBSIZE call; omit to keep Get/Set O(1).
	}
}

// Ping checks whether the Redis server is reachable.
func (r *RedisStore) Ping() error {
	return r.client.Ping()
}
