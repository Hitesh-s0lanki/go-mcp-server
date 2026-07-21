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
	"github.com/Hitesh-s0lanki/go-mcp-server/internal/memory"
)

type hdrRT struct{ key string }

func (h hdrRT) RoundTrip(r *http.Request) (*http.Response, error) {
	if h.key != "" {
		r.Header.Set(memory.APIKeyHeader, h.key)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func session(t *testing.T, url, key string) *mcp.ClientSession {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "skill", Version: "0"}, nil)
	s, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: url, HTTPClient: &http.Client{Transport: hdrRT{key}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// postStatus sends a bare POST with the given key (empty for none) and reports
// the status code, for asserting transport-level admission without opening an
// MCP session.
func postStatus(t *testing.T, url, key string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := &http.Client{Transport: hdrRT{key}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	return res.StatusCode
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

// mintKey inserts an api key directly and returns the key string a client would
// present in the X-API-Key header.
func mintKey(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	key := memory.GenerateAPIKey()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO api_keys (key, label) VALUES ($1, 'protocol-test')`, key); err != nil {
		t.Fatalf("mint key: %v", err)
	}
	return key
}

// Simulates the stateful-memory protocol as an agent would run it.
func TestSkillFlow(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("needs DATABASE_URL + OPENAI_API_KEY")
	}
	pool, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	defer pool.Close()
	_, _ = pool.Exec(context.Background(), "TRUNCATE memories")

	key := mintKey(t, pool)
	otherKey := mintKey(t, pool)

	h, err := mcpx.Handler(mcpx.Options{Log: slog.New(slog.DiscardHandler), Deps: mcpx.Deps{DB: pool}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()
	url := ts.URL + "/memory/mcp"

	// 1. No key -> fails closed at the transport. This used to surface as a tool
	// error result the model could read; mcpx.RequireAPIKey now rejects the
	// request at the mux, so the session never opens and no tool is dispatched.
	if got := postStatus(t, url, ""); got != http.StatusUnauthorized {
		t.Fatalf("no key: got %d, want 401", got)
	}

	// 1b. A well-formed but unregistered key -> rejected too. Matching the
	// minted format is not proof the key exists.
	if got := postStatus(t, url, memory.GenerateAPIKey()); got != http.StatusUnauthorized {
		t.Fatalf("unregistered key: got %d, want 401", got)
	}

	// 2. Registered key: identity resolves (what .mcp.json provides).
	s := session(t, url, key)
	defer func() { _ = s.Close() }()

	// REMEMBER: store a user-profile fact + a task summary.
	call(t, s, "memory_write", map[string]any{
		"content": "Prefers Go stdlib over frameworks; wants tests run against real infra.",
		"tags":    []string{"user-profile"},
	})
	call(t, s, "memory_write", map[string]any{
		"content": "[TASK] Hybrid memory search\nWhat: vector + BM25 with RRF.\nDecisions: min_score 0.35; identity via the X-API-Key header.\nFiles: internal/memory/store.go.",
		"tags":    []string{"task-summary", "project:go-mcp-server", "topic:memory"},
	})

	// RECALL at "session start": load profile by tag (no embedding).
	prof := call(t, s, "memory_list", map[string]any{"tags": []string{"user-profile"}})
	if !strings.Contains(text(prof), "stdlib") {
		t.Fatalf("profile recall failed: %s", text(prof))
	}

	// RECALL a prior task semantically. Embedding happens off the write path, so
	// the vector may land a moment after the write returns; poll briefly.
	var found *mcp.CallToolResult
	deadline := time.Now().Add(10 * time.Second)
	for {
		found = call(t, s, "memory_search", map[string]any{
			"query": "how did we build retrieval that mixes meaning and keywords",
		})
		if strings.Contains(text(found), "RRF") || time.Now().After(deadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !strings.Contains(text(found), "RRF") {
		t.Fatalf("semantic recall of task summary failed: %s", text(found))
	}
	t.Logf("session-start profile recall:\n%s", text(prof))
	t.Logf("semantic task recall:\n%s", text(found))

	// 3. Isolation: a different key sees none of it.
	other := session(t, url, otherKey)
	defer func() { _ = other.Close() }()
	r2 := call(t, other, "memory_search", map[string]any{"query": "hybrid memory search RRF"})
	if strings.Contains(text(r2), "RRF") {
		t.Fatalf("cross-key leak: other key saw the summary")
	}
}
