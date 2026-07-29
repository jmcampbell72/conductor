package middleware

import "net/http"

type Middleware func(http.Handler) http.Handler

// Chain applies middlewares in order: the first middleware is outermost
// (runs first on the way in, last on the way out).
func Chain(ms ...Middleware) Middleware {
	return func(final http.Handler) http.Handler {
		for i := len(ms) - 1; i >= 0; i-- {
			final = ms[i](final)
		}
		return final
	}
}
