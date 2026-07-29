// HTTP handlers for the /admin/* management API. Provides endpoints for
// per-caller telemetry, cache stats, and live route configuration updates.
//
// Author: Justin Campbell
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"conductor/internal/cache"
	"conductor/internal/config"
	"conductor/internal/telemetry"
)

// Handler serves the /admin/* management API.
//
// Endpoints:
//   GET  /admin/stats          — per-caller telemetry snapshot
//   GET  /admin/cache/stats    — exact cache hit/miss counters
//   GET  /admin/routes         — current routes file (read-only)
//   PATCH /admin/routes/default — update default route config at runtime
//
// All endpoints require the admin API key in the Authorization: Bearer header.
type Handler struct {
	tele       *telemetry.Registry
	kv         cache.Store
	routeStore *config.RouteStore
	adminKey   string
}

func NewHandler(
	tele *telemetry.Registry,
	kv cache.Store,
	routeStore *config.RouteStore,
	adminKey string,
) *Handler {
	return &Handler{
		tele:       tele,
		kv:         kv,
		routeStore: routeStore,
		adminKey:   adminKey,
	}
}

// Register mounts the admin routes on mux under the /admin/ prefix.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/admin/stats", h.auth(http.HandlerFunc(h.stats)))
	mux.Handle("/admin/cache/stats", h.auth(http.HandlerFunc(h.cacheStats)))
	mux.Handle("/admin/routes", h.auth(http.HandlerFunc(h.routes)))
	mux.Handle("/admin/routes/default", h.auth(http.HandlerFunc(h.routesDefault)))
}

// ── auth middleware ───────────────────────────────────────────────────────────

func (h *Handler) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.adminKey == "" {
			// No key configured — admin endpoints disabled.
			http.Error(w, `{"error":"admin API not configured"}`, http.StatusForbidden)
			return
		}
		token := bearerToken(r)
		if token != h.adminKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	after, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}

// callerFrom reads the admin caller identity from the request for audit logging.
func callerFrom(r *http.Request) string {
	// In the admin layer, use the remote address as identity since
	// the admin key is shared, not per-caller.
	return r.RemoteAddr
}

// ── handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.tele.Snapshot())
}

func (h *Handler) cacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.kv.Stats())
}

func (h *Handler) routes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.routeStore.Snapshot())
}

func (h *Handler) routesDefault(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, h.routeStore.Default())

	case http.MethodPatch:
		// Merge the patch into the current config: decode partial JSON over the
		// existing RouteConfig so omitted fields keep their current values.
		current := h.routeStore.Default()
		// Marshal current → unmarshal patch on top (JSON merge-patch behaviour).
		base, err := json.Marshal(current)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(base, &current); err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&current); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		h.routeStore.UpdateDefault(current)

		slog.Info("admin.audit",
			"action", "update_default_route",
			"caller", callerFrom(r),
			"new_config", current,
		)
		writeJSON(w, current)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
