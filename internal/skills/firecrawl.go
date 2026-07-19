package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// firecrawlBaseURL is the v2 API root. The two endpoints this package uses,
// /search and /scrape, hang off it.
const firecrawlBaseURL = "https://api.firecrawl.dev/v2"

// maxRespBytes caps how much of a Firecrawl response we read. Scraped markdown
// for a heavy page can be large; this keeps a pathological response from
// exhausting memory while staying well above any realistic single page.
const maxRespBytes = 1 << 24 // 16 MiB

// Firecrawl is a thin client over the Firecrawl HTTP API.
//
// Like memory's OpenAIEmbedder this is hand-rolled rather than pulling in the
// vendor SDK: it is two endpoints behind one bearer token, and the SDK would be
// a large dependency for two POSTs. The API key is optional -- Firecrawl serves
// unauthenticated requests at a lower rate limit -- so an empty key is valid and
// simply omits the Authorization header.
type Firecrawl struct {
	APIKey string
	Client *http.Client

	// baseURL is the API root. Unexported and set only to the firecrawlBaseURL
	// const in production (tests, in-package, override it), so it is not a
	// user-controlled taint source: the scrape/search target rides in the body.
	baseURL string
}

// NewFirecrawl builds a client. An empty apiKey is allowed; requests then go out
// unauthenticated (rate-limited by Firecrawl).
func NewFirecrawl(apiKey string) *Firecrawl {
	return &Firecrawl{
		APIKey:  apiKey,
		baseURL: firecrawlBaseURL,
		// Scraping can render JS and take tens of seconds; this is a hard ceiling.
		// The per-call context deadline governs the common case.
		Client: &http.Client{Timeout: 120 * time.Second},
	}
}

// --- search ---

// SearchRequest is the JSON body for POST /search. Only the fields this server
// exposes are modelled; Firecrawl accepts more.
type SearchRequest struct {
	Query          string            `json:"query"`
	Limit          int               `json:"limit,omitempty"`
	Sources        []string          `json:"sources,omitempty"`
	IncludeDomains []string          `json:"includeDomains,omitempty"`
	ScrapeOptions  *SearchScrapeOpts `json:"scrapeOptions,omitempty"`
}

// SearchScrapeOpts asks Firecrawl to scrape each result and inline the content.
type SearchScrapeOpts struct {
	Formats []string `json:"formats,omitempty"`
}

// SearchResult is one hit from a search. markdown is populated only when the
// caller requested scraping.
type SearchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Position    int    `json:"position"`
	Markdown    string `json:"markdown,omitempty"`
}

type searchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Web []SearchResult `json:"web"`
	} `json:"data"`
	Error string `json:"error"`
}

// Search runs a web search and returns the "web" results.
func (f *Firecrawl) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	var out searchResponse
	if err := f.do(ctx, "/search", req, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, apiErr("search", out.Error)
	}
	return out.Data.Web, nil
}

// --- scrape ---

// ScrapeRequest is the JSON body for POST /scrape.
type ScrapeRequest struct {
	URL             string   `json:"url"`
	Formats         []string `json:"formats,omitempty"`
	OnlyMainContent bool     `json:"onlyMainContent"`
}

// ScrapeMetadata is the subset of page metadata Firecrawl always returns.
type ScrapeMetadata struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Language    string `json:"language"`
	SourceURL   string `json:"sourceURL"`
	StatusCode  int    `json:"statusCode"`
}

// ScrapeResult is the scraped page. markdown/html are present per requested
// formats.
type ScrapeResult struct {
	Markdown string         `json:"markdown"`
	HTML     string         `json:"html"`
	Metadata ScrapeMetadata `json:"metadata"`
}

type scrapeResponse struct {
	Success bool         `json:"success"`
	Data    ScrapeResult `json:"data"`
	Error   string       `json:"error"`
}

// Scrape fetches one URL and returns its content in the requested formats.
func (f *Firecrawl) Scrape(ctx context.Context, req ScrapeRequest) (*ScrapeResult, error) {
	var out scrapeResponse
	if err := f.do(ctx, "/scrape", req, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, apiErr("scrape", out.Error)
	}
	return &out.Data, nil
}

// --- transport ---

// do marshals body, POSTs it to path, and decodes the response into out.
func (f *Firecrawl) do(ctx context.Context, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if f.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+f.APIKey)
	}

	resp, err := f.Client.Do(req)
	if err != nil {
		return fmt.Errorf("firecrawl request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Firecrawl returns a JSON {"error": "..."} body on failure; surface it
		// verbatim when present, otherwise fall back to the status.
		if msg := decodeError(data); msg != "" {
			return fmt.Errorf("firecrawl %s: status %d: %s", path, resp.StatusCode, msg)
		}
		return fmt.Errorf("firecrawl %s: status %d", path, resp.StatusCode)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// decodeError best-effort extracts the "error" field from a failure body.
func decodeError(data []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &e)
	return e.Error
}

func apiErr(op, msg string) error {
	if msg == "" {
		msg = "request unsuccessful"
	}
	return fmt.Errorf("firecrawl %s: %s", op, msg)
}
