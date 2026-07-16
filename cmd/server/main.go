// Command server wires every registered MCP namespace onto one HTTP mux over
// the Streamable HTTP transport and serves them.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"

	// Blank imports run each namespace's init(), which self-registers it with
	// mcpx. Adding a namespace = add its package here.
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/event"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/memory"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/skills"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	handler := mcpx.Handler(func(path string) {
		log.Info("mounted namespace", "path", path)
	})

	addr := ":" + envOr("PORT", "8080")
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
