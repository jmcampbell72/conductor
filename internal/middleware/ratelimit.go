package middleware

import (
	"net/http"
	"sync"
	"time"

	"conductor/internal/api"
)

type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	rate     float64
	last     time.Time
}

func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens += now.Sub(b.last).Seconds() * b.rate
	b.last = now
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	rate     float64
	capacity float64
}

func (rl *rateLimiter) bucket(id string) *tokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[id]
	if !ok {
		b = &tokenBucket{
			tokens:   rl.capacity,
			capacity: rl.capacity,
			rate:     rl.rate,
			last:     time.Now(),
		}
		rl.buckets[id] = b
	}
	return b
}

// RateLimit enforces a per-caller token-bucket limit of rps requests per second
// with an initial burst capacity.
func RateLimit(rps float64, burst int) Middleware {
	rl := &rateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     rps,
		capacity: float64(burst),
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := CallerID(r.Context())
			if id == "" {
				id = r.RemoteAddr
			}
			if !rl.bucket(id).allow() {
				api.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
