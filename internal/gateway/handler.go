package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"conductor/internal/api"
	"conductor/internal/cache"
	"conductor/internal/codec"
	"conductor/internal/config"
	"conductor/internal/middleware"
	"conductor/internal/output"
	"conductor/internal/provider"
	"conductor/internal/router"
	"conductor/internal/telemetry"
	"conductor/internal/trim"
	"conductor/internal/vec"
)

const cacheTTL = 24 * time.Hour

type Handler struct {
	providers  map[string]provider.Provider
	cfg        *config.Config
	routeStore *config.RouteStore
	trimmer    *trim.Manager
	analyzer   *router.Analyzer
	selector   *router.Selector
	enforcer   *output.Enforcer
	codec      *codec.Codec
	kv         cache.Store
	semantic   *vec.Store // nil disables semantic cache
	tele       *telemetry.Registry
}

func NewHandler(
	providers map[string]provider.Provider,
	cfg *config.Config,
	routeStore *config.RouteStore,
	trimmer *trim.Manager,
	analyzer *router.Analyzer,
	selector *router.Selector,
	enforcer *output.Enforcer,
	codec *codec.Codec,
	kv cache.Store,
	semantic *vec.Store,
	tele *telemetry.Registry,
) *Handler {
	return &Handler{
		providers:  providers,
		cfg:        cfg,
		routeStore: routeStore,
		trimmer:    trimmer,
		analyzer:   analyzer,
		selector:   selector,
		enforcer:   enforcer,
		codec:      codec,
		kv:         kv,
		semantic:   semantic,
		tele:       tele,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Telemetry is recorded via defer so it fires on every return path.
	var teleModel, teleCacheStatus, teleCallerID, teleTier string
	var teleTokens int
	var teleRoute config.RouteConfig
	defer func() {
		if h.tele != nil {
			h.tele.Record(teleCallerID, teleModel, teleCacheStatus, teleTokens)
			if teleTier != "" {
				h.tele.RecordTaskRoute(teleCallerID, teleTier)
			}
			if teleTokens > 0 && teleRoute.ModelCosts != nil {
				if cost, ok := teleRoute.ModelCosts[teleModel]; ok {
					dollars := float64(teleTokens) / 1000 * cost
					h.tele.RecordSpend("default", dollars)
				}
			}
		}
	}()

	teleCallerID = middleware.CallerID(r.Context())

	var req api.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	if req.Stream {
		api.WriteError(w, http.StatusBadRequest, "streaming_not_supported", "Streaming is not yet supported")
		return
	}

	if err := validate(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Model == "" {
		req.Model = h.cfg.DefaultModel
	}

	route := h.routeStore.Default()
	teleRoute = route

	// Decompress agent-traffic history before trimming so the trimmer and scorer
	// see natural language rather than shorthand tokens.
	if route.AgentTraffic {
		req.Messages = h.codec.DecompressMessages(req.Messages)
	}

	// Trim conversation history and RAG context to the route token budget.
	processed, err := h.trimmer.Process(r.Context(), &req, route.TokenBudget)
	if err != nil || processed == nil {
		processed = &req
	}

	// Analyse complexity and task type, then select the model tier.
	analysis := h.analyzer.Analyze(processed)
	budgetExceeded := route.BudgetThreshold > 0 &&
		h.tele != nil &&
		h.tele.RouteSpend("default") >= route.BudgetThreshold

	processed.Model = h.selector.Select(analysis, req.Model, budgetExceeded)
	teleTier = taskTier(analysis, budgetExceeded, h.selector)

	// Apply per-route output controls (max_tokens cap, response_format, concise instruction).
	// Must run after model selection and before the cache key so the key reflects any injections.
	processed = h.enforcer.Apply(processed, route)

	teleModel = processed.Model
	middleware.SetLogField(r.Context(), "model", processed.Model)
	middleware.SetLogField(r.Context(), "task_type", string(analysis.TaskType))

	// Exact KV cache check.
	key := cache.Key(processed)
	if cached, ok := h.kv.Get(key); ok {
		teleCacheStatus = "hit"
		teleTokens = int(cached.Usage.TotalTokens)
		middleware.SetLogField(r.Context(), "cache", "hit")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		if route.AgentTraffic {
			cached = h.codec.CompressResponse(cached)
		}
		json.NewEncoder(w).Encode(cached)
		return
	}

	// Semantic cache check — only when the corpus is warm enough to be useful.
	if h.semantic != nil {
		threshold := route.SimilarityThreshold
		if threshold <= 0 || threshold > 1 {
			threshold = 0.85
		}
		if semResp, score, ok := h.semantic.Search(processed, threshold); ok {
			status := fmt.Sprintf("semantic-hit:%.2f", score)
			teleCacheStatus = status
			teleTokens = int(semResp.Usage.TotalTokens)
			middleware.SetLogField(r.Context(), "cache", status)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", fmt.Sprintf("SEMANTIC-HIT;score=%.4f", score))
			if route.AgentTraffic {
				semResp = h.codec.CompressResponse(semResp)
			}
			json.NewEncoder(w).Encode(semResp)
			return
		}
	}

	teleCacheStatus = "miss"
	middleware.SetLogField(r.Context(), "cache", "miss")

	p := h.selectProvider(processed.Model)
	if p == nil {
		api.WriteError(w, http.StatusBadRequest, "unknown_model", "No provider configured for model: "+processed.Model)
		return
	}

	resp, err := p.Complete(r.Context(), processed)
	if err != nil {
		api.WriteError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}

	teleTokens = int(resp.Usage.TotalTokens)

	// Store uncompressed so cache hits work for both agent and non-agent routes.
	h.kv.Set(key, resp, cacheTTL)
	if h.semantic != nil {
		h.semantic.Add(key, processed, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	if route.AgentTraffic {
		resp = h.codec.CompressResponse(resp)
	}
	json.NewEncoder(w).Encode(resp)
}

// selectProvider maps a model name to a registered provider.
// Exact name match is checked first so that hosted LLMs and any explicitly
// named provider are resolved before falling back to prefix-based routing.
func (h *Handler) selectProvider(model string) provider.Provider {
	if p, ok := h.providers[model]; ok {
		return p
	}
	switch {
	case strings.HasPrefix(model, "gpt-"),
		strings.HasPrefix(model, "o1"),
		strings.HasPrefix(model, "o3"),
		strings.HasPrefix(model, "o4"):
		return h.providers["openai"]
	case strings.HasPrefix(model, "claude-"):
		return h.providers["anthropic"]
	default:
		for _, p := range h.providers {
			return p
		}
		return nil
	}
}

// taskTier returns a telemetry label for the routing decision that was made.
func taskTier(analysis router.Analysis, budgetExceeded bool, s *router.Selector) string {
	if budgetExceeded {
		return "economy"
	}
	switch analysis.TaskType {
	case router.TaskPlanning, router.TaskWriting:
		return "writing"
	case router.TaskQA:
		return "qa"
	}
	return ""
}

func validate(req *api.ChatCompletionRequest) error {
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages must not be empty")
	}
	for i, m := range req.Messages {
		if m.Role == "" {
			return fmt.Errorf("message[%d]: role is required", i)
		}
		if m.Content == "" {
			return fmt.Errorf("message[%d]: content is required", i)
		}
	}
	return nil
}
