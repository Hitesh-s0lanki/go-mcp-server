// Package skills hosts the /skills/mcp namespace. It exposes web tools backed by
// Firecrawl -- search and single-page scrape -- plus a dummy ping used to verify
// the connection end to end.
package skills

import (
	"context"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/skills/mcp" }

func (namespace) Server(deps *mcpx.Deps) (*mcp.Server, error) {
	s := mcp.NewServer(&mcp.Implementation{Name: "skills", Version: "0.1.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "skills_ping",
		Description: "Dummy tool that confirms the skills namespace is reachable.",
	}, ping)

	// The web search/scrape client is an internal dependency of the skills_find
	// agent, not an exposed tool -- callers reach it only through skills_find.
	// It works without a key at a lower rate limit, so a missing one is not
	// fatal; warn once at mount so the reduced limit is not a surprise.
	key := os.Getenv("FIRECRAWL_API_KEY")
	if key == "" && deps.Log != nil {
		deps.Log.Warn("FIRECRAWL_API_KEY not set; skills web search/scrape will use the unauthenticated rate limit")
	}
	web := NewFirecrawl(key)

	// The downloader needs no model or key -- it reads public repos through
	// GitHub's public API unauthenticated -- so it is always available.
	registerDownloadTool(s, NewDownloader())

	// skills_find drives an OpenAI tool-calling loop over the web client above,
	// so it needs a chat model. Register it only when a key is present.
	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" {
		chat := NewOpenAIChat(openaiKey, os.Getenv("SKILLS_AGENT_MODEL"))
		registerFindTool(s, &SkillFinder{Chat: chat, FC: web})
	} else if deps.Log != nil {
		deps.Log.Warn("OPENAI_API_KEY not set; skills_find (on-demand skill agent) is disabled")
	}

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
