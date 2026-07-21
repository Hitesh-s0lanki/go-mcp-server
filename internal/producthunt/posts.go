package producthunt

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPostTools wires the post (product launch) tools.
func registerPostTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_list_posts",
		Description: "List Product Hunt posts (product launches) with their votes, comments and topics. " +
			"Filter by 'featured', a 'topic' slug (e.g. artificial-intelligence), or a posted date window " +
			"(posted_after/posted_before, ISO-8601). Order by RANKING (default, the day's leaderboard), NEWEST, " +
			"VOTES or FEATURED_AT. Paginate with 'first' (default 10, max 20) and the returned 'after' cursor.",
	}, c.listPosts)

	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_get_post",
		Description: "Get one Product Hunt post in full by 'id' or 'slug' (the slug from its URL, " +
			"e.g. producthunt.com/posts/<slug>): description, website, media, makers, hunter and topics.",
	}, c.getPost)

	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_get_post_comments",
		Description: "List the comments on a Product Hunt post identified by 'id' or 'slug'. Order by " +
			"VOTES (default) or NEWEST. Paginate with 'first' (default 20, max 20) and the returned 'after' cursor.",
	}, c.getPostComments)
}

// --- shared GraphQL field selections ---

const postSummaryFields = `
  id
  name
  tagline
  slug
  url
  votesCount
  commentsCount
  reviewsRating
  featuredAt
  createdAt
  thumbnail { url }
  topics(first: 5) { edges { node { name slug } } }
`

const postDetailFields = `
  id
  name
  tagline
  description
  slug
  url
  website
  votesCount
  commentsCount
  reviewsCount
  reviewsRating
  featuredAt
  createdAt
  thumbnail { url }
  media { url type videoUrl }
  topics(first: 10) { edges { node { id name slug } } }
  makers { id name username url }
  user { id name username url }
`

// --- Go models mirroring the GraphQL selections above ---

type namedTopic struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type topicConnection struct {
	Edges []struct {
		Node namedTopic `json:"node"`
	} `json:"edges"`
}

type media struct {
	URL      string `json:"url"`
	Type     string `json:"type"`
	VideoURL string `json:"videoUrl,omitempty"`
}

type maker struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	URL      string `json:"url,omitempty"`
}

// post models a Product Hunt post as returned by the GraphQL API. Fields not
// selected by a given query stay at their zero value.
type post struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Tagline       string  `json:"tagline"`
	Description   string  `json:"description,omitempty"`
	Slug          string  `json:"slug"`
	URL           string  `json:"url"`
	Website       string  `json:"website,omitempty"`
	VotesCount    int     `json:"votesCount"`
	CommentsCount int     `json:"commentsCount"`
	ReviewsCount  int     `json:"reviewsCount,omitempty"`
	ReviewsRating float64 `json:"reviewsRating,omitempty"`
	FeaturedAt    string  `json:"featuredAt,omitempty"`
	CreatedAt     string  `json:"createdAt,omitempty"`
	Thumbnail     *struct {
		URL string `json:"url"`
	} `json:"thumbnail,omitempty"`
	Media  []media         `json:"media,omitempty"`
	Topics topicConnection `json:"topics"`
	Makers []maker         `json:"makers,omitempty"`
	User   *maker          `json:"user,omitempty"`
}

// --- producthunt_list_posts ---

type listPostsInput struct {
	Order        string `json:"order,omitempty" jsonschema:"RANKING (default), NEWEST, VOTES or FEATURED_AT"`
	Featured     *bool  `json:"featured,omitempty" jsonschema:"only posts that were featured (true) or only non-featured (false); omit for all"`
	Topic        string `json:"topic,omitempty" jsonschema:"filter to a topic slug, e.g. artificial-intelligence"`
	PostedAfter  string `json:"posted_after,omitempty" jsonschema:"only posts created after this ISO-8601 timestamp, e.g. 2026-07-01T00:00:00Z"`
	PostedBefore string `json:"posted_before,omitempty" jsonschema:"only posts created before this ISO-8601 timestamp"`
	First        int    `json:"first,omitempty" jsonschema:"page size, default 10, max 20"`
	After        string `json:"after,omitempty" jsonschema:"pagination cursor returned as end_cursor by a previous call"`
}

type pageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}

type listPostsOutput struct {
	Count       int    `json:"count"`
	Posts       []post `json:"posts"`
	EndCursor   string `json:"end_cursor,omitempty"`
	HasNextPage bool   `json:"has_next_page"`
}

func (c *Client) listPosts(ctx context.Context, _ *mcp.CallToolRequest, in listPostsInput) (*mcp.CallToolResult, listPostsOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[listPostsOutput]("%v", err)
	}

	query := `query ListPosts($first: Int, $after: String, $order: PostsOrder, $featured: Boolean, $topic: String, $postedAfter: DateTime, $postedBefore: DateTime) {
  posts(first: $first, after: $after, order: $order, featured: $featured, topic: $topic, postedAfter: $postedAfter, postedBefore: $postedBefore) {
    pageInfo { endCursor hasNextPage }
    edges { node {` + postSummaryFields + `} }
  }
}`

	vars := map[string]any{"first": clampFirst(in.First, 10, 20)}
	if order := strings.ToUpper(strings.TrimSpace(in.Order)); order != "" {
		vars["order"] = order
	}
	if in.Featured != nil {
		vars["featured"] = *in.Featured
	}
	setIf(vars, "topic", in.Topic)
	setIf(vars, "after", in.After)
	setIf(vars, "postedAfter", in.PostedAfter)
	setIf(vars, "postedBefore", in.PostedBefore)

	var data struct {
		Posts struct {
			PageInfo pageInfo `json:"pageInfo"`
			Edges    []struct {
				Node post `json:"node"`
			} `json:"edges"`
		} `json:"posts"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[listPostsOutput]("list posts: %v", err)
	}

	out := listPostsOutput{
		Posts:       make([]post, 0, len(data.Posts.Edges)),
		EndCursor:   data.Posts.PageInfo.EndCursor,
		HasNextPage: data.Posts.PageInfo.HasNextPage,
	}
	for _, e := range data.Posts.Edges {
		out.Posts = append(out.Posts, e.Node)
	}
	out.Count = len(out.Posts)
	return jsonResult(out)
}

// --- producthunt_get_post ---

type postRefInput struct {
	ID   string `json:"id,omitempty" jsonschema:"the post ID; provide this or slug"`
	Slug string `json:"slug,omitempty" jsonschema:"the post slug from its URL; provide this or id"`
}

func (c *Client) getPost(ctx context.Context, _ *mcp.CallToolRequest, in postRefInput) (*mcp.CallToolResult, post, error) {
	if err := c.ready(); err != nil {
		return toolErr[post]("%v", err)
	}
	if in.ID == "" && in.Slug == "" {
		return toolErr[post]("provide id or slug")
	}

	query := `query GetPost($id: ID, $slug: String) {
  post(id: $id, slug: $slug) {` + postDetailFields + `}
}`
	vars := map[string]any{}
	setIf(vars, "id", in.ID)
	setIf(vars, "slug", in.Slug)

	var data struct {
		Post *post `json:"post"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[post]("get post: %v", err)
	}
	if data.Post == nil {
		return toolErr[post]("no post found for the given id/slug")
	}
	return jsonResult(*data.Post)
}

// --- producthunt_get_post_comments ---

type postCommentsInput struct {
	ID    string `json:"id,omitempty" jsonschema:"the post ID; provide this or slug"`
	Slug  string `json:"slug,omitempty" jsonschema:"the post slug from its URL; provide this or id"`
	Order string `json:"order,omitempty" jsonschema:"VOTES (default) or NEWEST"`
	First int    `json:"first,omitempty" jsonschema:"page size, default 20, max 20"`
	After string `json:"after,omitempty" jsonschema:"pagination cursor from a previous call"`
}

type comment struct {
	ID         string `json:"id"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
	VotesCount int    `json:"votesCount"`
	URL        string `json:"url,omitempty"`
	User       *maker `json:"user,omitempty"`
}

type postCommentsOutput struct {
	PostID      string    `json:"post_id"`
	PostName    string    `json:"post_name"`
	Count       int       `json:"count"`
	Comments    []comment `json:"comments"`
	EndCursor   string    `json:"end_cursor,omitempty"`
	HasNextPage bool      `json:"has_next_page"`
}

func (c *Client) getPostComments(ctx context.Context, _ *mcp.CallToolRequest, in postCommentsInput) (*mcp.CallToolResult, postCommentsOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[postCommentsOutput]("%v", err)
	}
	if in.ID == "" && in.Slug == "" {
		return toolErr[postCommentsOutput]("provide id or slug")
	}

	query := `query PostComments($id: ID, $slug: String, $first: Int, $after: String, $order: CommentsOrder) {
  post(id: $id, slug: $slug) {
    id
    name
    comments(first: $first, after: $after, order: $order) {
      pageInfo { endCursor hasNextPage }
      edges { node { id body createdAt votesCount url user { id name username url } } }
    }
  }
}`
	vars := map[string]any{"first": clampFirst(in.First, 20, 20)}
	setIf(vars, "id", in.ID)
	setIf(vars, "slug", in.Slug)
	setIf(vars, "after", in.After)
	if order := strings.ToUpper(strings.TrimSpace(in.Order)); order != "" {
		vars["order"] = order
	}

	var data struct {
		Post *struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Comments struct {
				PageInfo pageInfo `json:"pageInfo"`
				Edges    []struct {
					Node comment `json:"node"`
				} `json:"edges"`
			} `json:"comments"`
		} `json:"post"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[postCommentsOutput]("get post comments: %v", err)
	}
	if data.Post == nil {
		return toolErr[postCommentsOutput]("no post found for the given id/slug")
	}

	out := postCommentsOutput{
		PostID:      data.Post.ID,
		PostName:    data.Post.Name,
		Comments:    make([]comment, 0, len(data.Post.Comments.Edges)),
		EndCursor:   data.Post.Comments.PageInfo.EndCursor,
		HasNextPage: data.Post.Comments.PageInfo.HasNextPage,
	}
	for _, e := range data.Post.Comments.Edges {
		out.Comments = append(out.Comments, e.Node)
	}
	out.Count = len(out.Comments)
	return jsonResult(out)
}
