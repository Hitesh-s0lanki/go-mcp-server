package producthunt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newStubClient builds a Client whose GraphQL endpoint is a stub HTTP server and
// whose auth is a static token, bypassing any real network or credentials.
func newStubClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return &Client{
		httpClient: ts.Client(),
		endpoint:   ts.URL,
		token:      "test-token",
		authSource: "developer/access token (PRODUCTHUNT_TOKEN)",
	}
}

func TestClampFirst(t *testing.T) {
	cases := []struct{ n, def, max, want int }{
		{0, 10, 20, 10},
		{-5, 10, 20, 10},
		{5, 10, 20, 5},
		{50, 10, 20, 20},
		{20, 10, 20, 20},
	}
	for _, c := range cases {
		if got := clampFirst(c.n, c.def, c.max); got != c.want {
			t.Errorf("clampFirst(%d,%d,%d) = %d, want %d", c.n, c.def, c.max, got, c.want)
		}
	}
}

func TestSetIf(t *testing.T) {
	vars := map[string]any{}
	setIf(vars, "a", "x")
	setIf(vars, "b", "")
	if vars["a"] != "x" {
		t.Errorf("setIf non-empty: got %v", vars["a"])
	}
	if _, ok := vars["b"]; ok {
		t.Error("setIf empty should not add the key")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "z"); got != "z" {
		t.Errorf("firstNonEmpty = %q, want z", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty all empty = %q, want empty", got)
	}
}

func TestReadyUnconfigured(t *testing.T) {
	c := &Client{err: context.Canceled}
	if err := c.ready(); err == nil {
		t.Fatal("want error when no credentials configured")
	}
	c2 := &Client{token: "tok"}
	if err := c2.ready(); err != nil {
		t.Fatalf("want nil with token, got %v", err)
	}
}

func TestUnconfiguredClientReportsToolError(t *testing.T) {
	c := &Client{} // no token, no client creds
	res, _, err := c.listPosts(context.Background(), nil, listPostsInput{})
	if err != nil {
		t.Fatalf("handler should not return a protocol error, got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected IsError result when unconfigured")
	}
}

func TestListPostsRoundTrip(t *testing.T) {
	c := newStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		var req graphqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !strings.Contains(req.Query, "posts(") {
			t.Errorf("query missing posts field: %s", req.Query)
		}
		if req.Variables["first"] != float64(10) && req.Variables["first"] != 10 {
			t.Errorf("first variable = %v, want 10", req.Variables["first"])
		}
		if req.Variables["featured"] != true {
			t.Errorf("featured variable = %v, want true", req.Variables["featured"])
		}
		_, _ = io.WriteString(w, `{"data":{"posts":{"pageInfo":{"endCursor":"CUR","hasNextPage":true},"edges":[{"node":{"id":"1","name":"Thing","tagline":"a thing","slug":"thing","votesCount":42}}]}}}`)
	})

	featured := true
	_, out, err := c.listPosts(context.Background(), nil, listPostsInput{Featured: &featured})
	if err != nil {
		t.Fatalf("listPosts: %v", err)
	}
	if out.Count != 1 || out.Posts[0].Name != "Thing" || out.Posts[0].VotesCount != 42 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if out.EndCursor != "CUR" || !out.HasNextPage {
		t.Fatalf("pagination not surfaced: %+v", out)
	}
}

func TestGraphQLSurfacesErrors(t *testing.T) {
	c := newStubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errors":[{"message":"boom"}]}`)
	})
	res, _, err := c.getPost(context.Background(), nil, postRefInput{Slug: "x"})
	if err != nil {
		t.Fatalf("handler should not return a protocol error, got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected IsError result when GraphQL returns errors")
	}
	if txt := firstText(res); !strings.Contains(txt, "boom") {
		t.Fatalf("error text should surface the GraphQL message, got %q", txt)
	}
}

func TestRateLimitMessage(t *testing.T) {
	c := newStubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Rate-Limit-Reset", "900")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	res, _, _ := c.getPost(context.Background(), nil, postRefInput{ID: "1"})
	if res == nil || !res.IsError {
		t.Fatal("expected IsError result on 429")
	}
	if txt := firstText(res); !strings.Contains(txt, "900") {
		t.Fatalf("rate-limit message should include the reset seconds, got %q", txt)
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
