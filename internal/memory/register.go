// Package memory hosts the /memory/mcp namespace. For now it exposes a single
// dummy tool used to verify the server connection end to end.
package memory

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/memory/mcp" }

func (namespace) Server() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "memory", Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "memory_ping",
		Description: "Dummy tool that confirms the memory namespace is reachable.",
	}, ping)
	return s
}

type pingInput struct {
	Message string `json:"message,omitempty" jsonschema:"optional message to echo back"`
}

type pingOutput struct {
	Reply string `json:"reply"`
}

func ping(_ context.Context, _ *mcp.CallToolRequest, in pingInput) (*mcp.CallToolResult, pingOutput, error) {
	reply := "pong from memory namespace"
	if in.Message != "" {
		reply = "memory received: " + in.Message
	}
	out := pingOutput{Reply: reply}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: reply}}}, out, nil
}
