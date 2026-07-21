package gsc

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	searchconsole "google.golang.org/api/searchconsole/v1"
)

// registerSitemapTools wires the sitemap tools.
func registerSitemapTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_list_sitemaps",
		Description: "List sitemaps for a property with status detail: submitted/downloaded dates, type, pending " +
			"flag, whether it is a sitemap index, error and warning counts, and per-content-type URL counts. " +
			"Pass sitemap_index to list the child sitemaps of an index file.",
	}, c.listSitemaps)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gsc_get_sitemap",
		Description: "Get full detail for a single sitemap by its full URL (feedpath), including content breakdown.",
	}, c.getSitemap)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_submit_sitemap",
		Description: "Submit (or resubmit) a sitemap to Search Console (Sitemaps.submit). Mutating; requires " +
			"GSC_ALLOW_DESTRUCTIVE=true.",
	}, c.submitSitemap)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_delete_sitemap",
		Description: "Delete/unsubmit a sitemap from Search Console (Sitemaps.delete). Mutating; requires " +
			"GSC_ALLOW_DESTRUCTIVE=true.",
	}, c.deleteSitemap)
}

type sitemapContent struct {
	Type      string `json:"type"`
	Submitted int64  `json:"submitted"`
	Indexed   int64  `json:"indexed"`
}

type sitemap struct {
	Path            string           `json:"path"`
	Type            string           `json:"type,omitempty"`
	LastSubmitted   string           `json:"last_submitted,omitempty"`
	LastDownloaded  string           `json:"last_downloaded,omitempty"`
	IsPending       bool             `json:"is_pending"`
	IsSitemapsIndex bool             `json:"is_sitemaps_index"`
	Warnings        int64            `json:"warnings"`
	Errors          int64            `json:"errors"`
	Contents        []sitemapContent `json:"contents,omitempty"`
}

func flattenSitemap(s *searchconsole.WmxSitemap) sitemap {
	out := sitemap{
		Path:            s.Path,
		Type:            s.Type,
		LastSubmitted:   s.LastSubmitted,
		LastDownloaded:  s.LastDownloaded,
		IsPending:       s.IsPending,
		IsSitemapsIndex: s.IsSitemapsIndex,
		Warnings:        s.Warnings,
		Errors:          s.Errors,
	}
	for _, cnt := range s.Contents {
		out.Contents = append(out.Contents, sitemapContent{Type: cnt.Type, Submitted: cnt.Submitted, Indexed: cnt.Indexed})
	}
	return out
}

// --- gsc_list_sitemaps ---

type listSitemapsInput struct {
	SiteURL      string `json:"site_url" jsonschema:"the Search Console property"`
	SitemapIndex string `json:"sitemap_index,omitempty" jsonschema:"optional sitemap-index URL to list child sitemaps of"`
}

type listSitemapsOutput struct {
	SiteURL      string    `json:"site_url"`
	Count        int       `json:"count"`
	PendingCount int       `json:"pending_count"`
	Sitemaps     []sitemap `json:"sitemaps"`
}

func (c *Client) listSitemaps(ctx context.Context, _ *mcp.CallToolRequest, in listSitemapsInput) (*mcp.CallToolResult, listSitemapsOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[listSitemapsOutput]("%v", err)
	}
	if in.SiteURL == "" {
		return toolErr[listSitemapsOutput]("site_url is required")
	}
	call := c.svc.Sitemaps.List(in.SiteURL)
	if in.SitemapIndex != "" {
		call = call.SitemapIndex(in.SitemapIndex)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return toolErr[listSitemapsOutput]("list sitemaps: %v", err)
	}
	out := listSitemapsOutput{SiteURL: in.SiteURL, Sitemaps: make([]sitemap, 0, len(resp.Sitemap))}
	for _, s := range resp.Sitemap {
		fs := flattenSitemap(s)
		if fs.IsPending {
			out.PendingCount++
		}
		out.Sitemaps = append(out.Sitemaps, fs)
	}
	out.Count = len(out.Sitemaps)
	return jsonResult(out)
}

// --- gsc_get_sitemap ---

type sitemapURLInput struct {
	SiteURL    string `json:"site_url" jsonschema:"the Search Console property"`
	SitemapURL string `json:"sitemap_url" jsonschema:"the full sitemap URL (feedpath), e.g. https://example.com/sitemap.xml"`
}

func (c *Client) getSitemap(ctx context.Context, _ *mcp.CallToolRequest, in sitemapURLInput) (*mcp.CallToolResult, sitemap, error) {
	if err := c.ready(); err != nil {
		return toolErr[sitemap]("%v", err)
	}
	if in.SiteURL == "" || in.SitemapURL == "" {
		return toolErr[sitemap]("site_url and sitemap_url are required")
	}
	s, err := c.svc.Sitemaps.Get(in.SiteURL, in.SitemapURL).Context(ctx).Do()
	if err != nil {
		return toolErr[sitemap]("get sitemap %q: %v", in.SitemapURL, err)
	}
	return jsonResult(flattenSitemap(s))
}

// --- gsc_submit_sitemap / gsc_delete_sitemap ---

func (c *Client) submitSitemap(ctx context.Context, _ *mcp.CallToolRequest, in sitemapURLInput) (*mcp.CallToolResult, mutationOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if err := c.requireDestructive("gsc_submit_sitemap"); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if in.SiteURL == "" || in.SitemapURL == "" {
		return toolErr[mutationOutput]("site_url and sitemap_url are required")
	}
	if err := c.svc.Sitemaps.Submit(in.SiteURL, in.SitemapURL).Context(ctx).Do(); err != nil {
		return toolErr[mutationOutput]("submit sitemap %q: %v", in.SitemapURL, err)
	}
	return jsonResult(mutationOutput{Status: "ok", SiteURL: in.SiteURL, Message: "sitemap submitted: " + in.SitemapURL})
}

func (c *Client) deleteSitemap(ctx context.Context, _ *mcp.CallToolRequest, in sitemapURLInput) (*mcp.CallToolResult, mutationOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if err := c.requireDestructive("gsc_delete_sitemap"); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if in.SiteURL == "" || in.SitemapURL == "" {
		return toolErr[mutationOutput]("site_url and sitemap_url are required")
	}
	if err := c.svc.Sitemaps.Delete(in.SiteURL, in.SitemapURL).Context(ctx).Do(); err != nil {
		return toolErr[mutationOutput]("delete sitemap %q: %v", in.SitemapURL, err)
	}
	return jsonResult(mutationOutput{Status: "ok", SiteURL: in.SiteURL, Message: "sitemap deleted: " + in.SitemapURL})
}
