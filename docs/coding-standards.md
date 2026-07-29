# Conductor — Coding Standards

## Language and Toolchain

- Go 1.21 or later. The `go.mod` module name is `conductor`.
- No external dependencies. Every import must resolve to the Go standard library or another `conductor/internal/...` package.
- `gofmt` formatting is assumed. No linter configuration is committed, but `go vet ./...` must pass.

---

## Package Layout

```
cmd/gateway/          # main package — wiring only, no business logic
internal/
  admin/              # management HTTP handlers
  api/                # shared OpenAI-compatible types (leaf, no internal imports)
  cache/              # exact response cache (Memory and Redis implementations)
  codec/              # shorthand compression codec
  config/             # env-driven config loader and RouteStore
  gateway/            # core request handler and pipeline
  middleware/         # HTTP middleware (Logging, Auth, RateLimit)
  output/             # per-route output enforcement
  provider/           # LLM provider abstraction (OpenAI, Anthropic)
  redis/              # minimal RESP2 TCP client
  reqid/              # request ID generation
  router/             # complexity scoring and model selection
  telemetry/          # per-caller attribution counters
  trim/               # context trimming and summarisation
  vec/                # TF embedder, HNSW index, semantic store
```

Packages are named after what they contain, not after the layer they belong to. No package has a `utils`, `common`, or `shared` name.

---

## Dependencies Between Packages

The dependency graph is strictly acyclic:

- `internal/api` imports nothing internal — it is a pure-types leaf.
- `internal/provider` imports `internal/api`.
- `internal/trim` imports `internal/api` and `internal/provider`.
- `internal/gateway` imports everything below it; nothing imports `internal/gateway` except `cmd/gateway`.
- `cmd/gateway` imports everything and does only wiring.

No package may import its parent or sibling at the same directory level. All edges point inward toward `internal/api`.

---

## Error Handling

- Errors propagate upward via Go's standard `error` return.
- The gateway handler converts provider errors to HTTP 502 via `api.WriteError`.
- Cache operations are best-effort: write failures are silently swallowed (`_ = err`); read failures fall through to a miss.
- Configuration load failures for optional fields (missing routes file, bad YAML) fall back to defaults and log a warning — they do not terminate the process.
- Fatal errors (server bind failure) call `os.Exit(1)` with a structured log entry.

---

## Concurrency

- Shared state is protected by `sync.RWMutex`: multiple concurrent reads are allowed; writes acquire the full lock.
- Counters that are only incremented (telemetry, cache hit/miss) use `sync/atomic.Int64` to avoid holding a mutex.
- Goroutines are started only in `main` (eviction sweep, persistence sweep, shutdown waiter) and in the middleware/handler path where the stdlib `net/http` server manages goroutine lifetime.
- No goroutine is started without a clear termination condition. Background goroutines accept a `context.Context` and exit when it is cancelled.

---

## HTTP Handlers

- Every handler writes exactly one response and returns. No implicit status-200 fall-through.
- Error responses use `api.WriteError` (gateway) or `http.Error` (admin), both of which set the `Content-Type` header and write the status before the body.
- Handlers do not write to `http.ResponseWriter` after calling `return`.
- The middleware chain is composed with `middleware.Chain(first, ..., last)` where `first` is the outermost wrapper. The order is: Logging → Auth → RateLimit → Handler.

---

## Logging

- All logging uses `log/slog` with a JSON handler (`slog.NewJSONHandler`).
- Log fields are key-value pairs passed as alternating `string, any` arguments: `slog.Info("event", "key", value)`.
- Request-scoped fields (caller ID, model, cache outcome) are written to a `logFields` struct stored in the request context and read by the logging middleware after the handler chain completes. Inner handlers call `middleware.SetLogField(ctx, key, value)` — they do not call `slog` directly for request-level fields.
- No `fmt.Println`, `log.Printf`, or bare `fmt.Fprintf(os.Stderr, ...)` in any package.

---

## Testing

- Tests live alongside the code they test (`package foo` for white-box, `package foo_test` for black-box).
- Each test function tests one behaviour. Test names follow `TestType_Scenario` or `TestFunction_Scenario`.
- Table-driven tests are used only when the same assertion runs over more than three inputs. Otherwise, separate named test functions are clearer.
- No mocks. Tests use real implementations with controlled inputs: in-memory stores, stub providers implemented as local types in the test file, `httptest.NewRecorder` for HTTP handlers.
- Integration tests that require external services (Redis) are skipped when the service is unreachable.
- Benchmarks use `b.ReportAllocs()` and reset the timer after setup with `b.ResetTimer()`.

---

## Comments

Comments explain **why**, not what. Well-named identifiers already say what.

Write a comment when:
- A constraint is non-obvious (e.g. "must run after model selection so the key reflects injected content")
- A behaviour would surprise a reader (e.g. "rand.Float64() can return 0.0")
- A workaround exists for a specific bug or limitation

Do not write comments that:
- Restate the function signature
- Reference the current task, issue number, or caller
- Say "This function does X" when the function is named `doX`

One short line maximum per comment block. No multi-paragraph docstrings.

---

## Naming

| Thing | Convention | Example |
|---|---|---|
| Packages | Lowercase, single word | `cache`, `codec`, `trim` |
| Exported types | PascalCase noun | `RouteStore`, `Embedder` |
| Exported functions/methods | PascalCase verb or verb-phrase | `NewMemory`, `CompressResponse` |
| Unexported helpers | camelCase | `requestText`, `insertSorted` |
| Interface names | Noun or noun-er | `Provider`, `Indexer`, `Store` |
| Test helpers | camelCase with `make` prefix | `makeReq`, `makeTestResp` |
| Constants | camelCase for unexported, PascalCase for exported | `cacheTTL`, `hnswDefaultM` |

---

## Immutability

Functions that accept a pointer and return a modified version must not mutate the original:

- `output.Enforcer.Apply` returns a shallow copy of the request; `Messages` is replaced with a new slice when concise injection is needed.
- `codec.Codec.CompressResponse` returns a shallow copy of the response with a new `Choices` slice.
- `trim.Manager.Process` may return the original pointer unchanged or a new struct; it never mutates the input.

---

## Configuration

- All configuration is read from environment variables at startup via `config.Load()`.
- No configuration is re-read after startup, except route config which is managed by `config.RouteStore` and can be mutated at runtime via the admin API.
- Sensitive values (API keys) are never logged. The startup log prints which providers are configured, not their keys.
