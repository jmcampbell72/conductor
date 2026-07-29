package middleware

import (
	"context"
	"net/http"
	"strings"

	"conductor/internal/api"
)

type callerKey struct{}

// Auth validates the Bearer token against the provided key map.
// If keys is empty, all requests are allowed through (development mode).
func Auth(keys map[string]string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(keys) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			token := bearerToken(r)
			callerID, ok := keys[token]
			if !ok {
				api.WriteError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
				return
			}

			ctx := context.WithValue(r.Context(), callerKey{}, callerID)
			SetLogField(ctx, "caller_id", callerID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CallerID(ctx context.Context) string {
	v, _ := ctx.Value(callerKey{}).(string)
	return v
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	after, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}
