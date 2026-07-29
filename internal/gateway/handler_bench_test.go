// Benchmarks for gateway handler throughput: the exact cache-hit path and the
// validation+routing path up to (but not including) provider dispatch.
//
// Author: Justin Campbell
package gateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"conductor/internal/api"
	"conductor/internal/cache"
	"conductor/internal/codec"
	"conductor/internal/config"
	"conductor/internal/gateway"
	"conductor/internal/output"
	"conductor/internal/router"
	"conductor/internal/telemetry"
	"conductor/internal/trim"
	"conductor/internal/vec"
)

// stubProvider satisfies provider.Provider and returns a fixed response.
type stubProvider struct{}

func (s *stubProvider) Complete(_ interface{ Done() <-chan struct{} }, _ *api.ChatCompletionRequest) (*api.ChatCompletionResponse, error) {
	return &api.ChatCompletionResponse{
		ID:    "stub-id",
		Model: "gpt-4o-mini",
		Choices: []api.ChatCompletionChoice{
			{Message: api.Message{Role: "assistant", Content: "stub response"}, FinishReason: "stop"},
		},
		Usage: api.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}
func (s *stubProvider) Name() string { return "stub" }

func benchHandler(b *testing.B) (*gateway.Handler, *cache.Memory) {
	b.Helper()
	cfg := &config.Config{
		DefaultModel:   "gpt-4o-mini",
		RateLimitRPS:   1e6,
		RateLimitBurst: 1e6,
	}
	routeStore := config.NewRouteStore(config.RoutesFile{
		Default: config.RouteConfig{
			TokenBudget:         4000,
			ComplexityThreshold: 0.6,
			SimilarityThreshold: 0.85,
		},
	})
	kv := cache.NewMemory()
	h := gateway.NewHandler(
		nil, // providers unused — stub is not wired; cache will hit before provider
		cfg,
		routeStore,
		trim.New(nil),
		router.NewAnalyzer(),
		router.NewSelector(routeStore.Default()),
		output.New(),
		codec.New(),
		kv,
		vec.NewStore(),
		telemetry.NewRegistry(),
	)
	return h, kv
}

func completionBody(t interface{ Helper() }) []byte {
	t.Helper()
	b, _ := json.Marshal(api.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []api.Message{{Role: "user", Content: "What is 2+2?"}},
	})
	return b
}

// BenchmarkHandler_CacheHit measures throughput when every request is an exact hit.
func BenchmarkHandler_CacheHit(b *testing.B) {
	h, kv := benchHandler(b)

	// Pre-populate the cache with the response for our fixed request.
	resp := &api.ChatCompletionResponse{
		ID:    "cached-id",
		Model: "gpt-4o-mini",
		Choices: []api.ChatCompletionChoice{
			{Message: api.Message{Role: "assistant", Content: "4"}, FinishReason: "stop"},
		},
		Usage: api.Usage{TotalTokens: 15},
	}
	// Compute the cache key for the same request the benchmark will send.
	req := &api.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []api.Message{{Role: "user", Content: "What is 2+2?"}},
	}
	kv.Set(cache.Key(req), resp, 24*time.Hour)

	body := completionBody(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

// BenchmarkHandler_ValidateMiss measures the path through validation and routing
// up to (but not including) the provider call. No provider is wired, so the
// benchmark ends at the "no provider configured" 400 response.
func BenchmarkHandler_ValidateMiss(b *testing.B) {
	h, _ := benchHandler(b)
	body := completionBody(b)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		// 400 is expected here — no provider configured.
	}
}
