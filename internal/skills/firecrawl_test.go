package skills

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient points a Firecrawl client at a stub server and returns both.
func newTestClient(t *testing.T, h http.HandlerFunc) *Firecrawl {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	fc := NewFirecrawl("test-key")
	fc.baseURL = ts.URL
	return fc
}

func TestSearch(t *testing.T) {
	fc := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header = %q, want bearer test-key", got)
		}
		var req SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Query != "golang" {
			t.Errorf("query = %q, want golang", req.Query)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"web": []map[string]any{
					{"url": "https://go.dev", "title": "Go", "description": "The Go language"},
				},
			},
		})
	})

	got, err := fc.Search(context.Background(), SearchRequest{Query: "golang"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].URL != "https://go.dev" {
		t.Fatalf("unexpected results: %+v", got)
	}
}

func TestScrape(t *testing.T) {
	fc := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req ScrapeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.URL != "https://go.dev" {
			t.Errorf("url = %q, want https://go.dev", req.URL)
		}
		if !req.OnlyMainContent {
			t.Errorf("onlyMainContent = false, want true")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"markdown": "# Go",
				"metadata": map[string]any{"title": "Go", "statusCode": 200},
			},
		})
	})

	got, err := fc.Scrape(context.Background(), ScrapeRequest{URL: "https://go.dev", OnlyMainContent: true})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if got.Markdown != "# Go" || got.Metadata.Title != "Go" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

// TestErrorBody pins that a non-200 with a Firecrawl {"error": ...} body is
// surfaced verbatim rather than swallowed into a bare status.
func TestErrorBody(t *testing.T) {
	fc := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(w, `{"error":"insufficient credits"}`)
	})

	_, err := fc.Scrape(context.Background(), ScrapeRequest{URL: "https://go.dev"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if want := "insufficient credits"; !contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// TestNoAuthHeaderWhenKeyEmpty pins that an empty key omits Authorization
// entirely (Firecrawl allows unauthenticated calls) rather than sending
// "Bearer ".
func TestNoAuthHeaderWhenKeyEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			t.Errorf("Authorization header present with empty key")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{}})
	}))
	t.Cleanup(ts.Close)

	fc := NewFirecrawl("")
	fc.baseURL = ts.URL
	if _, err := fc.Search(context.Background(), SearchRequest{Query: "x"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
