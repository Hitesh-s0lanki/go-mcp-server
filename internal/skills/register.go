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

	// Firecrawl works without a key at a lower rate limit, so a missing key is
	// not fatal -- unlike memory's database. Warn once at mount so the reduced
	// limit is not a surprise, then register the tools regardless.
	key := os.Getenv("FIRECRAWL_API_KEY")
	if key == "" && deps.Log != nil {
		deps.Log.Warn("FIRECRAWL_API_KEY not set; skills web tools will use Firecrawl's unauthenticated rate limit")
	}
	fc := NewFirecrawl(key)
	registerFirecrawlTools(s, fc)

	// The downloader needs no model or key (optional GITHUB_TOKEN just raises the
	// rate limit), so it is always available.
	registerDownloadTool(s, NewDownloader(os.Getenv("GITHUB_TOKEN")))

	// The on-demand skill finder drives an OpenAI tool-calling loop over the
	// Firecrawl tools, so it needs a chat model. Register it only when a key is
	// present; the Firecrawl primitives above stand alone without one.
	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" {
		chat := NewOpenAIChat(openaiKey, os.Getenv("SKILLS_AGENT_MODEL"))
		registerFindTool(s, &SkillFinder{Chat: chat, FC: fc})
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
