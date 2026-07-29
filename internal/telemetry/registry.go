package telemetry

import (
	"strings"
	"sync"
	"sync/atomic"
)

// CallerMetrics holds per-caller request counters.
type CallerMetrics struct {
	Requests atomic.Int64
	Tokens   atomic.Int64
	// cache outcomes
	ExactHits    atomic.Int64
	SemanticHits atomic.Int64
	Misses       atomic.Int64
	// routing outcomes
	SimpleRoutes  atomic.Int64
	ComplexRoutes atomic.Int64
}

// Snapshot is a point-in-time copy safe for JSON marshalling.
type Snapshot struct {
	Requests      int64 `json:"requests"`
	Tokens        int64 `json:"tokens"`
	ExactHits     int64 `json:"exact_hits"`
	SemanticHits  int64 `json:"semantic_hits"`
	Misses        int64 `json:"misses"`
	SimpleRoutes  int64 `json:"simple_routes"`
	ComplexRoutes int64 `json:"complex_routes"`
}

func (m *CallerMetrics) snapshot() Snapshot {
	return Snapshot{
		Requests:      m.Requests.Load(),
		Tokens:        m.Tokens.Load(),
		ExactHits:     m.ExactHits.Load(),
		SemanticHits:  m.SemanticHits.Load(),
		Misses:        m.Misses.Load(),
		SimpleRoutes:  m.SimpleRoutes.Load(),
		ComplexRoutes: m.ComplexRoutes.Load(),
	}
}

// Registry accumulates per-caller attribution data.
type Registry struct {
	mu      sync.RWMutex
	callers map[string]*CallerMetrics
	global  CallerMetrics
}

func NewRegistry() *Registry {
	return &Registry{callers: make(map[string]*CallerMetrics)}
}

// Record tallies a completed request.
// callerID may be empty (dev mode); model selects the routing bucket;
// cacheStatus is "hit", "miss", or a "semantic-hit:N.NN" prefix;
// tokens is the total token count from the provider response (0 for cache hits
// without usage data).
func (r *Registry) Record(callerID, model, cacheStatus string, tokens int) {
	m := r.getOrCreate(callerID)
	r.global.Requests.Add(1)
	m.Requests.Add(1)

	r.global.Tokens.Add(int64(tokens))
	m.Tokens.Add(int64(tokens))

	switch {
	case cacheStatus == "hit":
		r.global.ExactHits.Add(1)
		m.ExactHits.Add(1)
	case strings.HasPrefix(cacheStatus, "semantic-hit"):
		r.global.SemanticHits.Add(1)
		m.SemanticHits.Add(1)
	default:
		r.global.Misses.Add(1)
		m.Misses.Add(1)
	}

	// Bucket model tier by name prefix.
	switch {
	case strings.HasPrefix(model, "gpt-4o-mini"), strings.HasPrefix(model, "gpt-3"):
		r.global.SimpleRoutes.Add(1)
		m.SimpleRoutes.Add(1)
	case model != "":
		r.global.ComplexRoutes.Add(1)
		m.ComplexRoutes.Add(1)
	}
}

// StatsResponse is the full telemetry payload returned by the admin endpoint.
type StatsResponse struct {
	Global  Snapshot            `json:"global"`
	Callers map[string]Snapshot `json:"callers"`
}

// Snapshot returns a point-in-time copy of all metrics.
func (r *Registry) Snapshot() StatsResponse {
	r.mu.RLock()
	defer r.mu.RUnlock()
	callers := make(map[string]Snapshot, len(r.callers))
	for id, m := range r.callers {
		callers[id] = m.snapshot()
	}
	return StatsResponse{
		Global:  r.global.snapshot(),
		Callers: callers,
	}
}

func (r *Registry) getOrCreate(callerID string) *CallerMetrics {
	if callerID == "" {
		callerID = "_anonymous"
	}
	r.mu.RLock()
	m, ok := r.callers[callerID]
	r.mu.RUnlock()
	if ok {
		return m
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok = r.callers[callerID]; ok {
		return m
	}
	m = &CallerMetrics{}
	r.callers[callerID] = m
	return m
}
