// Package clerkauth verifies Clerk session JWTs and exposes the caller's Clerk
// user id to downstream handlers.
//
// It is the user-authentication layer for the dashboard's key-management API
// (internal/keysapi), and is deliberately separate from internal/auth: that
// package does X-API-Key admission for the MCP namespaces (the key IS the
// credential), whereas this one proves *which human* is calling so keys can be
// scoped to and capped per Clerk user.
package clerkauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
)

type ctxKey int

const userIDKey ctxKey = 0

// Authenticator verifies Clerk session tokens presented as `Authorization:
// Bearer <jwt>`. It caches JSON Web Keys by key id: Clerk rotates signing keys
// rarely, so after the first request per key the verification is fully local
// (no network round trip), and a rotation simply misses the cache and refetches.
type Authenticator struct {
	log *slog.Logger

	mu   sync.RWMutex
	jwks map[string]*clerk.JSONWebKey
}

// New configures the Clerk SDK with secretKey (which is what lets it fetch the
// instance's JWKS) and returns an Authenticator. It errors on an empty key so a
// misconfiguration fails at startup rather than silently rejecting every caller.
func New(secretKey string, log *slog.Logger) (*Authenticator, error) {
	if secretKey == "" {
		return nil, errors.New("clerkauth: CLERK_SECRET_KEY is required")
	}
	if log == nil {
		log = slog.Default()
	}
	// SetKey configures the SDK's default backend, which jwt.GetJSONWebKey uses
	// to fetch this instance's JWKS.
	clerk.SetKey(secretKey)
	return &Authenticator{log: log, jwks: make(map[string]*clerk.JSONWebKey)}, nil
}

// jsonWebKey returns the signing key for kid, fetching and caching it on first
// use. Concurrent misses may both fetch; that is harmless and cheaper than
// holding the write lock across a network call.
func (a *Authenticator) jsonWebKey(ctx context.Context, kid string) (*clerk.JSONWebKey, error) {
	a.mu.RLock()
	key, ok := a.jwks[kid]
	a.mu.RUnlock()
	if ok {
		return key, nil
	}

	key, err := jwt.GetJSONWebKey(ctx, &jwt.GetJSONWebKeyParams{KeyID: kid})
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.jwks[kid] = key
	a.mu.Unlock()
	return key, nil
}

// RequireUser admits only requests carrying a valid, unexpired Clerk session
// token, storing the caller's user id in the request context for the handler to
// read via UserID. A missing or invalid token is 401; an inability to reach
// Clerk's JWKS is 503, so an outage is not reported as a credential failure.
func (a *Authenticator) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		// Decode (without verifying) only to read the key id, so we can select
		// the right signing key before the real verification below.
		unsafe, err := jwt.Decode(r.Context(), &jwt.DecodeParams{Token: token})
		if err != nil {
			a.log.Warn("clerk token decode failed", "err", err, "remote", r.RemoteAddr)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		key, err := a.jsonWebKey(r.Context(), unsafe.KeyID)
		if err != nil {
			a.log.Error("fetch clerk jwks failed", "err", err)
			http.Error(w, "cannot verify credentials", http.StatusServiceUnavailable)
			return
		}

		claims, err := jwt.Verify(r.Context(), &jwt.VerifyParams{Token: token, JWK: key})
		if err != nil {
			a.log.Warn("clerk token verify failed", "err", err, "remote", r.RemoteAddr)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if claims.Subject == "" {
			http.Error(w, "token has no subject", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserID returns the authenticated Clerk user id that RequireUser stored, and
// whether it was present. Handlers behind RequireUser can rely on ok being true.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}

// bearerToken extracts the token from an `Authorization: Bearer <token>` header,
// or "" when the header is absent or not a bearer credential.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
