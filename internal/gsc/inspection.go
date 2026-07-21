package gsc

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
	searchconsole "google.golang.org/api/searchconsole/v1"
)

// maxBatchURLs bounds the batch inspection tools so a single call cannot fan out
// unbounded requests against the (quota-limited) URL Inspection API.
const maxBatchURLs = 10

// registerInspectionTools wires the URL Inspection tools.
func registerInspectionTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_inspect_url",
		Description: "Inspect a single URL's Google index status (URL Inspection API): verdict, coverage state, " +
			"indexing state, last crawl time, crawled-as, Google/user canonical, robots.txt state, referring " +
			"URLs, plus mobile-usability and rich-results verdicts and the live report link.",
	}, c.inspectURL)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_batch_inspect_urls",
		Description: fmt.Sprintf("Inspect up to %d URLs concurrently and return each one's index status. "+
			"Provide urls as a newline- or comma-separated list.", maxBatchURLs),
	}, c.batchInspectURLs)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_check_indexing_issues",
		Description: fmt.Sprintf("Inspect up to %d URLs and summarize indexing problems: which are indexed vs not, "+
			"and buckets for canonical mismatches, robots.txt blocks and fetch failures. Best for auditing a set "+
			"of important pages.", maxBatchURLs),
	}, c.checkIndexingIssues)
}

// urlInspection is the flattened, tool-friendly view of an inspection result.
type urlInspection struct {
	URL             string   `json:"url"`
	Verdict         string   `json:"verdict,omitempty"`
	CoverageState   string   `json:"coverage_state,omitempty"`
	IndexingState   string   `json:"indexing_state,omitempty"`
	LastCrawlTime   string   `json:"last_crawl_time,omitempty"`
	CrawledAs       string   `json:"crawled_as,omitempty"`
	GoogleCanonical string   `json:"google_canonical,omitempty"`
	UserCanonical   string   `json:"user_canonical,omitempty"`
	RobotsTxtState  string   `json:"robots_txt_state,omitempty"`
	PageFetchState  string   `json:"page_fetch_state,omitempty"`
	ReferringURLs   []string `json:"referring_urls,omitempty"`
	Sitemaps        []string `json:"sitemaps,omitempty"`
	MobileVerdict   string   `json:"mobile_usability_verdict,omitempty"`
	RichResults     string   `json:"rich_results_verdict,omitempty"`
	ReportLink      string   `json:"report_link,omitempty"`
	Error           string   `json:"error,omitempty"`
}

func flattenInspection(url string, res *searchconsole.UrlInspectionResult) urlInspection {
	out := urlInspection{URL: url}
	if res == nil {
		return out
	}
	out.ReportLink = res.InspectionResultLink
	if idx := res.IndexStatusResult; idx != nil {
		out.Verdict = idx.Verdict
		out.CoverageState = idx.CoverageState
		out.IndexingState = idx.IndexingState
		out.LastCrawlTime = idx.LastCrawlTime
		out.CrawledAs = idx.CrawledAs
		out.GoogleCanonical = idx.GoogleCanonical
		out.UserCanonical = idx.UserCanonical
		out.RobotsTxtState = idx.RobotsTxtState
		out.PageFetchState = idx.PageFetchState
		out.ReferringURLs = idx.ReferringUrls
		out.Sitemaps = idx.Sitemap
	}
	if m := res.MobileUsabilityResult; m != nil {
		out.MobileVerdict = m.Verdict
	}
	if r := res.RichResultsResult; r != nil {
		out.RichResults = r.Verdict
	}
	return out
}

// inspect runs a single URL Inspection call and flattens the result.
func (c *Client) inspect(ctx context.Context, siteURL, pageURL string) urlInspection {
	resp, err := c.svc.UrlInspection.Index.Inspect(&searchconsole.InspectUrlIndexRequest{
		SiteUrl:       siteURL,
		InspectionUrl: pageURL,
	}).Context(ctx).Do()
	if err != nil {
		return urlInspection{URL: pageURL, Error: err.Error()}
	}
	return flattenInspection(pageURL, resp.InspectionResult)
}

// inspectMany inspects a set of URLs concurrently (bounded), preserving input
// order in the returned slice.
func (c *Client) inspectMany(ctx context.Context, siteURL string, urls []string) []urlInspection {
	results := make([]urlInspection, len(urls))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5) // modest concurrency; the Inspection API is quota-sensitive
	for i, u := range urls {
		i, u := i, u
		g.Go(func() error {
			results[i] = c.inspect(gctx, siteURL, u)
			return nil
		})
	}
	_ = g.Wait() // individual errors are captured per-result; Wait never errors here
	return results
}

// --- gsc_inspect_url ---

type inspectInput struct {
	SiteURL string `json:"site_url" jsonschema:"the Search Console property that owns the URL"`
	PageURL string `json:"page_url" jsonschema:"the fully-qualified URL to inspect"`
}

func (c *Client) inspectURL(ctx context.Context, _ *mcp.CallToolRequest, in inspectInput) (*mcp.CallToolResult, urlInspection, error) {
	if err := c.ready(); err != nil {
		return toolErr[urlInspection]("%v", err)
	}
	if in.SiteURL == "" || in.PageURL == "" {
		return toolErr[urlInspection]("site_url and page_url are required")
	}
	res := c.inspect(ctx, in.SiteURL, in.PageURL)
	if res.Error != "" {
		return toolErr[urlInspection]("inspect %q: %s", in.PageURL, res.Error)
	}
	return jsonResult(res)
}

// --- gsc_batch_inspect_urls ---

type batchInspectInput struct {
	SiteURL string `json:"site_url" jsonschema:"the Search Console property"`
	URLs    string `json:"urls" jsonschema:"URLs to inspect, one per line or comma-separated (max 10)"`
}

type batchInspectOutput struct {
	SiteURL string          `json:"site_url"`
	Count   int             `json:"count"`
	Results []urlInspection `json:"results"`
}

func (c *Client) batchInspectURLs(ctx context.Context, _ *mcp.CallToolRequest, in batchInspectInput) (*mcp.CallToolResult, batchInspectOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[batchInspectOutput]("%v", err)
	}
	urls, errMsg := parseBatchURLs(in.SiteURL, in.URLs)
	if errMsg != "" {
		return toolErr[batchInspectOutput]("%s", errMsg)
	}
	results := c.inspectMany(ctx, in.SiteURL, urls)
	return jsonResult(batchInspectOutput{SiteURL: in.SiteURL, Count: len(results), Results: results})
}

// --- gsc_check_indexing_issues ---

type indexingSummary struct {
	Total             int      `json:"total"`
	Indexed           int      `json:"indexed"`
	NotIndexed        int      `json:"not_indexed"`
	CanonicalMismatch []string `json:"canonical_mismatch"`
	RobotsBlocked     []string `json:"robots_blocked"`
	FetchFailed       []string `json:"fetch_failed"`
	Errored           []string `json:"errored"`
}

type checkIssuesOutput struct {
	SiteURL string          `json:"site_url"`
	Summary indexingSummary `json:"summary"`
	Results []urlInspection `json:"results"`
}

func (c *Client) checkIndexingIssues(ctx context.Context, _ *mcp.CallToolRequest, in batchInspectInput) (*mcp.CallToolResult, checkIssuesOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[checkIssuesOutput]("%v", err)
	}
	urls, errMsg := parseBatchURLs(in.SiteURL, in.URLs)
	if errMsg != "" {
		return toolErr[checkIssuesOutput]("%s", errMsg)
	}
	results := c.inspectMany(ctx, in.SiteURL, urls)

	sum := indexingSummary{Total: len(results)}
	for _, r := range results {
		switch {
		case r.Error != "":
			sum.Errored = append(sum.Errored, r.URL)
		case r.Verdict == "PASS":
			// PASS is the URL Inspection API's verdict for a URL that is on Google.
			sum.Indexed++
		default:
			sum.NotIndexed++
		}
		// Independent issue buckets — a URL can land in more than one.
		if r.GoogleCanonical != "" && r.UserCanonical != "" && r.GoogleCanonical != r.UserCanonical {
			sum.CanonicalMismatch = append(sum.CanonicalMismatch, r.URL)
		}
		if r.RobotsTxtState != "" && r.RobotsTxtState != "ALLOWED" {
			sum.RobotsBlocked = append(sum.RobotsBlocked, r.URL)
		}
		if r.PageFetchState != "" && r.PageFetchState != "SUCCESSFUL" {
			sum.FetchFailed = append(sum.FetchFailed, r.URL)
		}
	}

	return jsonResult(checkIssuesOutput{SiteURL: in.SiteURL, Summary: sum, Results: results})
}

// parseBatchURLs validates the shared batch input and returns the URL list or a
// user-facing error message.
func parseBatchURLs(siteURL, raw string) (urls []string, errMsg string) {
	if siteURL == "" {
		return nil, "site_url is required"
	}
	urls = splitLines(raw)
	if len(urls) == 0 {
		return nil, "urls is required (one URL per line or comma-separated)"
	}
	if len(urls) > maxBatchURLs {
		return nil, fmt.Sprintf("too many URLs: %d provided, max is %d", len(urls), maxBatchURLs)
	}
	return urls, ""
}
