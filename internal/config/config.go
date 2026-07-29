// Loads Config from environment variables at startup and defines all
// configuration types: route, model tier, hosted LLM, and gateway settings.
//
// Author: Justin Campbell
package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// HostedLLM describes a private OpenAI-compatible inference endpoint.
type HostedLLM struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Key  string `json:"key"`
}

// ModelsConfig holds the model names for each routing tier.
type ModelsConfig struct {
	Simple  string `json:"simple"`
	Complex string `json:"complex"`
	Writing string `json:"writing,omitempty"`
	QA      string `json:"qa,omitempty"`
	Economy string `json:"economy,omitempty"`
}

// RouteConfig defines trimming, routing, and output behaviour for a route.
type RouteConfig struct {
	TokenBudget         int                `json:"token_budget"`
	ComplexityThreshold float64            `json:"complexity_threshold"`
	SimilarityThreshold float64            `json:"similarity_threshold"`
	MaxTokens           int                `json:"max_tokens,omitempty"`
	ResponseFormatType  string             `json:"response_format_type,omitempty"`
	ConciseResponse     bool               `json:"concise_response,omitempty"`
	Models              ModelsConfig       `json:"models"`
	ModelOverride       string             `json:"model_override,omitempty"`
	AgentTraffic        bool               `json:"agent_traffic,omitempty"`
	ModelCosts          map[string]float64 `json:"model_costs,omitempty"`  // cost per 1k tokens per model
	BudgetThreshold     float64            `json:"budget_threshold,omitempty"` // dollars; 0 = disabled
}

// RoutesFile is the top-level structure of config/routes.json.
type RoutesFile struct {
	Default RouteConfig            `json:"default"`
	Named   map[string]RouteConfig `json:"named,omitempty"`
}

type Config struct {
	Addr           string
	GatewayKeys    map[string]string
	OpenAIKey      string
	AnthropicKey   string
	HostedLLMs     []HostedLLM
	RateLimitRPS   float64
	RateLimitBurst int
	DefaultModel   string
	CachePath      string
	Routes         RoutesFile
}

func Load() *Config {
	cfg := &Config{
		Addr:           env("GATEWAY_ADDR", ":8080"),
		GatewayKeys:    parseKeyMap(os.Getenv("GATEWAY_API_KEYS")),
		OpenAIKey:      os.Getenv("OPENAI_API_KEY"),
		AnthropicKey:   os.Getenv("ANTHROPIC_API_KEY"),
		HostedLLMs:     parseHostedLLMs(os.Getenv("HOSTED_LLMS")),
		RateLimitRPS:   envFloat("RATE_LIMIT_RPS", 10),
		RateLimitBurst: envInt("RATE_LIMIT_BURST", 20),
		DefaultModel:   env("DEFAULT_MODEL", "gpt-4o-mini"),
		CachePath:      env("CACHE_PATH", "data/cache.gob"),
		Routes:         defaultRoutes(),
	}

	routesPath := env("ROUTES_CONFIG", "config/routes.json")
	if data, err := os.ReadFile(routesPath); err == nil {
		json.Unmarshal(data, &cfg.Routes)
	}

	return cfg
}

func defaultRoutes() RoutesFile {
	return RoutesFile{
		Default: RouteConfig{
			TokenBudget:         4000,
			ComplexityThreshold: 0.6,
			SimilarityThreshold: 0.85,
		},
	}
}

// parseHostedLLMs parses a JSON array from the HOSTED_LLMS env var.
// Example: [{"name":"my-llm","url":"https://api.example.com/v1","key":"sk-..."}]
func parseHostedLLMs(s string) []HostedLLM {
	if s == "" {
		return nil
	}
	var llms []HostedLLM
	if err := json.Unmarshal([]byte(s), &llms); err != nil {
		return nil
	}
	return llms
}

// parseKeyMap parses "key1:callerID1,key2:callerID2" into a map.
// If a token has no colon, the key and caller ID are the same value.
func parseKeyMap(s string) map[string]string {
	m := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, ":")
		if ok {
			m[k] = v
		} else {
			m[pair] = pair
		}
	}
	return m
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
