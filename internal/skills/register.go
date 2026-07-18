// Package skills hosts the /skills/mcp namespace. For now it exposes a single
// dummy tool used to verify the server connection end to end.
package skills

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/skills/mcp" }

func (namespace) Server(_ *mcpx.Deps) (*mcp.Server, error) {
	s := mcp.NewServer(&mcp.Implementation{Name: "skills", Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "skills_ping",
		Description: "Dummy tool that confirms the skills namespace is reachable.",
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
	reply := "pong from skills namespace"
	if in.Message != "" {
		reply = "skills received: " + in.Message
	}
	out := pingOutput{Reply: reply}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: reply}}}, out, nil
}
