// Command server wires every registered MCP namespace onto one HTTP mux over
// the Streamable HTTP transport and serves them.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"

	// Blank imports run each namespace's init(), which self-registers it with
	// mcpx. Adding a namespace = add its package here.
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/event"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/gsc"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/memory"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/producthunt"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/skills"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Behind a tunnel or reverse proxy (ngrok, Cloudflare), the Host header is
	// the public domain while the connection lands on loopback — which the MCP
	// transport's DNS-rebinding guard rejects with 403. Opt out explicitly.
	allowExternalHost := envOr("MCP_ALLOW_EXTERNAL_HOST", "") == "true"
	if allowExternalHost {
		log.Warn("DNS-rebinding protection disabled; only do this behind a trusted proxy")
	}

	// Connect to Postgres up front so a bad DATABASE_URL fails at startup rather
	// than on the first tool call.
	pool, err := openDB(context.Background(), log)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	if pool != nil {
		defer pool.Close()
	}

	// Signal-scoped context, created before the handler so it can back
	// namespaces' background work (memory's embedding workers): cancelling it on
	// SIGINT/SIGTERM stops those goroutines as part of shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// CLERK_SECRET_KEY, when set, mounts the Clerk-authenticated key-management
	// API (/api/keys) the web dashboard calls. Without it the server serves only
	// the MCP namespaces and keys are minted via `make apikey`.
	clerkSecretKey := os.Getenv("CLERK_SECRET_KEY")
	if clerkSecretKey == "" {
		log.Warn("CLERK_SECRET_KEY not set; key-management API (/api/keys) disabled")
	}

	handler, err := mcpx.Handler(mcpx.Options{
		Log:               log,
		AllowExternalHost: allowExternalHost,
		ClerkSecretKey:    clerkSecretKey,
		Deps:              mcpx.Deps{Log: log, DB: pool, Ctx: ctx},
		OnMount: func(path string) {
			log.Info("mounted route", "path", path)
		},
	})
	if err != nil {
		log.Error("build handler", "err", err)
		os.Exit(1)
	}

	addr := ":" + envOr("PORT", "8080")
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM (ctx is the signal context created
	// above; cancelling it also stops namespace background workers).
	go func() {
		log.Info("listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "err", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// openDB connects to Postgres and verifies the connection. It returns
// (nil, nil) when DATABASE_URL is unset, so namespaces that do not need a
// database still start; those that do will report the missing dependency
// themselves.
func openDB(ctx context.Context, log *slog.Logger) (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Warn("DATABASE_URL not set; namespaces requiring Postgres will fail to mount")
		return nil, nil
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	log.Info("database connected")
	return pool, nil
}
