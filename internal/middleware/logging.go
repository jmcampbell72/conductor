// Structured JSON request logging middleware. Emits one slog entry per request
// with timing, HTTP status, model, and cache outcome fields.
//
// Author: Justin Campbell
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"conductor/internal/reqid"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) written() int {
	if rw.status == 0 {
		return http.StatusOK
	}
	return rw.status
}

func Logging() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			id := reqid.New()
			ctx, fields := withFields(r.Context())
			ctx = reqid.WithID(ctx, id)
			rw := &responseWriter{ResponseWriter: w}

			next.ServeHTTP(rw, r.WithContext(ctx))

			fields.mu.Lock()
			callerID := fields.CallerID
			model := fields.Model
			cacheStatus := fields.Cache
			fields.mu.Unlock()

			attrs := []any{
				"req_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.written(),
				"duration_ms", time.Since(start).Milliseconds(),
			}
			if callerID != "" {
				attrs = append(attrs, "caller_id", callerID)
			}
			if model != "" {
				attrs = append(attrs, "model", model)
			}
			if cacheStatus != "" {
				attrs = append(attrs, "cache", cacheStatus)
			}
			slog.Info("request", attrs...)
		})
	}
}
