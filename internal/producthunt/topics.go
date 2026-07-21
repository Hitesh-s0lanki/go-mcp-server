package producthunt

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTopicTools wires the topic (category) tools.
func registerTopicTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_list_topics",
		Description: "List or search Product Hunt topics (categories) with their follower and post counts. " +
			"Pass 'query' to search by name. Order by FOLLOWERS_COUNT (default) or NEWEST. Paginate with " +
			"'first' (default 20, max 20) and the returned 'after' cursor. Use a topic's slug to filter " +
			"producthunt_list_posts.",
	}, c.listTopics)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "producthunt_get_topic",
		Description: "Get one Product Hunt topic by 'id' or 'slug' (e.g. artificial-intelligence): description, follower and post counts, and URL.",
	}, c.getTopic)
}

type topic struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description,omitempty"`
	FollowersCount int    `json:"followersCount"`
	PostsCount     int    `json:"postsCount"`
	URL            string `json:"url,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
}

const topicFields = `
  id
  name
  slug
  description
  followersCount
  postsCount
  url
  createdAt
`

// --- producthunt_list_topics ---

type listTopicsInput struct {
	Query string `json:"query,omitempty" jsonschema:"search topics by name"`
	Order string `json:"order,omitempty" jsonschema:"FOLLOWERS_COUNT (default) or NEWEST"`
	First int    `json:"first,omitempty" jsonschema:"page size, default 20, max 20"`
	After string `json:"after,omitempty" jsonschema:"pagination cursor from a previous call"`
}

type listTopicsOutput struct {
	Count       int     `json:"count"`
	Topics      []topic `json:"topics"`
	EndCursor   string  `json:"end_cursor,omitempty"`
	HasNextPage bool    `json:"has_next_page"`
}

func (c *Client) listTopics(ctx context.Context, _ *mcp.CallToolRequest, in listTopicsInput) (*mcp.CallToolResult, listTopicsOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[listTopicsOutput]("%v", err)
	}

	query := `query ListTopics($first: Int, $after: String, $query: String, $order: TopicsOrder) {
  topics(first: $first, after: $after, query: $query, order: $order) {
    pageInfo { endCursor hasNextPage }
    edges { node {` + topicFields + `} }
  }
}`
	vars := map[string]any{"first": clampFirst(in.First, 20, 20)}
	setIf(vars, "query", in.Query)
	setIf(vars, "after", in.After)
	if order := strings.ToUpper(strings.TrimSpace(in.Order)); order != "" {
		vars["order"] = order
	}

	var data struct {
		Topics struct {
			PageInfo pageInfo `json:"pageInfo"`
			Edges    []struct {
				Node topic `json:"node"`
			} `json:"edges"`
		} `json:"topics"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[listTopicsOutput]("list topics: %v", err)
	}

	out := listTopicsOutput{
		Topics:      make([]topic, 0, len(data.Topics.Edges)),
		EndCursor:   data.Topics.PageInfo.EndCursor,
		HasNextPage: data.Topics.PageInfo.HasNextPage,
	}
	for _, e := range data.Topics.Edges {
		out.Topics = append(out.Topics, e.Node)
	}
	out.Count = len(out.Topics)
	return jsonResult(out)
}

// --- producthunt_get_topic ---

type topicRefInput struct {
	ID   string `json:"id,omitempty" jsonschema:"the topic ID; provide this or slug"`
	Slug string `json:"slug,omitempty" jsonschema:"the topic slug, e.g. artificial-intelligence; provide this or id"`
}

func (c *Client) getTopic(ctx context.Context, _ *mcp.CallToolRequest, in topicRefInput) (*mcp.CallToolResult, topic, error) {
	if err := c.ready(); err != nil {
		return toolErr[topic]("%v", err)
	}
	if in.ID == "" && in.Slug == "" {
		return toolErr[topic]("provide id or slug")
	}

	query := `query GetTopic($id: ID, $slug: String) {
  topic(id: $id, slug: $slug) {` + topicFields + `}
}`
	vars := map[string]any{}
	setIf(vars, "id", in.ID)
	setIf(vars, "slug", in.Slug)

	var data struct {
		Topic *topic `json:"topic"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[topic]("get topic: %v", err)
	}
	if data.Topic == nil {
		return toolErr[topic]("no topic found for the given id/slug")
	}
	return jsonResult(*data.Topic)
}
