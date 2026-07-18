package mcpx

import (
	"log/slog"
	"net/http"
	"time"
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
