package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// maxAgentSteps bounds the finder's OpenAI round trips. One search plus a few
// scrapes is the normal path; this is the runaway backstop, not a target.
const maxAgentSteps = 8

// agentSystemPrompt steers the loop. It is deliberately explicit about *where*
// skills live and *what* "complete" means, because the model only sees GitHub
// through the two tools below.
const agentSystemPrompt = `You find and fetch Agent Skills from GitHub on demand.

An "Agent Skill" is a folder containing a SKILL.md file: YAML frontmatter (name,
description) followed by markdown instructions. They live in repos like
anthropics/skills (skills/<name>/SKILL.md) and community collections
(e.g. *awesome-claude-skills* lists that link out to individual skill repos).

Given a requirement, do this:
1. Use search_github to locate candidate skills. Prefer the official
   anthropics/skills repo and well-known collections; refine the query if the
   first results are only list pages rather than actual skills.
2. Use fetch_url to read the most relevant candidate's SKILL.md. GitHub "blob"
   pages work, but the raw file is cleaner:
   https://raw.githubusercontent.com/<owner>/<repo>/<branch>/<path>/SKILL.md
3. Return the COMPLETE SKILL.md content verbatim (frontmatter + full body), not a
   summary. If the skill references other files (scripts, references), list them
   as links but do not fetch every one.

Stop as soon as you have the skill(s) that satisfy the requirement. If nothing on
GitHub fits, say so plainly. Your final message is delivered to the caller as-is,
so make it the ready-to-use skill: a short note on why it fits, the source URL,
then the full SKILL.md in a fenced block.`

// searchToolSchema / fetchToolSchema are the JSON Schemas advertised to OpenAI.
var searchToolSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "web search query; append 'SKILL.md' or a repo name to bias toward real skills"},
    "limit": {"type": "integer", "description": "max results (default 6)"}
  },
  "required": ["query"]
}`)

var fetchToolSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "url": {"type": "string", "description": "URL to fetch as markdown; prefer raw.githubusercontent.com for file contents"}
  },
  "required": ["url"]
}`)

func agentTools() []chatTool {
	return []chatTool{
		{Type: "function", Function: functionDef{
			Name:        "search_github",
			Description: "Search the web (results biased to github.com) for Agent Skills matching a query.",
			Parameters:  searchToolSchema,
		}},
		{Type: "function", Function: functionDef{
			Name:        "fetch_url",
			Description: "Fetch a single URL and return its content as markdown (use for SKILL.md files and repo pages).",
			Parameters:  fetchToolSchema,
		}},
	}
}

// SkillFinder runs a bounded OpenAI tool-calling loop that discovers and fetches
// skills through Firecrawl in real time. It holds no state between calls and
// persists nothing: every Find is a fresh search against live GitHub.
type SkillFinder struct {
	Chat *OpenAIChat
	FC   *Firecrawl
}

// FindResult is what one Find call produced. Error is set instead of the others
// when this single requirement failed, so a batch reports per-requirement
// outcomes rather than collapsing on the first failure.
type FindResult struct {
	Requirement string   `json:"requirement"`
	Skill       string   `json:"skill"`           // the assembled, ready-to-use skill content
	Sources     []string `json:"sources"`         // URLs fetched during the search
	Steps       int      `json:"steps"`           // OpenAI round trips used
	Error       string   `json:"error,omitempty"` // set when this requirement failed
}

// Find satisfies a natural-language requirement by locating and fetching the
// most relevant skill(s) from GitHub.
func (f *SkillFinder) Find(ctx context.Context, requirement string) (*FindResult, error) {
	messages := []chatMessage{
		{Role: "system", Content: agentSystemPrompt},
		{Role: "user", Content: "Requirement: " + requirement},
	}
	tools := agentTools()

	var sources []string
	for step := 1; step <= maxAgentSteps; step++ {
		msg, err := f.Chat.Complete(ctx, messages, tools)
		if err != nil {
			return nil, err
		}
		messages = append(messages, *msg)

		// No tool calls means the model produced its final answer.
		if len(msg.ToolCalls) == 0 {
			return &FindResult{
				Requirement: requirement,
				Skill:       strings.TrimSpace(msg.Content),
				Sources:     sources,
				Steps:       step,
			}, nil
		}

		// Execute every requested tool call and feed each result back keyed by
		// its call id, as the API requires.
		for _, tc := range msg.ToolCalls {
			result, fetched := f.dispatch(ctx, tc)
			if fetched != "" {
				sources = append(sources, fetched)
			}
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    result,
			})
		}
	}

	return nil, fmt.Errorf("skill finder did not converge within %d steps", maxAgentSteps)
}

// dispatch runs one tool call and returns the text to feed back to the model,
// plus the URL fetched (for the sources list) when the call was a fetch. Tool
// errors are returned as text, not Go errors: the model should see the failure
// and adapt (try another URL) rather than have the whole loop abort.
func (f *SkillFinder) dispatch(ctx context.Context, tc toolCall) (result, fetchedURL string) {
	switch tc.Function.Name {
	case "search_github":
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return "error: bad arguments: " + err.Error(), ""
		}
		if args.Limit <= 0 {
			args.Limit = 6
		}
		hits, err := f.FC.Search(ctx, SearchRequest{
			Query:          args.Query,
			Limit:          args.Limit,
			Sources:        []string{"web"},
			IncludeDomains: []string{"github.com"},
		})
		if err != nil {
			return "error: " + err.Error(), ""
		}
		return renderHitsForAgent(hits), ""

	case "fetch_url":
		var args struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return "error: bad arguments: " + err.Error(), ""
		}
		res, err := f.FC.Scrape(ctx, ScrapeRequest{
			URL:             args.URL,
			Formats:         []string{"markdown"},
			OnlyMainContent: true,
		})
		if err != nil {
			return "error: " + err.Error(), ""
		}
		return clip(res.Markdown, agentFetchBudget), args.URL

	default:
		return "error: unknown tool " + tc.Function.Name, ""
	}
}

// agentFetchBudget caps a single fetched page fed back to the model. A SKILL.md
// is typically a few KB; this keeps a fetched list page from flooding the
// context while leaving ample room for a real skill.
const agentFetchBudget = 24000

func renderHitsForAgent(hits []SearchResult) string {
	if len(hits) == 0 {
		return "no results"
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n", i+1, h.Title, h.URL, h.Description)
	}
	return b.String()
}
