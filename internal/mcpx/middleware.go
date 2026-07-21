package mcpx

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
)

// statusRecorder captures the response status while preserving the interfaces
// the Streamable HTTP transport depends on. It MUST forward Flush: the
// transport streams SSE, and a wrapper that swallows http.Flusher would buffer
// the stream and hang the client.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// LogRequests logs each request on arrival and on completion. The arrival line
// matters for debugging connectivity: an SSE stream can stay open for a long
// time, so completion-only logging looks like "nothing is happening".
func LogRequests(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"host", r.Host,
				"remote", r.RemoteAddr,
			)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			log.Info("response",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"dur", time.Since(start).String(),
			)
		})
	}
}

// RequireAPIKey wraps a namespace handler, rejecting any request that does not
// present a registered X-API-Key before it reaches the namespace.
//
// Admission is enforced by this middleware rather than per-tool: a namespace
// that forgets the check would otherwise be reachable unauthenticated, which is
// exactly how skills, event, gsc and producthunt ended up open while only
// memory verified the key. It wraps each namespace route individually, so
// unauthenticated routes (/healthz) and Clerk-authenticated ones (/api/keys)
// simply are not wrapped with it.
//
// Namespaces that need the caller's identity (memory, for row scoping) still
// resolve the key themselves. That is not a redundant round trip: the resolver
// caches, so the second lookup is a map hit, and it keeps admission and scoping
// independently correct rather than coupling one to the other's bookkeeping.
func RequireAPIKey(res *auth.Resolver, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := strings.TrimSpace(r.Header.Get(auth.Header))
			if key == "" {
				// Log the rejection but never the key itself.
				log.Warn("unauthenticated request", "path", r.URL.Path, "remote", r.RemoteAddr)
				http.Error(w, auth.ErrNoKey.Error(), http.StatusUnauthorized)
				return
			}

			if _, err := res.Resolve(r.Context(), key); err != nil {
				// A malformed or unregistered key is the caller's problem (401);
				// anything else is ours (503), and must not be reported as a
				// credential failure or an operator will chase the wrong bug.
				if errors.Is(err, auth.ErrMalformed) || errors.Is(err, auth.ErrInvalidKey) {
					log.Warn("rejected api key", "path", r.URL.Path, "remote", r.RemoteAddr, "reason", err)
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
				log.Error("api key lookup failed", "path", r.URL.Path, "err", err)
				http.Error(w, "cannot verify credentials", http.StatusServiceUnavailable)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Recover turns a panic in a handler into a 500 instead of taking the process
// down with it.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					log.Error("panic", "path", r.URL.Path, "value", v)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
