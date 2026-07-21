package event

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/event/mcp" }

// Server builds the event (Kafka) MCP server.
//
// Unlike memory (which hard-requires Postgres), a missing broker or Confluent
// credential is not a mount failure: the namespace mounts regardless and every
// tool reports the configuration problem, so the rest of the server still boots.
// event_capabilities surfaces the current status.
func (namespace) Server(deps *mcpx.Deps) (*mcp.Server, error) {
	ctx := deps.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	client := NewClient(ctx, deps.Log)

	s := mcp.NewServer(&mcp.Implementation{Name: "event", Version: "0.1.0"}, nil)

	registerCapabilitiesTool(s, client)
	registerPublishTool(s, client)
	registerConsumeTool(s, client)
	registerTopicTools(s, client)

	return s, nil
}

// registerCapabilitiesTool wires event_capabilities: a self-describing tool that
// reports connection status and configuration without touching the network.
func registerCapabilitiesTool(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "event_capabilities",
		Description: "Report this event (Kafka) server's configuration and readiness: whether a broker and " +
			"Confluent Cloud credentials are configured, the default consumer group and topic, whether topic " +
			"admin (create/delete) is enabled, and the catalogue of available tools. Call this first if other " +
			"tools error.",
	}, c.capabilities)
}

type capabilitiesOutput struct {
	Configured      bool     `json:"configured"`
	Bootstrap       string   `json:"bootstrap,omitempty"`
	AuthConfigured  bool     `json:"auth_configured"`
	ConfigError     string   `json:"config_error,omitempty"`
	DefaultGroup    string   `json:"default_group"`
	DefaultTopic    string   `json:"default_topic,omitempty"`
	AllowTopicAdmin bool     `json:"allow_topic_admin"`
	Tools           []string `json:"tools"`
	Notes           []string `json:"notes,omitempty"`
}

func (c *Client) capabilities(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, capabilitiesOutput, error) {
	out := capabilitiesOutput{
		Configured:      c != nil && c.kc != nil,
		Bootstrap:       c.bootstrap,
		AuthConfigured:  c.authConfigured,
		DefaultGroup:    c.defaultGroup,
		DefaultTopic:    c.defaultTopic,
		AllowTopicAdmin: c.allowTopicAdmin,
		Tools: []string{
			"event_capabilities",
			"event_publish", "event_consume",
			"event_topics", "event_create_topic", "event_delete_topic",
		},
	}
	if !out.Configured && c.err != nil {
		out.ConfigError = c.err.Error()
		out.Notes = append(out.Notes, "set KAFKA_BOOTSTRAP_SERVERS, KAFKA_API_KEY and KAFKA_API_SECRET to enable the Kafka tools")
	}
	if !c.allowTopicAdmin {
		out.Notes = append(out.Notes, "topic create/delete are disabled; set KAFKA_ALLOW_TOPIC_ADMIN=true to enable event_create_topic / event_delete_topic")
	}
	return jsonResult(out)
}
