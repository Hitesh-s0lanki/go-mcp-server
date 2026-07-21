package producthunt

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/producthunt/mcp" }

// Server builds the Product Hunt MCP server.
//
// Like gsc (and unlike memory), a missing credential is not a mount failure: the
// namespace mounts regardless and every tool reports the configuration problem,
// so the rest of the server still boots. producthunt_capabilities surfaces the
// current auth status.
func (namespace) Server(deps *mcpx.Deps) (*mcp.Server, error) {
	client := NewClient(deps.Log)

	s := mcp.NewServer(&mcp.Implementation{Name: "producthunt", Version: "0.1.0"}, nil)

	registerCapabilitiesTool(s, client)
	registerPostTools(s, client)
	registerTopicTools(s, client)
	registerCollectionTools(s, client)
	registerUserTools(s, client)
	registerGraphQLTool(s, client)

	return s, nil
}

// registerCapabilitiesTool wires producthunt_capabilities: a self-describing
// tool that reports auth status and configuration without touching the network.
func registerCapabilitiesTool(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_capabilities",
		Description: "Report this Product Hunt server's configuration and readiness: whether credentials " +
			"resolved, the auth source, and the catalogue of available tools. Call this first if other tools error.",
	}, c.capabilities)
}

type capabilitiesOutput struct {
	Configured  bool     `json:"configured"`
	AuthSource  string   `json:"auth_source,omitempty"`
	ConfigError string   `json:"config_error,omitempty"`
	Endpoint    string   `json:"endpoint"`
	Tools       []string `json:"tools"`
	Notes       []string `json:"notes,omitempty"`
}

func (c *Client) capabilities(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, capabilitiesOutput, error) {
	out := capabilitiesOutput{
		Configured: c != nil && c.ready() == nil,
		Endpoint:   graphqlEndpoint,
		Tools: []string{
			"producthunt_capabilities",
			"producthunt_list_posts", "producthunt_get_post", "producthunt_get_post_comments",
			"producthunt_list_topics", "producthunt_get_topic",
			"producthunt_list_collections", "producthunt_get_collection",
			"producthunt_get_user", "producthunt_viewer",
			"producthunt_graphql",
		},
		Notes: []string{
			"read-only: only public queries are exposed; no mutations",
			"producthunt_viewer needs a user-scoped token (developer tokens are tied to their owner); client-credentials tokens have no viewer",
		},
	}
	if out.Configured {
		out.AuthSource = c.authSource
	} else if err := c.ready(); err != nil {
		out.ConfigError = err.Error()
	}
	return jsonResult(out)
}

// registerGraphQLTool wires the raw GraphQL escape hatch for anything the typed
// tools do not cover.
func registerGraphQLTool(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_graphql",
		Description: "Run an arbitrary GraphQL query against the Product Hunt v2 API and return the raw data. " +
			"Escape hatch for fields or queries the typed tools do not expose. Pass 'query' (a GraphQL document) " +
			"and optional 'variables'. The token's scope applies — default tokens are read-only.",
	}, c.rawGraphQL)
}

type rawGraphQLInput struct {
	Query     string         `json:"query" jsonschema:"the GraphQL query or document to execute"`
	Variables map[string]any `json:"variables,omitempty" jsonschema:"optional variables object referenced by the query"`
}

type rawGraphQLOutput struct {
	Data any `json:"data"`
}

func (c *Client) rawGraphQL(ctx context.Context, _ *mcp.CallToolRequest, in rawGraphQLInput) (*mcp.CallToolResult, rawGraphQLOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[rawGraphQLOutput]("%v", err)
	}
	if in.Query == "" {
		return toolErr[rawGraphQLOutput]("query is required")
	}
	var data any
	if err := c.graphql(ctx, in.Query, in.Variables, &data); err != nil {
		return toolErr[rawGraphQLOutput]("%v", err)
	}
	return jsonResult(rawGraphQLOutput{Data: data})
}
