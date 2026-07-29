// Thread-safe store for live route configuration. The handler reads from it
// on every request; the admin API writes to it without requiring a restart.
//
// Author: Justin Campbell
package config

import "sync"

// RouteStore holds the live route configuration and allows atomic runtime updates.
// The handler reads from it on every request; the admin API writes to it.
type RouteStore struct {
	mu     sync.RWMutex
	routes RoutesFile
}

func NewRouteStore(initial RoutesFile) *RouteStore {
	return &RouteStore{routes: initial}
}

// Default returns the current default route config.
func (rs *RouteStore) Default() RouteConfig {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.routes.Default
}

// Named returns the named route config and whether it exists.
func (rs *RouteStore) Named(name string) (RouteConfig, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	r, ok := rs.routes.Named[name]
	return r, ok
}

// Update atomically replaces the full routes file.
func (rs *RouteStore) Update(routes RoutesFile) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.routes = routes
}

// UpdateDefault atomically replaces only the default route config.
func (rs *RouteStore) UpdateDefault(r RouteConfig) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.routes.Default = r
}

// Snapshot returns a point-in-time copy of the full routes file.
func (rs *RouteStore) Snapshot() RoutesFile {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.routes
}
