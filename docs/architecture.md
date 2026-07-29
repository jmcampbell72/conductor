# Conductor — Architecture

## Overview

Conductor is an AI inference gateway that sits between callers (applications, agents) and LLM providers (OpenAI, Anthropic). It exposes an OpenAI-compatible API so existing SDKs need only a `base_url` change.

Its primary goals are cost reduction and throughput improvement through:

- **Tiered routing** — cheap model for simple requests, capable model for complex ones
- **Exact caching** — identical requests never hit the provider twice
- **Semantic caching** — similar requests are served from the cache using vector similarity
- **Context compression** — agent-to-agent traffic is compressed to reduce token spend across multi-turn conversations

---

## Request Pipeline

Every request passes through the following stages in order. Stages earlier in the chain can short-circuit and return without reaching the provider.

```
Caller
  │
  ▼
┌────────────────────────────────────────────┐
│  HTTP middleware chain                      │
│  1. Logging     (outermost, captures all)  │
│  2. Auth        (validates Bearer token)   │
│  3. Rate limit  (per-caller token bucket)  │
└──────────────────────┬─────────────────────┘
                       │
                       ▼
┌────────────────────────────────────────────┐
│  Gateway handler                           │
│                                            │
│  4. Decompress  (agent traffic only)       │
│  5. Trim        (sliding window + summary) │
│  6. Score       (complexity heuristics)    │
│  7. Route       (model tier selection)     │
│  8. Output ctrl (max_tokens, format, tone) │
│                                            │
│  9. Cache key  ──────────────────────────► │ exact hit
│                                            │   │
│ 10. Semantic search ────────────────────► │ semantic hit
│                                            │   │
│ 11. Provider call                          │   │
│ 12. Store (exact + semantic)               │   │
│ 13. Compress (agent traffic only)          │   │
└──────────────────────┬─────────────────────┘   │
                       │ ◄──────────────────────┘
                       ▼
                    Caller
```

---

## Components

### `cmd/gateway`
The entry point. Loads config, wires all components, registers HTTP routes, and manages graceful shutdown via `signal.NotifyContext`.

### `internal/middleware`
Standard HTTP middleware with a `Chain(first, ..., last)` combinator where the first argument is the outermost wrapper.

| Middleware | Responsibility |
|---|---|
| `Logging` | Captures status, duration, caller ID, model, cache outcome after the inner chain returns |
| `Auth` | Validates `Authorization: Bearer <token>` against configured key map; dev-mode pass-through when map is empty |
| `RateLimit` | Per-caller-ID token bucket; falls back to `RemoteAddr` for unauthenticated traffic |

### `internal/gateway`
Core request handler. Orchestrates all pipeline stages. Holds references to all subsystems.

### `internal/provider`
Provider abstraction. The `Provider` interface exposes a single `Complete` method. The gateway selects a provider by model name prefix (`gpt-` / `o1` / `o3` / `o4` → OpenAI; `claude-` → Anthropic).

The Anthropic adapter translates to the Messages API and back: system messages are extracted to the `system` field; stop reasons are normalised; `max_tokens` defaults to 4096 when the caller sends 0.

### `internal/trim`
Enforces a per-route token budget by sliding a window over non-system messages (newest kept, oldest dropped). Dropped messages are optionally summarised by calling `gpt-4o-mini` with a 200-token budget and injecting the summary as a system message.

### `internal/router`
Two-step model selection:

1. **Analyzer** produces an `Analysis{Score, TaskType}` from four signals:
   - Content length (saturates at 3 000 chars → 0.25)
   - Turn count (saturates at 10 turns → 0.15)
   - Keyword presence (~30 domain/complexity terms → 0.35)
   - Structural cues (code fences, equations, JSON → 0.25)

   Task type is classified from keyword sets — `planning` (plan, outline, roadmap, …), `writing` (write, draft, compose, …), `qa` (test, debug, verify, …) — with `general` as the default. Priority: qa > writing > planning > general.

2. **Selector** applies a priority-ordered routing matrix:
   1. `model_override` — always wins when set
   2. Budget exceeded + `economy` model configured → economy model
   3. Task type `planning` or `writing` + `writing` model configured → writing model
   4. Task type `qa` + `qa` model configured → qa model
   5. Complexity score ≥ `complexity_threshold` → complex model
   6. Otherwise → simple model

### `internal/output`
`Enforcer.Apply` applies per-route output controls to the request before the cache key is computed:

- **`max_tokens` cap** — prevents runaway completions
- **`response_format_type`** — enforces `json_object` or `text` when the caller hasn't specified
- **`concise_response`** — injects a system message after the caller's system prompt instructing brevity

### `internal/cache`
Two-tier exact response cache.

| Implementation | Backing store | Notes |
|---|---|---|
| `Memory` | In-process `map[string]entry` | TTL eviction goroutine; gob snapshot persistence; atomic rename for crash safety |
| `RedisStore` | Redis via custom RESP2 client | JSON-encoded values; drop-in via `cache.Store` interface |

The cache key is a SHA-256 hash of the normalised request (model, messages, max_tokens, temperature, response_format) computed **after** trimming and output enforcement, so the key reflects exactly what the provider will receive.

### `internal/vec`
Semantic cache backed by an HNSW vector index.

**Embedder** — normalised TF (term-frequency) vectors. Pure TF (no IDF) keeps stored and query vectors in the same space as the corpus grows. Stopword removal and a 3-character minimum substitute for IDF's common-term downweighting.

**HNSW index** — Hierarchical Navigable Small World graph. Nodes are assigned levels via geometric distribution (`P(level ≥ l) = (1/M)^l`, M=16). Insert connects M nearest neighbours per layer with pruning; search greedily descends to layer 1 then beam-searches layer 0 with `ef=50`. O(log n) expected search time. Replaces the flat O(n) scan from early phases.

**Store** — combines Embedder + HNSW + response map. All stored responses are in natural language; compression is applied at the handler layer, not here.

### `internal/codec`
Symmetric shorthand codec for agent-to-agent traffic. A 60-entry dictionary maps verbose LLM preamble and transition phrases to compact `<<TOKEN>>` markers. Uppercase tokens for sentence-starting (capitalised) forms; lowercase `<<token>>` for mid-sentence variants. Round-trip fidelity is guaranteed: `Decompress(Compress(x)) == x` for all covered phrases.

Inbound request history is decompressed before trimming (so the trimmer sees natural language). Outbound responses are compressed before returning to the agent.

### `internal/telemetry`
Per-caller atomic counters: requests, tokens, exact/semantic/miss cache outcomes, route tier splits (simple, complex, writing, qa, economy). Per-route spend tracking in microdollars (accumulated from token counts × configured cost rates) used for budget threshold enforcement. All metrics exposed via `GET /admin/stats`.

### `internal/admin`
Management API protected by a separate `ADMIN_API_KEY`. Provides read/write access to live route config, cache stats, and telemetry. Route mutations are audit-logged via `slog` and take effect immediately without restart.

### `internal/redis`
Minimal RESP2 protocol client using only `net.Conn`. Implements GET, SET EX, and PING over a single persistent TCP connection with reconnect-on-failure.

---

## Package Dependency Graph

```
cmd/gateway
  └─ internal/admin
  └─ internal/cache       ← internal/redis
  └─ internal/codec
  └─ internal/config
  └─ internal/gateway
       └─ internal/api
       └─ internal/cache
       └─ internal/codec
       └─ internal/config
       └─ internal/middleware
       └─ internal/output
       └─ internal/provider
       └─ internal/router
       └─ internal/telemetry
       └─ internal/trim    ← internal/provider
       └─ internal/vec
  └─ internal/middleware
  └─ internal/output
  └─ internal/provider
  └─ internal/router
  └─ internal/telemetry
  └─ internal/trim
  └─ internal/vec
```

No package imports its parent or sibling; all edges point inward. `internal/api` is a pure-types leaf with no dependencies on other internal packages.

---

## Key Interface Seams

| Interface | File | Implementations | Purpose |
|---|---|---|---|
| `provider.Provider` | `internal/provider/provider.go` | OpenAI, Anthropic | Swap or add providers |
| `cache.Store` | `internal/cache/store.go` | Memory, RedisStore | Replace in-memory cache with Redis |
| `vec.Indexer` | `internal/vec/store.go` | Index (flat), HNSW | Upgrade search algorithm |

---

## Data Flow: Cache Key Timing

The cache key is intentionally computed **after** trimming, routing, and output enforcement:

```
raw request
    │
    ▼ decompress (agent traffic)
    ▼ trim to token budget
    ▼ select model tier
    ▼ apply output controls (max_tokens, format, concise injection)
    │
    └─► SHA-256 → cache key
```

This ensures that two requests which differ only in verbosity but produce the same trimmed, enforced request share a cache entry. A route with `concise_response: true` gets its own cache namespace from a route without it because the injected system message is part of the hashed payload.
