# Conductor — Design

## Problem Statement

LLM API costs scale with token volume. Applications that naively route every request to the most capable model, never cache semantically-equivalent responses, and carry full conversation history into every turn leave significant cost on the table.

Conductor addresses this without requiring callers to change their code — it speaks the OpenAI wire protocol, so any SDK or tool that accepts a `base_url` works unchanged.

---

## Core Principles

**Zero external dependencies through Phase 6.**
Every package uses only the Go standard library. This eliminates version conflicts, licence complications, and supply-chain risk. The Redis client is hand-written RESP2 over `net.Conn`; the HNSW index is implemented from scratch; serialisation uses `encoding/json` and `encoding/gob`.

**Interface-driven design at system boundaries.**
Three interfaces exist — `provider.Provider`, `cache.Store`, and `vec.Indexer` — each at a point where the implementation is expected to change. Everything internal uses concrete types.

**Operate-able from day one.**
Configuration has sane defaults for every field. The server starts and handles traffic with no environment variables set (dev mode). Logging is structured JSON via `slog` from the first commit.

**Pipeline stages are decoupled and ordered.**
Each stage (trim → route → output → cache → provider) has a single responsibility and a defined position. The ordering is not arbitrary: trimming must precede routing (the scorer needs the final message set), routing must precede output enforcement (max_tokens cap is model-tier-aware), and output enforcement must precede the cache key (injected content changes what the provider receives).

---

## Phase Progression

The application was built in seven phases, each adding a complete vertical slice of functionality.

| Phase | Feature | Key addition |
|---|---|---|
| 1 | Gateway shell | HTTP server, auth, rate limit, logging, OpenAI + Anthropic providers |
| 2 | Context management + routing | Token trimming with summarisation, complexity scoring, model tier selection |
| 3 | Exact KV cache | SHA-256 keyed in-memory cache with TTL, gob persistence, `cache.Store` seam |
| 4 | Semantic cache | TF-IDF embedder, flat k-NN index, cosine similarity matching |
| 5 | Output control | Per-route `max_tokens` cap, `response_format`, concise-response injection |
| 6 | Shorthand compression | Symmetric phrase codec for agent-to-agent traffic |
| 7 | Observability + scale | Telemetry registry, admin API, live route mutation, Redis adapter, HNSW index |

---

## Key Design Decisions

### Mutable log record pattern

The logging middleware is outermost so it captures the final HTTP status. But auth and the gateway handler know things the logger needs — caller ID, selected model, cache outcome — that aren't known when the log record opens.

The solution is a `logFields` struct allocated at the start of each request and stored in `context.Context`. Inner handlers call `middleware.SetLogField(ctx, key, value)` to write into it. When the logging middleware resumes after the inner chain returns, it reads the struct and appends the fields to the log line.

This avoids coupling any inner handler to the logger and avoids a response-writer wrapper that would need to buffer the body to inspect it.

### Cache key after trim and enforce

An early design computed the cache key from the raw request. This breaks because two requests with different history lengths but the same most-recent message would get different keys even though they'd produce the same provider response after trimming.

The correct invariant: **the cache key is a hash of exactly what will be sent to the provider**. That means trim → route → output enforce → hash, in that order.

### Pure TF vectors (no IDF)

The semantic cache initially used TF-IDF. This introduces corpus drift: a stored document's vector is computed with IDF values at insertion time, but query vectors use current IDF values (which change as more documents are added). The dot-product comparison then mixes incompatible vector spaces.

The fix: drop IDF. Stopword removal already handles common-term downweighting, and pure TF vectors are consistent regardless of corpus size. The trade-off — slightly lower recall for domain-specific rare terms — is acceptable for a cache where precision matters more than recall.

### Storing uncompressed in both caches

Agent-traffic routes compress their responses before returning them to the caller. An early approach compressed before storing, which broke non-agent-traffic routes that share the same cache keys. The correct approach: always store uncompressed, compress on the way out only for agent-traffic routes.

### Admin API uses JSON merge-patch semantics

`PATCH /admin/routes/default` decodes the request body on top of the current config (marshal current → unmarshal patch over it). This means operators can send `{"complexity_threshold": 0.7}` to change just one field without resending the entire config. Fields omitted from the patch keep their current values.

### HNSW over flat scan

The flat k-NN scan in Phase 4 is O(n) per query. HNSW is O(log n) expected. For caches under ~500 entries the difference is negligible, but HNSW was implemented in Phase 7 to establish the infrastructure for the eventual HNSW-backed production index and to satisfy the `vec.Indexer` interface so the upgrade is transparent to the `Store`.

M=16, efConstruction=200, ef=50 are standard defaults from the original HNSW paper. They favour recall over speed; adjust ef downward for latency-sensitive deployments.

### Codec uses two-character class scheme

The shorthand dictionary distinguishes sentence-starting capitalised phrases (`<<FUR>>` for "Furthermore, ") from mid-sentence lowercase variants (`<<fur>>` for "furthermore, "). This is essential for round-trip fidelity: a text with "furthermore, " mid-sentence compresses to `<<fur>>` and decompresses back to "furthermore, ". If both cases shared one token, the restored case would be wrong for at least one form.

---

## Trade-offs and Future Work

| Area | Current approach | Known limitation | Future direction |
|---|---|---|---|
| Summarisation model | Hard-coded `gpt-4o-mini` | Caller can't choose | Make configurable per route |
| Semantic corpus | Pure TF, in-memory | Lost on restart; no IDF | Persist HNSW graph; add BM25 or neural embedder |
| Codec | Rule-based dictionary | ~30% compression ceiling | Fine-tuned seq2seq compression model |
| Route selection | Default route only | Named routes defined but not matched per-request | Match by path prefix or caller header |
| Redis client | Single connection | No pooling; one slow command blocks all | Connection pool with retry |
| HNSW | Global write lock | Inserts block all searches | Lock-free insert with version generation |
