// Package mcpx holds cross-namespace infrastructure: the namespace registry
// and shared HTTP middleware. Domain packages (memory, skills, event) depend on
// mcpx; mcpx never depends on them.
package mcpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Deps are the shared resources handed to every namespace at build time.
// Fields may be nil when the corresponding config is absent; a namespace that
// requires one must return an error from Server rather than panic later.
type Deps struct {
	Log *slog.Logger
	DB  *pgxpool.Pool

	// Ctx is the process lifetime: it is cancelled when the server begins
	// shutting down. Namespaces that spawn background goroutines (e.g. memory's
	// embedding workers) should tie them to this so they stop cleanly. Nil is
	// treated as context.Background() by consumers.
	Ctx context.Context
}

// Namespace is one mounted MCP server. Each domain package implements this and
// registers it from an init() so cmd/server never has to change when a new
// namespace is added.
type Namespace interface {
	// Path is the exact HTTP route the namespace mounts on, e.g. "/memory/mcp".
	Path() string
	// Server builds the MCP server with its tools registered. It returns an
	// error if a required dependency is missing or misconfigured.
	Server(*Deps) (*mcp.Server, error)
}

var registry []Namespace

// Register adds a namespace. Call it from a domain package's init().
func Register(ns Namespace) { registry = append(registry, ns) }

// All returns every registered namespace.
func All() []Namespace { return registry }

// Chain wraps h with the given middleware, applied outermost-first so the first
// argument runs first on the way in.
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// Options configures the mounted handler.
type Options struct {
	// Log receives request and mount logs. Defaults to slog.Default().
	Log *slog.Logger

	// Deps are passed to every namespace. Optional; Log is filled in from
	// Options.Log when unset.
	Deps Deps

	// AllowExternalHost disables the SDK's DNS-rebinding protection.
	//
	// By default the transport rejects (403) any request that arrives on a
	// loopback address carrying a non-loopback Host header. That is exactly
	// what a reverse proxy or tunnel (ngrok, Cloudflare Tunnel) looks like, so
	// fronting this server with one requires setting this to true.
	//
	// Only enable it when a trusted proxy is in front: it removes the guard
	// against DNS-rebinding attacks from the browser.
	// See https://modelcontextprotocol.io/specification/2025-11-25/basic/security_best_practices
	AllowExternalHost bool

	// OnMount is called once per namespace after it is mounted. Optional.
	OnMount func(path string)
}

// Handler builds an http.Handler that mounts every registered namespace on its
// path over the Streamable HTTP transport, plus a GET /healthz endpoint.
//
// It fails fast: if any namespace cannot build (missing dependency, bad config)
// the error is returned rather than mounting a half-working server that only
// reveals the problem on the first tool call.
func Handler(opts Options) (http.Handler, error) {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	deps := opts.Deps
	if deps.Log == nil {
		deps.Log = log
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	streamOpts := &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: opts.AllowExternalHost,
	}

	for _, ns := range All() {
		srv, err := ns.Server(&deps)
		if err != nil {
			return nil, fmt.Errorf("namespace %s: %w", ns.Path(), err)
		}
		mux.Handle(ns.Path(), mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return srv }, streamOpts,
		))
		if opts.OnMount != nil {
			opts.OnMount(ns.Path())
		}
	}

	return Chain(mux, Recover(log), LogRequests(log)), nil
}
