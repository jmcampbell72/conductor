// Generates random 16-character hex request IDs and propagates them through
// the request context for structured logging correlation.
//
// Author: Justin Campbell
package reqid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type contextKey struct{}

func New() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKey{}).(string)
	return v
}
