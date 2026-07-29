package middleware

import (
	"context"
	"sync"
)

type logFields struct {
	mu       sync.Mutex
	CallerID string
	Model    string
	Cache    string // "hit" | "miss"
}

type fieldsKey struct{}

func withFields(ctx context.Context) (context.Context, *logFields) {
	f := &logFields{}
	return context.WithValue(ctx, fieldsKey{}, f), f
}

func fieldsFrom(ctx context.Context) *logFields {
	v, _ := ctx.Value(fieldsKey{}).(*logFields)
	return v
}

// SetLogField writes a named field into the log record for the current request.
// Recognised keys: "caller_id", "model", "cache".
func SetLogField(ctx context.Context, key, value string) {
	f := fieldsFrom(ctx)
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch key {
	case "caller_id":
		f.CallerID = value
	case "model":
		f.Model = value
	case "cache":
		f.Cache = value
	}
}
