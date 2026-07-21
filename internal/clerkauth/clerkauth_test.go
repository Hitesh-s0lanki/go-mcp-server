package clerkauth_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/clerkauth"
)

// TestRequireUserRejectsBadCredentials pins the auth boundary that does not need
// a live Clerk instance: a request with no bearer token, or a token that isn't
// even a well-formed JWT, is rejected with 401 before any JWKS fetch — so the
// wrapped handler never runs. (Verifying a genuine token is exercised
// end-to-end against Clerk, not here.)
func TestRequireUserRejectsBadCredentials(t *testing.T) {
	authn, err := clerkauth.New("sk_test_dummy", slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	reached := false
	h := authn.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	cases := map[string]string{
		"no header":       "",
		"not bearer":      "Basic abc",
		"malformed token": "Bearer not-a-jwt",
		"empty bearer":    "Bearer ",
	}

	for name, authHeader := range cases {
		t.Run(name, func(t *testing.T) {
			reached = false
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/keys", nil)
			if authHeader != "" {
				req.Header.Set("Authorization", authHeader)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got status %d, want 401", rec.Code)
			}
			if reached {
				t.Error("wrapped handler ran for an unauthenticated request")
			}
		})
	}
}

// TestNewRequiresSecret ensures a missing secret fails fast at construction
// rather than silently rejecting every caller at request time.
func TestNewRequiresSecret(t *testing.T) {
	if _, err := clerkauth.New("", nil); err == nil {
		t.Fatal("want error for empty secret key, got nil")
	}
}
