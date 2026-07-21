package gsc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	searchconsole "google.golang.org/api/searchconsole/v1"
)

// newStubClient builds a Client whose Search Console service is pointed at a
// stub HTTP server, bypassing real Google credentials.
func newStubClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	svc, err := searchconsole.NewService(context.Background(),
		option.WithEndpoint(ts.URL),
		option.WithoutAuthentication(),
		option.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("build stub service: %v", err)
	}
	return &Client{svc: svc, dataState: "all"}
}

func TestParseDimensions(t *testing.T) {
	cases := map[string][]string{
		"":               {"query"},
		"query":          {"query"},
		"query, page":    {"query", "page"},
		" page , device": {"page", "device"},
	}
	for in, want := range cases {
		got := parseDimensions(in)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("parseDimensions(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDataStateEnum(t *testing.T) {
	cases := map[string]string{"": "ALL", "all": "ALL", "All": "ALL", "final": "FINAL", "FINAL": "FINAL"}
	for in, want := range cases {
		if got := dataStateEnum(in); got != want {
			t.Errorf("dataStateEnum(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines("a\nb , c\n\n d ")
	want := []string{"a", "b", "c", "d"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("splitLines = %v, want %v", got, want)
	}
}

func TestParseBatchURLsLimit(t *testing.T) {
	many := strings.Repeat("https://x/\n", maxBatchURLs+1)
	if _, msg := parseBatchURLs("https://x/", many); msg == "" {
		t.Fatal("expected error when exceeding max batch size")
	}
	if _, msg := parseBatchURLs("", "https://x/"); msg == "" {
		t.Fatal("expected error when site_url missing")
	}
	urls, msg := parseBatchURLs("https://x/", "https://a/\nhttps://b/")
	if msg != "" || len(urls) != 2 {
		t.Fatalf("parseBatchURLs valid case: urls=%v msg=%q", urls, msg)
	}
}

func TestBuildRowsCTRAndKeys(t *testing.T) {
	rows := buildRows([]string{"query", "page"}, []*searchconsole.ApiDataRow{
		{Keys: []string{"shoes", "https://x/p"}, Clicks: 10, Impressions: 200, Ctr: 0.05, Position: 3.456},
	})
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.CTR != 5.0 {
		t.Errorf("CTR = %v, want 5.0 (percentage)", r.CTR)
	}
	if r.Position != 3.46 {
		t.Errorf("Position = %v, want 3.46", r.Position)
	}
	if r.Keys["query"] != "shoes" || r.Keys["page"] != "https://x/p" {
		t.Errorf("keys mapped wrong: %v", r.Keys)
	}
}

func TestNumericHelpers(t *testing.T) {
	if got := pct(10, 200); got != 5 {
		t.Errorf("pct = %v, want 5", got)
	}
	if got := pct(1, 0); got != 0 {
		t.Errorf("pct div-by-zero = %v, want 0", got)
	}
	if got := pctChange(100, 150); got != 50 {
		t.Errorf("pctChange = %v, want 50", got)
	}
	if got := pctChange(0, 5); got != 100 {
		t.Errorf("pctChange from zero = %v, want 100", got)
	}
	if got := round2(3.14159); got != 3.14 {
		t.Errorf("round2 = %v, want 3.14", got)
	}
}

func TestDestructiveGating(t *testing.T) {
	c := &Client{allowDestructive: false}
	if err := c.requireDestructive("gsc_add_site"); err == nil {
		t.Fatal("expected gating error when destructive ops disabled")
	}
	c.allowDestructive = true
	if err := c.requireDestructive("gsc_add_site"); err != nil {
		t.Fatalf("unexpected error when enabled: %v", err)
	}
}

func TestUnconfiguredClientReportsError(t *testing.T) {
	c := &Client{dataState: "all"} // svc nil
	res, _, err := c.listProperties(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler should not return protocol error, got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected IsError result when unconfigured")
	}
}

func TestListPropertiesRoundTrip(t *testing.T) {
	c := newStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/webmasters/v3/sites") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(searchconsole.SitesListResponse{
			SiteEntry: []*searchconsole.WmxSite{
				{SiteUrl: "https://a.com/", PermissionLevel: "siteOwner"},
				{SiteUrl: "sc-domain:b.com", PermissionLevel: "siteFullUser"},
			},
		})
	})

	_, out, err := c.listProperties(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("listProperties: %v", err)
	}
	if out.Count != 2 || out.Properties[0].SiteURL != "https://a.com/" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestSearchAnalyticsRoundTrip(t *testing.T) {
	c := newStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "searchAnalytics/query") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req searchconsole.SearchAnalyticsQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode query request: %v", err)
		}
		if req.DataState != "ALL" {
			t.Errorf("dataState = %q, want ALL", req.DataState)
		}
		if len(req.Dimensions) != 1 || req.Dimensions[0] != "query" {
			t.Errorf("dimensions = %v, want [query]", req.Dimensions)
		}
		_ = json.NewEncoder(w).Encode(searchconsole.SearchAnalyticsQueryResponse{
			Rows: []*searchconsole.ApiDataRow{
				{Keys: []string{"shoes"}, Clicks: 5, Impressions: 100, Ctr: 0.05, Position: 2.5},
			},
		})
	})

	_, out, err := c.searchAnalytics(context.Background(), nil, searchAnalyticsInput{SiteURL: "https://a.com/"})
	if err != nil {
		t.Fatalf("searchAnalytics: %v", err)
	}
	if out.RowCount != 1 || out.Rows[0].CTR != 5.0 || out.Rows[0].Keys["query"] != "shoes" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestInspectURLRoundTrip(t *testing.T) {
	c := newStubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "urlInspection/index:inspect") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(searchconsole.InspectUrlIndexResponse{
			InspectionResult: &searchconsole.UrlInspectionResult{
				InspectionResultLink: "https://search.google.com/report",
				IndexStatusResult: &searchconsole.IndexStatusInspectionResult{
					Verdict: "PASS", CoverageState: "Submitted and indexed", RobotsTxtState: "ALLOWED",
				},
			},
		})
	})

	_, out, err := c.inspectURL(context.Background(), nil, inspectInput{SiteURL: "https://a.com/", PageURL: "https://a.com/p"})
	if err != nil {
		t.Fatalf("inspectURL: %v", err)
	}
	if out.Verdict != "PASS" || out.ReportLink == "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestCapabilitiesUnconfigured(t *testing.T) {
	c := &Client{dataState: "final", err: context.Canceled}
	_, out, err := c.capabilities(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if out.Configured {
		t.Error("want Configured=false")
	}
	if out.DefaultDataState != "FINAL" {
		t.Errorf("DefaultDataState = %q, want FINAL", out.DefaultDataState)
	}
	if len(out.Tools) == 0 {
		t.Error("want a non-empty tool catalogue")
	}
}
