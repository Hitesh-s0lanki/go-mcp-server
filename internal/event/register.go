// Package event hosts the /event/mcp namespace. For now it exposes a single
// dummy tool used to verify the server connection end to end.
package event

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/event/mcp" }

func (namespace) Server(_ *mcpx.Deps) (*mcp.Server, error) {
	s := mcp.NewServer(&mcp.Implementation{Name: "event", Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "event_ping",
		Description: "Dummy tool that confirms the event namespace is reachable.",
	}, ping)
	return s, nil
}

type pingInput struct {
	Message string `json:"message,omitempty" jsonschema:"optional message to echo back"`
}

type pingOutput struct {
	Reply string `json:"reply"`
}

func ping(_ context.Context, _ *mcp.CallToolRequest, in pingInput) (*mcp.CallToolResult, pingOutput, error) {
	reply := "pong from event namespace"
	if in.Message != "" {
		reply = "event received: " + in.Message
	}
	out := pingOutput{Reply: reply}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: reply}}}, out, nil
}
