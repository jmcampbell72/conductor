# Conductor — Implementation Reference

## Environment Variables

All configuration is supplied through environment variables. Every variable has a default so the server starts without any variables set (development mode).

### Provider Keys

| Variable | Default | Description |
|---|---|---|
| `OPENAI_API_KEY` | _(empty)_ | OpenAI API key. If empty, the OpenAI provider is not registered. |
| `ANTHROPIC_API_KEY` | _(empty)_ | Anthropic API key. If empty, the Anthropic provider is not registered. |

At least one provider key is required to serve real traffic. With neither set, the server starts and accepts requests but returns 400 for any model it cannot route.

### Gateway Authentication

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_API_KEYS` | _(empty)_ | Comma-separated `key:caller_id` pairs. If empty, all requests are allowed (dev mode). |

Format: `sk-gateway-abc:alice,sk-gateway-def:bob`

Each pair maps a bearer token to a caller ID used for logging, telemetry, and rate limiting. If a token has no colon separator, the key and caller ID are the same value.

### Admin API

| Variable | Default | Description |
|---|---|---|
| `ADMIN_API_KEY` | _(empty)_ | Bearer token for the `/admin/*` endpoints. If empty, all admin endpoints return 403. |

### Server

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_ADDR` | `:8080` | TCP address the HTTP server binds to. |
| `DEFAULT_MODEL` | `gpt-4o-mini` | Model used when the caller does not specify one. |

### Rate Limiting

| Variable | Default | Description |
|---|---|---|
| `RATE_LIMIT_RPS` | `10` | Sustained requests per second per caller. |
| `RATE_LIMIT_BURST` | `20` | Maximum burst above the sustained rate. |

### Cache

| Variable | Default | Description |
|---|---|---|
| `CACHE_PATH` | `data/cache.gob` | Path to the gob snapshot file for the in-memory cache. Directory must exist. |
| `ROUTES_CONFIG` | `config/routes.json` | Path to the route configuration file. Missing file falls back to built-in defaults. |

---

## Route Configuration (`config/routes.json`)

The route file is loaded at startup and can be updated at runtime via `PATCH /admin/routes/default`. Fields not present in the file take built-in defaults.

```json
{
  "default": {
    "token_budget": 4000,
    "complexity_threshold": 0.6,
    "similarity_threshold": 0.85,
    "max_tokens": 0,
    "response_format_type": "",
    "concise_response": false,
    "agent_traffic": false,
    "model_override": "",
    "models": {
      "simple": "gpt-4o-mini",
      "complex": "claude-opus-5"
    }
  },
  "named": {}
}
```

### Field Reference

| Field | Type | Default | Description |
|---|---|---|---|
| `token_budget` | int | 4000 | Maximum tokens in the context window before trimming. |
| `complexity_threshold` | float | 0.6 | Score at or above which the complex model is used. Range 0.0–1.0. |
| `similarity_threshold` | float | 0.85 | Minimum cosine similarity for a semantic cache hit. Range 0.0–1.0. |
| `max_tokens` | int | 0 | Hard cap on `max_tokens` sent to the provider. 0 means no cap. |
| `response_format_type` | string | `""` | Forces `response_format` when the caller hasn't set one. Values: `""`, `"text"`, `"json_object"`. |
| `concise_response` | bool | false | Injects a system message instructing the model to be brief. |
| `agent_traffic` | bool | false | Enables shorthand compression/decompression for this route. |
| `model_override` | string | `""` | Forces a specific model, bypassing complexity scoring. |
| `models.simple` | string | `"gpt-4o-mini"` | Model used for low-complexity requests. |
| `models.complex` | string | `"claude-opus-5"` | Model used for high-complexity requests. |

---

## Packages

### `internal/api`
Shared OpenAI-compatible types. No internal dependencies. This package is a leaf that every other package can import safely.

Key types: `Message`, `ChatCompletionRequest`, `ChatCompletionResponse`, `ResponseFormat`, `Usage`, `ErrorResponse`.

Helper: `WriteError(w, status, errType, message)` — writes a JSON error body and sets the HTTP status.

### `internal/config`
Loads `Config` from environment variables at startup. Provides `RouteStore` for runtime-mutable route configuration.

`config.Load()` reads env vars, applies defaults, and attempts to parse `ROUTES_CONFIG`. A missing or unparseable route file silently falls back to built-in defaults.

`config.NewRouteStore(initial)` wraps the loaded routes. The handler calls `routeStore.Default()` on every request; the admin handler calls `routeStore.UpdateDefault(r)` to apply live changes.

### `internal/middleware`
Three middleware functions composable with `middleware.Chain(...)`.

`middleware.CallerID(ctx)` extracts the authenticated caller ID from the request context. Used by the gateway handler for telemetry recording.

### `internal/provider`
`provider.Provider` interface: `Complete(ctx, req) (*ChatCompletionResponse, error)` and `Name() string`.

`provider.NewOpenAI(key)` — passes requests through unchanged to `https://api.openai.com/v1/chat/completions`.

`provider.NewAnthropic(key)` — translates to the Anthropic Messages API. System messages are extracted. Stop reasons are normalised: `end_turn` / `stop_sequence` → `"stop"`, `max_tokens` → `"length"`, `tool_use` → `"tool_calls"`. `max_tokens` defaults to 4096 when the caller sends 0.

### `internal/trim`
`trim.New(provider)` creates a Manager. Pass `nil` as the provider to disable summarisation (trimming still works but dropped messages are discarded without summarisation).

`Manager.Process(ctx, req, budget)` returns the trimmed request. Returns the original pointer unchanged if the request is already within budget.

### `internal/router`
`router.NewAnalyzer()` — creates a scorer with the built-in keyword list.

`router.NewSelector(route)` — creates a selector using the route's threshold, models, and optional override.

### `internal/output`
`output.New()` returns an `Enforcer`. `Enforcer.Apply(req, route)` returns a modified copy of the request. The original is never mutated.

### `internal/cache`
`cache.NewMemory()` — in-memory cache. Call `Load(path)` before starting goroutines, then `StartEviction(ctx, interval)` and `StartPersist(ctx, path, interval)`.

`cache.NewRedis(addr)` — Redis-backed cache. `addr` is `"host:port"`. Connection is established lazily on the first Get/Set call.

`cache.Key(req)` — computes the SHA-256 cache key. Call this after trimming and output enforcement.

### `internal/vec`
`vec.NewStore()` — creates a semantic store backed by HNSW.

`Store.Add(id, req, resp)` — indexes a request+response pair. `id` should be the exact cache key so both caches reference the same entry.

`Store.Search(req, threshold)` — returns the best match above threshold, or `(nil, 0, false)`.

`vec.NewHNSW()` — creates an HNSW index with M=16, efConstruction=200, ef=50.

### `internal/codec`
`codec.New()` — creates a Codec from the built-in dictionary.

`Compress(text)` / `Decompress(text)` — operate on raw strings.

`CompressMessages(msgs)` / `DecompressMessages(msgs)` — operate on message slices, skipping system messages.

`CompressResponse(resp)` — returns a copy of the response with compressed choice content.

`Ratio(original, compressed)` — returns `len(compressed)/len(original)`; values below 1.0 indicate savings.

### `internal/telemetry`
`telemetry.NewRegistry()` — creates a registry. Thread-safe.

`Registry.Record(callerID, model, cacheStatus, tokens)` — called once per request after the response is sent. `cacheStatus` is `"hit"`, `"miss"`, or a `"semantic-hit:N.NN"` prefixed string.

`Registry.Snapshot()` — returns a point-in-time copy of all counters, safe for JSON serialisation.

### `internal/admin`
`admin.NewHandler(tele, kv, routeStore, adminKey)` — creates the admin handler. Call `Register(mux)` to mount all endpoints.

### `internal/redis`
`redis.New(addr)` — creates a client. `Ping()` checks connectivity. `Get(key)` returns `redis.ErrNil` when the key is absent. `Set(key, value, ttl)` stores raw bytes with an expiry.

---

## Cache Persistence

The in-memory cache saves a gob snapshot to `CACHE_PATH` (default `data/cache.gob`) on an hourly interval and on clean shutdown. Entries include their original TTL; expired entries loaded from disk are discarded on first access.

The snapshot write is atomic: the data is written to a temp file in the same directory, then renamed. A crash during write leaves the previous snapshot intact.

Create the `data/` directory before starting the server if you are using the default path:

```sh
mkdir -p data
```

---

## Redis as Cache Backend

The Redis adapter is a drop-in replacement for the in-memory cache. To use it, modify `cmd/gateway/main.go`:

```go
// Replace:
kv := cache.NewMemory()
kv.Load(cfg.CachePath)
kv.StartEviction(ctx, time.Minute)
kv.StartPersist(ctx, cfg.CachePath, time.Hour)

// With:
kv := cache.NewRedis(os.Getenv("REDIS_ADDR")) // e.g. "localhost:6379"
```

Values are JSON-encoded `ChatCompletionResponse` objects. Keys are the same SHA-256 hex strings used by the in-memory cache. TTL is set to 24 hours per entry.

---

## Streaming

Streaming (`"stream": true`) is not yet supported. The gateway returns HTTP 400 with `streaming_not_supported` for any streaming request.

---

## Provider Routing Logic

Provider selection is based on the model name prefix, resolved after complexity scoring:

| Model prefix | Provider |
|---|---|
| `gpt-`, `o1`, `o3`, `o4` | OpenAI |
| `claude-` | Anthropic |
| _(anything else)_ | First registered provider (fallback) |

If no provider is registered for the selected model, the gateway returns HTTP 400 with `unknown_model`.
