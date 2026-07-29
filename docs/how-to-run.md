# Conductor — How To Run

## Prerequisites

- Go 1.21 or later (`go version`)
- An OpenAI or Anthropic API key for live traffic (not required to start the server or run tests)
- Redis (optional — only needed if you switch the cache backend to `RedisStore`)

---

## Build

```sh
go build ./...
```

Build the gateway binary:

```sh
go build -o conductor ./cmd/gateway
```

---

## Run

### Development mode (no API keys, no auth)

The server starts with all defaults. No environment variables are required. Requests are allowed from anyone. Responses will fail at the provider call if no API key is configured.

```sh
mkdir -p data
go run ./cmd/gateway
```

The server listens on `:8080` and logs JSON to stdout.

### With an OpenAI provider

```sh
export OPENAI_API_KEY=sk-...
mkdir -p data
go run ./cmd/gateway
```

### With both providers and gateway auth

```sh
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=sk-ant-...
export GATEWAY_API_KEYS="sk-gw-alice:alice,sk-gw-bob:bob"
export ADMIN_API_KEY=sk-admin-secret
mkdir -p data
go run ./cmd/gateway
```

### Full configuration example

```sh
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=sk-ant-...
export HOSTED_LLMS='[{"name":"my-private-llm","url":"https://api.example.com/v1","key":"sk-..."}]'
export GATEWAY_API_KEYS="sk-gw-alice:alice"
export ADMIN_API_KEY=sk-admin-secret
export GATEWAY_ADDR=:9000
export RATE_LIMIT_RPS=50
export RATE_LIMIT_BURST=100
export CACHE_PATH=data/cache.gob
export ROUTES_CONFIG=config/routes.json
export DEFAULT_MODEL=gpt-4o-mini
mkdir -p data
go run ./cmd/gateway
```

---

## Health Check

```sh
curl http://localhost:8080/health
```

Returns `200 OK` with an empty body.

---

## Making Requests

The gateway is OpenAI-compatible. Any client that supports a custom `base_url` works with no other changes.

### curl

```sh
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-gw-alice" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'
```

Dev mode (no auth configured):

```sh
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

### Python (openai SDK)

```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-gw-alice",
    base_url="http://localhost:8080/v1",
)

response = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Explain HNSW in one sentence."}],
)
print(response.choices[0].message.content)
```

### JavaScript (openai SDK)

```js
import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk-gw-alice",
  baseURL: "http://localhost:8080/v1",
});

const resp = await client.chat.completions.create({
  model: "gpt-4o-mini",
  messages: [{ role: "user", content: "Hello" }],
});
console.log(resp.choices[0].message.content);
```

---

## Response Headers

| Header | Values | Meaning |
|---|---|---|
| `X-Cache` | `HIT` | Response served from exact cache |
| `X-Cache` | `SEMANTIC-HIT;score=0.8800` | Response served from semantic cache |
| _(absent)_ | | Cache miss; response from provider |

---

## Admin API

All admin endpoints require `Authorization: Bearer <ADMIN_API_KEY>`.

### Telemetry

```sh
curl http://localhost:8080/admin/stats \
  -H "Authorization: Bearer sk-admin-secret"
```

Response:
```json
{
  "global": {
    "requests": 42,
    "tokens": 8400,
    "exact_hits": 15,
    "semantic_hits": 7,
    "misses": 20,
    "simple_routes": 30,
    "complex_routes": 12
  },
  "callers": {
    "alice": { "requests": 30, "tokens": 5000, "exact_hits": 10, ... },
    "bob":   { "requests": 12, "tokens": 3400, "exact_hits": 5,  ... }
  }
}
```

### Cache stats

```sh
curl http://localhost:8080/admin/cache/stats \
  -H "Authorization: Bearer sk-admin-secret"
```

Response:
```json
{
  "hits": 22,
  "misses": 20,
  "entries": 18,
  "hit_ratio": 0.524
}
```

### Inspect current routes

```sh
curl http://localhost:8080/admin/routes \
  -H "Authorization: Bearer sk-admin-secret"
```

### Update a route field at runtime

Raises the complexity threshold so more requests route to the capable model:

```sh
curl -X PATCH http://localhost:8080/admin/routes/default \
  -H "Authorization: Bearer sk-admin-secret" \
  -H "Content-Type: application/json" \
  -d '{"complexity_threshold": 0.5}'
```

Only the fields you send are changed. All other fields keep their current values. The change takes effect immediately for the next request — no restart required.

Enable concise responses:

```sh
curl -X PATCH http://localhost:8080/admin/routes/default \
  -H "Authorization: Bearer sk-admin-secret" \
  -H "Content-Type: application/json" \
  -d '{"concise_response": true}'
```

---

## Tests

Run all tests:

```sh
go test ./...
```

Run tests for a specific package:

```sh
go test ./internal/vec/...
go test ./internal/cache/...
go test ./internal/codec/...
```

Run with verbose output:

```sh
go test ./... -v
```

Run with the race detector:

```sh
go test ./... -race
```

---

## Benchmarks

Gateway cache-hit throughput:

```sh
go test ./internal/gateway/... -bench=BenchmarkHandler_CacheHit -benchmem -benchtime=5s
```

HNSW index — insert and search:

```sh
go test ./internal/vec/... -bench=BenchmarkHNSW -benchmem -benchtime=5s
```

All benchmarks:

```sh
go test ./... -bench=. -benchmem -benchtime=3s
```

---

## Route Configuration

Edit `config/routes.json` and restart the server, or use `PATCH /admin/routes/default` to update live without restarting.

Example — full model tier configuration with a hosted private LLM and budget threshold:

```json
{
  "default": {
    "token_budget": 4000,
    "complexity_threshold": 0.6,
    "similarity_threshold": 0.85,
    "models": {
      "simple": "gpt-4o-mini",
      "complex": "claude-opus-5",
      "writing": "claude-sonnet-5",
      "qa": "gpt-4o",
      "economy": "my-private-llm"
    },
    "model_costs": {
      "claude-opus-5": 0.015,
      "claude-sonnet-5": 0.003,
      "gpt-4o": 0.005,
      "gpt-4o-mini": 0.00015,
      "my-private-llm": 0.0001
    },
    "budget_threshold": 50.0
  }
}
```

When `budget_threshold` is set, requests accumulate spend (tokens × cost-per-1k) until the threshold is reached, at which point all subsequent requests route to the `economy` model. Spend resets on process restart.

Example — enable agent traffic compression and JSON-only output:

```json
{
  "default": {
    "token_budget": 4000,
    "complexity_threshold": 0.6,
    "similarity_threshold": 0.85,
    "agent_traffic": true,
    "response_format_type": "json_object",
    "concise_response": true,
    "models": {
      "simple": "gpt-4o-mini",
      "complex": "claude-opus-5"
    }
  }
}
```

---

## Shutdown

The server catches `SIGINT` (Ctrl-C) and `SIGTERM`. On signal:

1. The in-memory cache is saved to disk immediately.
2. In-flight requests are allowed to complete (up to 10 seconds).
3. The server exits cleanly.

```
^C
{"level":"INFO","msg":"shutdown complete"}
```
