package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// snippetLen bounds the content returned by search. Search returns snippets and
// IDs; memory_get returns full content. Otherwise one broad search dumps every
// matched document into the context window -- the exact cost RAG exists to avoid.
const snippetLen = 320

// defaultMinScore is the cosine-similarity floor for search.
//
// 0.35 is not arbitrary: with text-embedding-3-small, unrelated text pairs land
// around 0.10-0.30, so this sits just above that noise floor. It is NOT
// transferable -- on text-embedding-ada-002 unrelated pairs score ~0.70-0.80 and
// this value would match everything. Recalibrate if EmbedModel changes.
const defaultMinScore = 0.35

type searchInput struct {
	Query    string   `json:"query" jsonschema:"what to search for, in natural language"`
	Limit    int      `json:"limit,omitempty" jsonschema:"max results to return (default 5)"`
	MinScore float64  `json:"min_score,omitempty" jsonschema:"cosine similarity floor 0-1 (default 0.35); raise for stricter matches"`
	Tags     []string `json:"tags,omitempty" jsonschema:"only search memories carrying any of these tags"`
}

type searchOutput struct {
	Hits  []Hit  `json:"hits"`
	Note  string `json:"note,omitempty"`
	Query string `json:"query"`
}

type writeInput struct {
	Content  string            `json:"content" jsonschema:"the memory to store"`
	Tags     []string          `json:"tags,omitempty" jsonschema:"labels for filtering later"`
	Metadata map[string]string `json:"metadata,omitempty" jsonschema:"arbitrary key-value context"`
}

type idInput struct {
	ID string `json:"id" jsonschema:"the memory id, as returned by memory_search"`
}

type updateInput struct {
	ID      string `json:"id" jsonschema:"the memory id to update"`
	Content string `json:"content" jsonschema:"replacement content; the memory is re-embedded"`
}

type listInput struct {
	Tags  []string `json:"tags,omitempty" jsonschema:"only list memories carrying any of these tags"`
	Limit int      `json:"limit,omitempty" jsonschema:"max results (default 20)"`
}

// registerTools wires the memory tool surface onto s.
func registerTools(srv *mcp.Server, store *Store) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "memory_search",
		Description: "Search your stored memories by meaning and keyword. " +
			"Returns ranked snippets with a similarity score and an id; " +
			"call memory_get with that id for the full content. " +
			"An empty result means nothing relevant is stored -- treat that as " +
			"'no memory of this' rather than retrying.",
	}, toolSearch(store))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "memory_write",
		Description: "Store a new memory. Returns the created memory's id.",
	}, toolWrite(store))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "memory_get",
		Description: "Fetch one memory in full by id.",
	}, toolGet(store))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "memory_update",
		Description: "Replace a memory's content by id. The memory is re-embedded.",
	}, toolUpdate(store))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "memory_delete",
		Description: "Permanently delete a memory by id.",
	}, toolDelete(store))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "memory_list",
		Description: "List recent memories newest-first, optionally filtered by tag. " +
			"Use this to browse when you have no search query.",
	}, toolList(store))
}

func toolSearch(store *Store) mcp.ToolHandlerFor[searchInput, searchOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		email, err := callerEmail(req)
		if err != nil {
			return errResult(err), searchOutput{}, nil
		}
		if strings.TrimSpace(in.Query) == "" {
			return errResult(errors.New("query is required")), searchOutput{}, nil
		}

		minScore := float32(defaultMinScore)
		if in.MinScore > 0 {
			minScore = float32(in.MinScore)
		}

		hits, err := store.Search(ctx, email, SearchParams{
			Query:    in.Query,
			Limit:    in.Limit,
			MinScore: minScore,
			Tags:     in.Tags,
		})
		if err != nil {
			return errResult(err), searchOutput{}, nil
		}

		out := searchOutput{Hits: truncate(hits), Query: in.Query}
		if len(hits) == 0 {
			out.Note = fmt.Sprintf(
				"No memories matched above a similarity of %.2f. Nothing relevant is stored.",
				minScore)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: renderHits(out)}},
		}, out, nil
	}
}

func toolWrite(store *Store) mcp.ToolHandlerFor[writeInput, Memory] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in writeInput) (*mcp.CallToolResult, Memory, error) {
		email, err := callerEmail(req)
		if err != nil {
			return errResult(err), Memory{}, nil
		}
		if strings.TrimSpace(in.Content) == "" {
			return errResult(errors.New("content is required")), Memory{}, nil
		}

		m, err := store.Write(ctx, email, in.Content, in.Tags, in.Metadata)
		if err != nil {
			return errResult(err), Memory{}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "stored memory " + m.ID}},
		}, *m, nil
	}
}

func toolGet(store *Store) mcp.ToolHandlerFor[idInput, Memory] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, Memory, error) {
		email, err := callerEmail(req)
		if err != nil {
			return errResult(err), Memory{}, nil
		}
		m, err := store.Get(ctx, email, in.ID)
		if err != nil {
			return errResult(err), Memory{}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: m.Content}},
		}, *m, nil
	}
}

func toolUpdate(store *Store) mcp.ToolHandlerFor[updateInput, Memory] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, Memory, error) {
		email, err := callerEmail(req)
		if err != nil {
			return errResult(err), Memory{}, nil
		}
		if strings.TrimSpace(in.Content) == "" {
			return errResult(errors.New("content is required")), Memory{}, nil
		}
		m, err := store.Update(ctx, email, in.ID, in.Content)
		if err != nil {
			return errResult(err), Memory{}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "updated memory " + m.ID}},
		}, *m, nil
	}
}

type deleteOutput struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func toolDelete(store *Store) mcp.ToolHandlerFor[idInput, deleteOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, deleteOutput, error) {
		email, err := callerEmail(req)
		if err != nil {
			return errResult(err), deleteOutput{}, nil
		}
		if err := store.Delete(ctx, email, in.ID); err != nil {
			return errResult(err), deleteOutput{}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "deleted memory " + in.ID}},
		}, deleteOutput{ID: in.ID, Deleted: true}, nil
	}
}

type listOutput struct {
	Memories []Memory `json:"memories"`
}

func toolList(store *Store) mcp.ToolHandlerFor[listInput, listOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
		email, err := callerEmail(req)
		if err != nil {
			return errResult(err), listOutput{}, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		ms, err := store.List(ctx, email, in.Tags, limit)
		if err != nil {
			return errResult(err), listOutput{}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: renderList(ms)}},
		}, listOutput{Memories: ms}, nil
	}
}

// errResult reports a failure to the model rather than to the transport.
//
// Tool handlers return (result, out, nil) with IsError set instead of a Go
// error: a transport-level error is opaque to the model, whereas an error
// *result* lets it read what went wrong and adjust (supply an id, relax a
// filter). ErrNotFound is mapped verbatim so the model does not learn to
// distinguish "absent" from "someone else's".
func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

// truncate trims hit content to a snippet.
func truncate(hits []Hit) []Hit {
	for i := range hits {
		if len(hits[i].Content) > snippetLen {
			hits[i].Content = hits[i].Content[:snippetLen] + "..."
		}
	}
	return hits
}

// renderList produces the text view of a browse result. Like search, it renders
// content (not just a count) so an agent that reads only the text block still
// sees the memories -- important for the session-start profile recall.
func renderList(ms []Memory) string {
	if len(ms) == 0 {
		return "no memories"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d memories:\n", len(ms))
	for _, m := range ms {
		content := m.Content
		if len(content) > snippetLen {
			content = content[:snippetLen] + "..."
		}
		fmt.Fprintf(&b, "\n%s", m.ID)
		if len(m.Tags) > 0 {
			fmt.Fprintf(&b, " %v", m.Tags)
		}
		fmt.Fprintf(&b, "\n%s\n", content)
	}
	return b.String()
}

// renderHits produces the human/text view of a search result.
func renderHits(out searchOutput) string {
	if len(out.Hits) == 0 {
		return out.Note
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d memories for %q:\n", len(out.Hits), out.Query)
	for _, h := range out.Hits {
		fmt.Fprintf(&b, "\n[%.2f %s] %s\n%s\n", h.Score, h.Source, h.ID, h.Content)
	}
	return b.String()
}
