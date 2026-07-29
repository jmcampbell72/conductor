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
	SimpleRoutes   atomic.Int64
	ComplexRoutes  atomic.Int64
	WritingRoutes  atomic.Int64
	QARoutes       atomic.Int64
	EconomyRoutes  atomic.Int64
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
	WritingRoutes int64 `json:"writing_routes"`
	QARoutes      int64 `json:"qa_routes"`
	EconomyRoutes int64 `json:"economy_routes"`
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
		WritingRoutes: m.WritingRoutes.Load(),
		QARoutes:      m.QARoutes.Load(),
		EconomyRoutes: m.EconomyRoutes.Load(),
	}
}

// Registry accumulates per-caller attribution data and per-route spend.
type Registry struct {
	mu      sync.RWMutex
	callers map[string]*CallerMetrics
	global  CallerMetrics

	// routeSpend tracks accumulated spend in microdollars per route name.
	// Resets on process restart. Used for budget threshold enforcement.
	routeMu    sync.RWMutex
	routeSpend map[string]*atomic.Int64
}

func NewRegistry() *Registry {
	return &Registry{
		callers:    make(map[string]*CallerMetrics),
		routeSpend: make(map[string]*atomic.Int64),
	}
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

// RecordTaskRoute tracks which task-type tier was selected for a request.
func (r *Registry) RecordTaskRoute(callerID, tier string) {
	m := r.getOrCreate(callerID)
	switch tier {
	case "writing":
		r.global.WritingRoutes.Add(1)
		m.WritingRoutes.Add(1)
	case "qa":
		r.global.QARoutes.Add(1)
		m.QARoutes.Add(1)
	case "economy":
		r.global.EconomyRoutes.Add(1)
		m.EconomyRoutes.Add(1)
	}
}

// RecordSpend adds the given dollar cost to the named route's running total.
func (r *Registry) RecordSpend(route string, dollars float64) {
	if dollars <= 0 {
		return
	}
	micros := int64(dollars * 1e6)
	v := r.getOrCreateSpend(route)
	v.Add(micros)
}

// RouteSpend returns the accumulated spend in dollars for the named route.
func (r *Registry) RouteSpend(route string) float64 {
	r.routeMu.RLock()
	v, ok := r.routeSpend[route]
	r.routeMu.RUnlock()
	if !ok {
		return 0
	}
	return float64(v.Load()) / 1e6
}

// StatsResponse is the full telemetry payload returned by the admin endpoint.
type StatsResponse struct {
	Global     Snapshot            `json:"global"`
	Callers    map[string]Snapshot `json:"callers"`
	RouteSpend map[string]float64  `json:"route_spend,omitempty"`
}

// Snapshot returns a point-in-time copy of all metrics.
func (r *Registry) Snapshot() StatsResponse {
	r.mu.RLock()
	callers := make(map[string]Snapshot, len(r.callers))
	for id, m := range r.callers {
		callers[id] = m.snapshot()
	}
	r.mu.RUnlock()

	r.routeMu.RLock()
	spend := make(map[string]float64, len(r.routeSpend))
	for name, v := range r.routeSpend {
		spend[name] = float64(v.Load()) / 1e6
	}
	r.routeMu.RUnlock()

	return StatsResponse{
		Global:     r.global.snapshot(),
		Callers:    callers,
		RouteSpend: spend,
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

func (r *Registry) getOrCreateSpend(route string) *atomic.Int64 {
	r.routeMu.RLock()
	v, ok := r.routeSpend[route]
	r.routeMu.RUnlock()
	if ok {
		return v
	}
	r.routeMu.Lock()
	defer r.routeMu.Unlock()
	if v, ok = r.routeSpend[route]; ok {
		return v
	}
	v = &atomic.Int64{}
	r.routeSpend[route] = v
	return v
}
