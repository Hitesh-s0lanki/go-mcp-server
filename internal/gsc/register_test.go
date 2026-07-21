package gsc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"

	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/gsc"
)

// TestGSCNamespaceRoundTrip mounts the gsc namespace on its own (no database,
// no credentials) and drives a real MCP client through initialize + tools/list
// + a tools/call of gsc_capabilities. It proves the namespace self-registers,
// every tool's schema generates, and the server boots without credentials.
func TestGSCNamespaceRoundTrip(t *testing.T) {
	// Ensure no ambient credentials leak in and flip "configured" true.
	t.Setenv("GSC_CREDENTIALS_PATH", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")

	var ns mcpx.Namespace
	for _, n := range mcpx.All() {
		if n.Path() == "/gsc/mcp" {
			ns = n
		}
	}
	if ns == nil {
		t.Fatal("gsc namespace did not self-register")
	}

	srv, err := ns.Server(&mcpx.Deps{Ctx: context.Background()})
	if err != nil {
		t.Fatalf("build gsc server: %v", err)
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 17 {
		names := make([]string, 0, len(tools.Tools))
		for _, tl := range tools.Tools {
			names = append(names, tl.Name)
		}
		t.Fatalf("want 17 gsc tools, got %d: %v", len(tools.Tools), names)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gsc_capabilities"})
	if err != nil {
		t.Fatalf("call gsc_capabilities: %v", err)
	}
	if res.IsError {
		t.Fatalf("gsc_capabilities returned tool error: %+v", res.Content)
	}
	text := firstText(res)
	if !strings.Contains(text, "\"configured\": false") {
		t.Fatalf("expected configured=false without credentials, got: %s", text)
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
