package memory_test

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

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
	_ "github.com/Hitesh-s0lanki/go-mcp-server/internal/memory"
)

type hdrRT struct{ email string }

func (h hdrRT) RoundTrip(r *http.Request) (*http.Response, error) {
	if h.email != "" {
		r.Header.Set("X-User-Email", h.email)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func session(t *testing.T, url, email string) *mcp.ClientSession {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "skill", Version: "0"}, nil)
	s, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: url, HTTPClient: &http.Client{Transport: hdrRT{email}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return r
}

func text(r *mcp.CallToolResult) string {
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// Simulates the CLAUDE.md protocol as an agent would run it.
func TestSkillFlow(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("needs DATABASE_URL + OPENAI_API_KEY")
	}
	pool, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	defer pool.Close()
	_, _ = pool.Exec(context.Background(), "TRUNCATE memories")

	h, err := mcpx.Handler(mcpx.Options{Log: slog.New(slog.DiscardHandler), Deps: mcpx.Deps{DB: pool}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()
	url := ts.URL + "/memory/mcp"

	// 1. Header missing -> identity fails closed.
	anon := session(t, url, "")
	r := call(t, anon, "memory_list", map[string]any{"tags": []string{"user-profile"}})
	if !r.IsError || !strings.Contains(text(r), "X-User-Email") {
		t.Fatalf("expected identity error without header, got: %+v", text(r))
	}
	_ = anon.Close()

	// 2. Agent session with identity (what .mcp.json header provides).
	s := session(t, url, "hitesh.solanki@strique.io")
	defer func() { _ = s.Close() }()

	// REMEMBER: store a user-profile fact + a task summary.
	call(t, s, "memory_write", map[string]any{
		"content": "Prefers Go stdlib over frameworks; wants tests run against real infra.",
		"tags":    []string{"user-profile"},
	})
	call(t, s, "memory_write", map[string]any{
		"content": "[TASK] Hybrid memory search\nWhat: vector + BM25 with RRF.\nDecisions: min_score 0.35; identity via X-User-Email header.\nFiles: internal/memory/store.go.",
		"tags":    []string{"task-summary", "project:go-mcp-server", "topic:memory"},
	})

	// RECALL at "session start": load profile by tag (no embedding).
	prof := call(t, s, "memory_list", map[string]any{"tags": []string{"user-profile"}})
	if !strings.Contains(text(prof), "stdlib") {
		t.Fatalf("profile recall failed: %s", text(prof))
	}

	// RECALL a prior task semantically (query shares almost no words).
	found := call(t, s, "memory_search", map[string]any{
		"query": "how did we build retrieval that mixes meaning and keywords",
	})
	if !strings.Contains(text(found), "RRF") {
		t.Fatalf("semantic recall of task summary failed: %s", text(found))
	}
	t.Logf("session-start profile recall:\n%s", text(prof))
	t.Logf("semantic task recall:\n%s", text(found))

	// 3. Isolation: a different user sees none of it.
	other := session(t, url, "someone-else@example.com")
	defer func() { _ = other.Close() }()
	r2 := call(t, other, "memory_search", map[string]any{"query": "hybrid memory search RRF"})
	if strings.Contains(text(r2), "RRF") {
		t.Fatalf("cross-user leak: other user saw the summary")
	}
}
