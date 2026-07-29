// Defines the Store interface and Stats type shared by all cache backends.
//
// Author: Justin Campbell
package cache

import (
	"time"

	"conductor/internal/api"
)

// Store is the interface every cache backend must satisfy.
// The seam exists so Redis can replace Memory in Phase 7 with no handler changes.
type Store interface {
	Get(key string) (*api.ChatCompletionResponse, bool)
	Set(key string, resp *api.ChatCompletionResponse, ttl time.Duration)
	Stats() Stats
}

type Stats struct {
	Hits     int64   `json:"hits"`
	Misses   int64   `json:"misses"`
	Entries  int64   `json:"entries"`
	HitRatio float64 `json:"hit_ratio"`
}
