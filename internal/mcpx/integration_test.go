package mcpx_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

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

	cases := map[string]string{
		"/skills/mcp": "skills_ping",
		"/event/mcp":  "event_ping",
	}

	for path, tool := range cases {
		t.Run(strings.Trim(path, "/"), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			session, err := client.Connect(ctx,
				&mcp.StreamableClientTransport{Endpoint: ts.URL + path}, nil)
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

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
