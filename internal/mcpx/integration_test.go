package mcpx_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"

	// Register all namespaces via their init().
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/event"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/memory"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/skills"
)

// TestNamespacesRoundTrip mounts every registered namespace and drives a real
// MCP client through initialize + tools/call against the dummy namespaces.
//
// It needs a database because the memory namespace refuses to mount without one
// and Handler fails fast. OPENAI_API_KEY is stubbed: the embedder is constructed
// at mount time but never called here, since this test only exercises the
// plumbing. Memory's own behaviour is covered in internal/memory.
func TestNamespacesRoundTrip(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	t.Setenv("OPENAI_API_KEY", "test-key-not-called")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	handler, err := mcpx.Handler(mcpx.Options{
		Log:  slog.New(slog.DiscardHandler),
		Deps: mcpx.Deps{DB: pool},
	})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Every namespace now sits behind RequireAPIKey, so the client must present
	// a registered key even to reach the dummy tools.
	keyed := &http.Client{Transport: keyRT{mintKey(t, pool)}}

	// The skills ping echoes its argument back; assert the round-trip.
	cases := map[string]string{
		"/skills/mcp": "skills_ping",
	}

	for path, tool := range cases {
		t.Run(strings.Trim(path, "/"), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			session, err := client.Connect(ctx,
				&mcp.StreamableClientTransport{Endpoint: ts.URL + path, HTTPClient: keyed}, nil)
			if err != nil {
				t.Fatalf("connect %s: %v", path, err)
			}
			defer func() { _ = session.Close() }()

			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tool,
				Arguments: map[string]any{"message": "hello"},
			})
			if err != nil {
				t.Fatalf("call %s: %v", tool, err)
			}
			if res.IsError {
				t.Fatalf("%s returned tool error: %+v", tool, res.Content)
			}
			if text := firstText(res); !strings.Contains(text, "received: hello") {
				t.Fatalf("%s: unexpected reply %q", tool, text)
			}
		})
	}

	// The event namespace has no echo tool; drive its no-network capabilities
	// tool instead. It mounts and answers even without Kafka configured.
	t.Run("event", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		session, err := client.Connect(ctx,
			&mcp.StreamableClientTransport{Endpoint: ts.URL + "/event/mcp", HTTPClient: keyed}, nil)
		if err != nil {
			t.Fatalf("connect /event/mcp: %v", err)
		}
		defer func() { _ = session.Close() }()

		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "event_capabilities"})
		if err != nil {
			t.Fatalf("call event_capabilities: %v", err)
		}
		if res.IsError {
			t.Fatalf("event_capabilities returned tool error: %+v", res.Content)
		}
		if text := firstText(res); !strings.Contains(text, "event_publish") {
			t.Fatalf("event_capabilities: unexpected reply %q", text)
		}
	})
}

// TestHandlerFailsWithoutDatabase pins the fail-fast contract: a namespace that
// cannot build must stop startup, not mount a server that only reveals the
// problem on the first tool call.
func TestHandlerFailsWithoutDatabase(t *testing.T) {
	_, err := mcpx.Handler(mcpx.Options{
		Log:  slog.New(slog.DiscardHandler),
		Deps: mcpx.Deps{}, // no DB
	})
	if err == nil {
		t.Fatal("want error when the memory namespace has no database, got nil")
	}
	if !strings.Contains(err.Error(), "/memory/mcp") {
		t.Fatalf("error should name the failing namespace, got: %v", err)
	}
}

// keyRT attaches an API key to every outbound request.
type keyRT struct{ key string }

func (k keyRT) RoundTrip(r *http.Request) (*http.Response, error) {
	if k.key != "" {
		r.Header.Set(auth.Header, k.key)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// mintKey inserts an api key directly and returns the string a client presents.
func mintKey(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	key := auth.GenerateKey()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO api_keys (key, label) VALUES ($1, 'mcpx-integration-test')`, key); err != nil {
		t.Fatalf("mint key: %v", err)
	}
	return key
}

// TestRequireAPIKey pins the admission contract that the audit was about: every
// namespace rejects an unauthenticated caller, and it does so at the transport
// rather than relying on each namespace to remember. /healthz stays open.
func TestRequireAPIKey(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	t.Setenv("OPENAI_API_KEY", "test-key-not-called")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	handler, err := mcpx.Handler(mcpx.Options{
		Log:  slog.New(slog.DiscardHandler),
		Deps: mcpx.Deps{DB: pool},
	})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	paths := []string{"/memory/mcp", "/skills/mcp", "/event/mcp"}

	t.Run("no key is rejected", func(t *testing.T) {
		for _, p := range paths {
			if got := status(t, "", http.MethodPost, ts.URL+p); got != http.StatusUnauthorized {
				t.Errorf("%s without a key: got %d, want 401", p, got)
			}
		}
	})

	t.Run("unregistered key is rejected", func(t *testing.T) {
		// Well-formed but never minted: the format check must not be mistaken
		// for proof the key exists.
		got := status(t, auth.GenerateKey(), http.MethodPost, ts.URL+"/skills/mcp")
		if got != http.StatusUnauthorized {
			t.Errorf("unregistered key: got %d, want 401", got)
		}
	})

	t.Run("healthz stays open", func(t *testing.T) {
		if got := status(t, "", http.MethodGet, ts.URL+healthzPath); got != http.StatusOK {
			t.Errorf("healthz without a key: got %d, want 200", got)
		}
	})
}

// TestKeyAPIRequiresClerk pins the second admission model: when a Clerk secret
// is configured the key-management API mounts, and it is gated by Clerk (a
// caller with no bearer token is rejected) rather than by X-API-Key. The MCP
// namespaces keep their own X-API-Key admission, unaffected.
func TestKeyAPIRequiresClerk(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	t.Setenv("OPENAI_API_KEY", "test-key-not-called")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	handler, err := mcpx.Handler(mcpx.Options{
		Log:            slog.New(slog.DiscardHandler),
		ClerkSecretKey: "sk_test_dummy", // mounts /api/keys; never verifies here
		Deps:           mcpx.Deps{DB: pool},
	})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// No Clerk token -> 401, before any JWKS fetch. An X-API-Key is not accepted
	// here: this route uses the Clerk model, not admission-key.
	if got := status(t, auth.GenerateKey(), http.MethodGet, ts.URL+"/api/keys"); got != http.StatusUnauthorized {
		t.Errorf("GET /api/keys without a Clerk token: got %d, want 401", got)
	}
}

// TestKeyAPIDisabledWithoutClerk confirms the API is absent (404) when no Clerk
// secret is configured, so the server runs MCP-only without it.
func TestKeyAPIDisabledWithoutClerk(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	t.Setenv("OPENAI_API_KEY", "test-key-not-called")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	handler, err := mcpx.Handler(mcpx.Options{
		Log:  slog.New(slog.DiscardHandler),
		Deps: mcpx.Deps{DB: pool}, // no ClerkSecretKey
	})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	if got := status(t, "", http.MethodGet, ts.URL+"/api/keys"); got != http.StatusNotFound {
		t.Errorf("GET /api/keys with Clerk disabled: got %d, want 404", got)
	}
}

// healthzPath mirrors the unauthenticated liveness route in registry.go.
const healthzPath = "/healthz"

// status sends one request carrying the given key (empty for none) and reports
// the status code.
func status(t *testing.T, key, method, url string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := (&http.Client{Transport: keyRT{key}}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = res.Body.Close() }()
	return res.StatusCode
}

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
