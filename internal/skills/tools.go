package skills

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultSearchLimit caps results when the caller does not specify one. Search
// costs 2 credits per 10 results, so a small default keeps casual queries cheap;
// the caller can raise it.
const defaultSearchLimit = 5

// snippetLen bounds inlined scraped content in a search result so one broad
// search does not dump many full pages into the context window. A caller that
// wants the whole page scrapes the URL directly with firecrawl_scrape.
const snippetLen = 1000

// maxConcurrentFinds bounds how many requirement agents run at once. Each runs
// its own OpenAI + Firecrawl loop, so this caps the fan-out against provider
// rate limits while still turning a batch of requests into parallel work.
const maxConcurrentFinds = 4

// registerFindTool wires the on-demand skill-finder onto s. It is registered
// only when an OpenAI key is available (the loop needs a chat model); the
// Firecrawl primitives below work without one.
func registerFindTool(s *mcp.Server, finder *SkillFinder) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "skills_find",
		Description: "THE tool for obtaining Agent Skills. Whenever you need a skill or " +
			"capability you don't already have -- or the user asks for one (\"get me a skill " +
			"for X\", \"find a skill that can Y\") -- call this instead of guessing. It runs a " +
			"live agent (OpenAI + web search/scrape) that finds the most relevant SKILL.md on " +
			"GitHub and returns its complete, ready-to-use content plus source links, in real " +
			"time. Nothing is cached -- every call reflects live GitHub. " +
			"Pass MULTIPLE needs at once via `requirements`: they are searched in PARALLEL, so " +
			"batching several is much faster than calling this once per requirement. " +
			"Then use skills_download to pull a found skill's full file set.",
	}, findHandler(finder))
}

type findInput struct {
	Requirement  string   `json:"requirement,omitempty" jsonschema:"a single need, in natural language, e.g. 'edit a PDF form'"`
	Requirements []string `json:"requirements,omitempty" jsonschema:"several needs at once; each is searched in parallel"`
}

// findOutput carries one result per requirement, in the order requested.
type findOutput struct {
	Results []FindResult `json:"results"`
}

func findHandler(finder *SkillFinder) mcp.ToolHandlerFor[findInput, findOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in findInput) (*mcp.CallToolResult, findOutput, error) {
		reqs := gatherRequirements(in)
		if len(reqs) == 0 {
			return errResult(errors.New("requirement is required (set `requirement` or `requirements`)")), findOutput{}, nil
		}

		// Fan out: one agent per requirement, bounded by a semaphore. Each
		// goroutine owns its result slot, so results stay ordered without a lock,
		// and a single failure is recorded per-requirement rather than aborting
		// the batch.
		results := make([]FindResult, len(reqs))
		sem := make(chan struct{}, maxConcurrentFinds)
		var wg sync.WaitGroup
		for i, rq := range reqs {
			wg.Add(1)
			go func(i int, rq string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				res, err := finder.Find(ctx, rq)
				if err != nil {
					results[i] = FindResult{Requirement: rq, Error: err.Error()}
					return
				}
				results[i] = *res
			}(i, rq)
		}
		wg.Wait()

		out := findOutput{Results: results}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: renderFindResults(results)}},
		}, out, nil
	}
}

// gatherRequirements merges the singular and plural inputs and drops blanks.
func gatherRequirements(in findInput) []string {
	var reqs []string
	if strings.TrimSpace(in.Requirement) != "" {
		reqs = append(reqs, in.Requirement)
	}
	for _, r := range in.Requirements {
		if strings.TrimSpace(r) != "" {
			reqs = append(reqs, r)
		}
	}
	return reqs
}

func renderFindResults(results []FindResult) string {
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n\n" + strings.Repeat("=", 60) + "\n\n")
		}
		fmt.Fprintf(&b, "requirement: %s\n\n", r.Requirement)
		if r.Error != "" {
			fmt.Fprintf(&b, "error: %s\n", r.Error)
			continue
		}
		b.WriteString(r.Skill)
		if len(r.Sources) > 0 {
			b.WriteString("\n\nsources:\n- " + strings.Join(r.Sources, "\n- "))
		}
	}
	return b.String()
}

// registerDownloadTool wires the complete-skill downloader onto s. It needs no
// model or key, so it is always available.
func registerDownloadTool(s *mcp.Server, d *Downloader) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "skills_download",
		Description: "Download a COMPLETE Agent Skill from GitHub: every file in the skill " +
			"folder (SKILL.md plus any scripts and reference files, recursively), fetched " +
			"concurrently and returned in full. Give it a skill location -- a GitHub URL " +
			"(repo, tree, blob, or raw SKILL.md link) or 'owner/repo/path'. Use this after " +
			"skills_find to actually install a skill you located; skills_find returns the " +
			"SKILL.md, this returns the whole package.",
	}, downloadHandler(d))
}

type downloadInput struct {
	Source string `json:"source" jsonschema:"the skill to download: a GitHub URL (repo/tree/blob/raw) or 'owner/repo/path'"`
}

func downloadHandler(d *Downloader) mcp.ToolHandlerFor[downloadInput, DownloadResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in downloadInput) (*mcp.CallToolResult, DownloadResult, error) {
		if strings.TrimSpace(in.Source) == "" {
			return errResult(errors.New("source is required")), DownloadResult{}, nil
		}
		res, err := d.Download(ctx, in.Source)
		if err != nil {
			return errResult(err), DownloadResult{}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: renderDownload(res)}},
		}, *res, nil
	}
}

func renderDownload(r *DownloadResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "skill %q from %s (%d files, %d bytes)\n", r.Name, r.Source, len(r.Files), r.TotalBytes)
	for _, f := range r.Files {
		if f.Error != "" {
			fmt.Fprintf(&b, "\n--- %s (ERROR: %s) ---\n", f.Path, f.Error)
			continue
		}
		fmt.Fprintf(&b, "\n--- %s (%d bytes) ---\n%s\n", f.Path, f.Bytes, f.Content)
	}
	return b.String()
}

// registerFirecrawlTools wires the search + scrape tools onto s.
func registerFirecrawlTools(s *mcp.Server, fc *Firecrawl) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "firecrawl_search",
		Description: "Search the web and get back ranked results (url, title, description). " +
			"Set scrape=true to also fetch each result's page content as markdown " +
			"(more credits, slower). Use this to discover pages; use firecrawl_scrape " +
			"to read one page in full.",
	}, searchHandler(fc))

	mcp.AddTool(s, &mcp.Tool{
		Name: "firecrawl_scrape",
		Description: "Fetch a single URL and return its content as clean markdown " +
			"(and optionally raw HTML). Use this to read a page you already have the " +
			"URL for; use firecrawl_search to find pages first.",
	}, scrapeHandler(fc))
}

type searchInput struct {
	Query  string `json:"query" jsonschema:"what to search the web for"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results (default 5)"`
	Scrape bool   `json:"scrape,omitempty" jsonschema:"also fetch each result's page content as markdown (default false)"`
}

type searchOutput struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}

func searchHandler(fc *Firecrawl) mcp.ToolHandlerFor[searchInput, searchOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		if strings.TrimSpace(in.Query) == "" {
			return errResult(errors.New("query is required")), searchOutput{}, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = defaultSearchLimit
		}

		req := SearchRequest{Query: in.Query, Limit: limit, Sources: []string{"web"}}
		if in.Scrape {
			req.ScrapeOptions = &SearchScrapeOpts{Formats: []string{"markdown"}}
		}

		results, err := fc.Search(ctx, req)
		if err != nil {
			return errResult(err), searchOutput{}, nil
		}
		for i := range results {
			results[i].Markdown = clip(results[i].Markdown, snippetLen)
		}

		out := searchOutput{Query: in.Query, Results: results}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: renderSearch(out)}},
		}, out, nil
	}
}

type scrapeInput struct {
	URL                string `json:"url" jsonschema:"the URL to fetch"`
	IncludeHTML        bool   `json:"include_html,omitempty" jsonschema:"also return raw HTML alongside markdown (default false)"`
	IncludeBoilerplate bool   `json:"include_boilerplate,omitempty" jsonschema:"keep nav/header/footer/boilerplate; default false returns only the main content"`
	FullContent        bool   `json:"full_content,omitempty" jsonschema:"return the whole page instead of a bounded snippet (default false)"`
}

type scrapeOutput struct {
	URL      string         `json:"url"`
	Markdown string         `json:"markdown"`
	HTML     string         `json:"html,omitempty"`
	Metadata ScrapeMetadata `json:"metadata"`
}

func scrapeHandler(fc *Firecrawl) mcp.ToolHandlerFor[scrapeInput, scrapeOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in scrapeInput) (*mcp.CallToolResult, scrapeOutput, error) {
		if strings.TrimSpace(in.URL) == "" {
			return errResult(errors.New("url is required")), scrapeOutput{}, nil
		}

		formats := []string{"markdown"}
		if in.IncludeHTML {
			formats = append(formats, "html")
		}

		res, err := fc.Scrape(ctx, ScrapeRequest{
			URL:     in.URL,
			Formats: formats,
			// Defaults to true; the caller opts out with include_boilerplate.
			OnlyMainContent: !in.IncludeBoilerplate,
		})
		if err != nil {
			return errResult(err), scrapeOutput{}, nil
		}

		md := res.Markdown
		if !in.FullContent {
			md = clip(md, snippetLen*8)
		}

		out := scrapeOutput{
			URL:      in.URL,
			Markdown: md,
			HTML:     res.HTML,
			Metadata: res.Metadata,
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: md}},
		}, out, nil
	}
}

// errResult reports a failure to the model rather than the transport, so it can
// read what went wrong and adjust. Mirrors memory's convention.
func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

// clip trims s to n runes, appending an ellipsis when it was cut.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// renderSearch produces the text view of a search result set.
func renderSearch(out searchOutput) string {
	if len(out.Results) == 0 {
		return fmt.Sprintf("no web results for %q", out.Query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d results for %q:\n", len(out.Results), out.Query)
	for _, r := range out.Results {
		fmt.Fprintf(&b, "\n%s\n%s\n", r.Title, r.URL)
		if r.Description != "" {
			fmt.Fprintf(&b, "%s\n", r.Description)
		}
		if r.Markdown != "" {
			fmt.Fprintf(&b, "%s\n", r.Markdown)
		}
	}
	return b.String()
}
