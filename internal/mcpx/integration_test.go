package mcpx_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"

	// Register all namespaces via their init().
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/event"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/memory"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/skills"
)

// TestNamespacesRoundTrip mounts every registered namespace on an httptest
// server and drives a real MCP client through initialize + tools/call against
// each one, asserting the dummy tool echoes back.
func TestNamespacesRoundTrip(t *testing.T) {
	namespaces := mcpx.All()
	if len(namespaces) < 3 {
		t.Fatalf("want at least 3 namespaces registered, got %d", len(namespaces))
	}

	ts := httptest.NewServer(mcpx.Handler(nil))
	defer ts.Close()

	cases := map[string]string{
		"/memory/mcp": "memory_ping",
		"/skills/mcp": "skills_ping",
		"/event/mcp":  "event_ping",
	}

	for path, tool := range cases {
		t.Run(strings.Trim(path, "/"), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			transport := &mcp.StreamableClientTransport{Endpoint: ts.URL + path}

			session, err := client.Connect(ctx, transport, nil)
			if err != nil {
				t.Fatalf("connect %s: %v", path, err)
			}
			defer session.Close()

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
			text := firstText(res)
			if !strings.Contains(text, "received: hello") {
				t.Fatalf("%s: unexpected reply %q", tool, text)
			}
		})
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
