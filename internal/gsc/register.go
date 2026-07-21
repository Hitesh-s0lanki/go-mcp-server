package gsc

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/gsc/mcp" }

// Server builds the Google Search Console MCP server.
//
// Unlike memory (which hard-requires Postgres), a missing GSC credential is not
// a mount failure: the namespace mounts regardless and every tool reports the
// configuration problem, so the rest of the server still boots. gsc_capabilities
// surfaces the current auth status.
func (namespace) Server(deps *mcpx.Deps) (*mcp.Server, error) {
	ctx := deps.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	client := NewClient(ctx, deps.Log)

	s := mcp.NewServer(&mcp.Implementation{Name: "gsc", Version: "0.1.0"}, nil)

	registerCapabilitiesTool(s, client)
	registerSiteTools(s, client)
	registerAnalyticsTools(s, client)
	registerInspectionTools(s, client)
	registerSitemapTools(s, client)

	return s, nil
}

// registerCapabilitiesTool wires gsc_capabilities: a self-describing tool that
// reports auth status and configuration without touching the network.
func registerCapabilitiesTool(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_capabilities",
		Description: "Report this Google Search Console server's configuration and readiness: whether " +
			"credentials resolved, the auth source, the default data-state, whether mutating operations are " +
			"enabled, and the catalogue of available tools. Call this first if other tools error.",
	}, c.capabilities)
}

type capabilitiesOutput struct {
	Configured       bool     `json:"configured"`
	AuthSource       string   `json:"auth_source,omitempty"`
	ConfigError      string   `json:"config_error,omitempty"`
	DefaultDataState string   `json:"default_data_state"`
	AllowDestructive bool     `json:"allow_destructive"`
	Tools            []string `json:"tools"`
	Notes            []string `json:"notes,omitempty"`
}

func (c *Client) capabilities(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, capabilitiesOutput, error) {
	out := capabilitiesOutput{
		Configured:       c != nil && c.svc != nil,
		DefaultDataState: dataStateEnum(c.dataState),
		AllowDestructive: c.allowDestructive,
		Tools: []string{
			"gsc_capabilities",
			"gsc_list_properties", "gsc_get_site_details", "gsc_add_site", "gsc_delete_site",
			"gsc_search_analytics", "gsc_advanced_search_analytics", "gsc_performance_overview",
			"gsc_compare_periods", "gsc_search_by_page_query",
			"gsc_inspect_url", "gsc_batch_inspect_urls", "gsc_check_indexing_issues",
			"gsc_list_sitemaps", "gsc_get_sitemap", "gsc_submit_sitemap", "gsc_delete_sitemap",
		},
	}
	if out.Configured {
		out.AuthSource = c.authSource
	} else if c.err != nil {
		out.ConfigError = c.err.Error()
	}
	if !c.allowDestructive {
		out.Notes = append(out.Notes, "mutating tools (add/delete site, submit/delete sitemap) are disabled; set GSC_ALLOW_DESTRUCTIVE=true to enable")
	}
	return jsonResult(out)
}
