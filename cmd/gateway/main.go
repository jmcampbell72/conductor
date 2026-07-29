package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"conductor/internal/admin"
	"conductor/internal/cache"
	"conductor/internal/codec"
	"conductor/internal/config"
	"conductor/internal/gateway"
	"conductor/internal/middleware"
	"conductor/internal/output"
	"conductor/internal/provider"
	"conductor/internal/router"
	"conductor/internal/telemetry"
	"conductor/internal/trim"
	"conductor/internal/vec"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.Load()

	providers := map[string]provider.Provider{}
	if cfg.OpenAIKey != "" {
		providers["openai"] = provider.NewOpenAI(cfg.OpenAIKey)
		slog.Info("provider registered", "name", "openai")
	}
	if cfg.AnthropicKey != "" {
		providers["anthropic"] = provider.NewAnthropic(cfg.AnthropicKey)
		slog.Info("provider registered", "name", "anthropic")
	}
	if len(providers) == 0 {
		slog.Warn("no providers configured — set OPENAI_API_KEY or ANTHROPIC_API_KEY")
	}

	var summariseWith provider.Provider
	for _, p := range providers {
		summariseWith = p
		break
	}

	routeStore := config.NewRouteStore(cfg.Routes)
	trimmer := trim.New(summariseWith)
	analyzer := router.NewAnalyzer()
	selector := router.NewSelector(cfg.Routes.Default)

	kv := cache.NewMemory()
	if err := kv.Load(cfg.CachePath); err != nil {
		slog.Warn("cache load failed", "path", cfg.CachePath, "err", err)
	} else {
		s := kv.Stats()
		slog.Info("cache loaded", "path", cfg.CachePath, "entries", s.Entries)
	}

	enforcer := output.New()
	msgCodec := codec.New()
	semantic := vec.NewStore()
	tele := telemetry.NewRegistry()

	h := gateway.NewHandler(
		providers, cfg, routeStore,
		trimmer, analyzer, selector,
		enforcer, msgCodec,
		kv, semantic, tele,
	)

	chain := middleware.Chain(
		middleware.Logging(),
		middleware.Auth(cfg.GatewayKeys),
		middleware.RateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst),
	)

	adminKey := os.Getenv("ADMIN_API_KEY")
	adminHandler := admin.NewHandler(tele, kv, routeStore, adminKey)

	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", chain(h))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	adminHandler.Register(mux)

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 130 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	kv.StartEviction(ctx, time.Minute)
	kv.StartPersist(ctx, cfg.CachePath, time.Hour)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	route := routeStore.Default()
	slog.Info("conductor gateway started",
		"addr", cfg.Addr,
		"token_budget", route.TokenBudget,
		"complexity_threshold", route.ComplexityThreshold,
		"similarity_threshold", route.SimilarityThreshold,
		"model_simple", route.Models.Simple,
		"model_complex", route.Models.Complex,
		"cache_path", cfg.CachePath,
		"admin_api", adminKey != "",
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
	slog.Info("shutdown complete")
}
