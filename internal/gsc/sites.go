package gsc

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerSiteTools wires the property (site) management tools.
func registerSiteTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_list_properties",
		Description: "List every Search Console property (site) the authenticated principal can access, " +
			"with its permission level. Use this first to discover the exact site_url other tools expect " +
			"(e.g. \"https://example.com/\" or a domain property \"sc-domain:example.com\").",
	}, c.listProperties)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gsc_get_site_details",
		Description: "Get details (permission level) for a single Search Console property identified by site_url.",
	}, c.getSiteDetails)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_add_site",
		Description: "Add a property to the account (Sites.add). Mutating; requires GSC_ALLOW_DESTRUCTIVE=true. " +
			"The site must already be verifiable by the authenticated principal.",
	}, c.addSite)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gsc_delete_site",
		Description: "Remove a property from the account (Sites.delete). Mutating; requires GSC_ALLOW_DESTRUCTIVE=true.",
	}, c.deleteSite)
}

type siteURLInput struct {
	SiteURL string `json:"site_url" jsonschema:"the Search Console property, e.g. https://example.com/ or sc-domain:example.com"`
}

type property struct {
	SiteURL         string `json:"site_url"`
	PermissionLevel string `json:"permission_level"`
}

type listPropertiesOutput struct {
	Count      int        `json:"count"`
	Properties []property `json:"properties"`
}

func (c *Client) listProperties(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listPropertiesOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[listPropertiesOutput]("%v", err)
	}
	resp, err := c.svc.Sites.List().Context(ctx).Do()
	if err != nil {
		return toolErr[listPropertiesOutput]("list properties: %v", err)
	}
	out := listPropertiesOutput{Properties: make([]property, 0, len(resp.SiteEntry))}
	for _, s := range resp.SiteEntry {
		out.Properties = append(out.Properties, property{SiteURL: s.SiteUrl, PermissionLevel: s.PermissionLevel})
	}
	out.Count = len(out.Properties)
	return jsonResult(out)
}

func (c *Client) getSiteDetails(ctx context.Context, _ *mcp.CallToolRequest, in siteURLInput) (*mcp.CallToolResult, property, error) {
	if err := c.ready(); err != nil {
		return toolErr[property]("%v", err)
	}
	if in.SiteURL == "" {
		return toolErr[property]("site_url is required")
	}
	s, err := c.svc.Sites.Get(in.SiteURL).Context(ctx).Do()
	if err != nil {
		return toolErr[property]("get site %q: %v", in.SiteURL, err)
	}
	return jsonResult(property{SiteURL: s.SiteUrl, PermissionLevel: s.PermissionLevel})
}

type mutationOutput struct {
	Status  string `json:"status"`
	SiteURL string `json:"site_url"`
	Message string `json:"message"`
}

func (c *Client) addSite(ctx context.Context, _ *mcp.CallToolRequest, in siteURLInput) (*mcp.CallToolResult, mutationOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if err := c.requireDestructive("gsc_add_site"); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if in.SiteURL == "" {
		return toolErr[mutationOutput]("site_url is required")
	}
	if err := c.svc.Sites.Add(in.SiteURL).Context(ctx).Do(); err != nil {
		return toolErr[mutationOutput]("add site %q: %v", in.SiteURL, err)
	}
	return jsonResult(mutationOutput{Status: "ok", SiteURL: in.SiteURL, Message: "property added"})
}

func (c *Client) deleteSite(ctx context.Context, _ *mcp.CallToolRequest, in siteURLInput) (*mcp.CallToolResult, mutationOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if err := c.requireDestructive("gsc_delete_site"); err != nil {
		return toolErr[mutationOutput]("%v", err)
	}
	if in.SiteURL == "" {
		return toolErr[mutationOutput]("site_url is required")
	}
	if err := c.svc.Sites.Delete(in.SiteURL).Context(ctx).Do(); err != nil {
		return toolErr[mutationOutput]("delete site %q: %v", in.SiteURL, err)
	}
	return jsonResult(mutationOutput{Status: "ok", SiteURL: in.SiteURL, Message: "property removed"})
}
