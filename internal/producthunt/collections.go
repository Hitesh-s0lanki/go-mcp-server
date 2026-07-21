package producthunt

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerCollectionTools wires the collection (curated list) tools.
func registerCollectionTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_list_collections",
		Description: "List Product Hunt collections (curated lists of posts). Filter by 'featured', a 'user_id' " +
			"(the curator) or a 'post_id' (collections containing that post). Order by FOLLOWERS_COUNT (default), " +
			"NEWEST or FEATURED_AT. Paginate with 'first' (default 10, max 20) and the returned 'after' cursor.",
	}, c.listCollections)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "producthunt_get_collection",
		Description: "Get one Product Hunt collection by 'id' or 'slug', including the first posts it contains.",
	}, c.getCollection)
}

type collection struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Tagline        string `json:"tagline,omitempty"`
	Description    string `json:"description,omitempty"`
	FollowersCount int    `json:"followersCount"`
	URL            string `json:"url,omitempty"`
	CoverImage     string `json:"coverImage,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	User           *maker `json:"user,omitempty"`
}

const collectionSummaryFields = `
  id
  name
  tagline
  followersCount
  url
  createdAt
  user { id name username url }
`

// --- producthunt_list_collections ---

type listCollectionsInput struct {
	Featured *bool  `json:"featured,omitempty" jsonschema:"only featured collections (true) or only non-featured (false); omit for all"`
	UserID   string `json:"user_id,omitempty" jsonschema:"only collections curated by this user ID"`
	PostID   string `json:"post_id,omitempty" jsonschema:"only collections that contain this post ID"`
	Order    string `json:"order,omitempty" jsonschema:"FOLLOWERS_COUNT (default), NEWEST or FEATURED_AT"`
	First    int    `json:"first,omitempty" jsonschema:"page size, default 10, max 20"`
	After    string `json:"after,omitempty" jsonschema:"pagination cursor from a previous call"`
}

type listCollectionsOutput struct {
	Count       int          `json:"count"`
	Collections []collection `json:"collections"`
	EndCursor   string       `json:"end_cursor,omitempty"`
	HasNextPage bool         `json:"has_next_page"`
}

func (c *Client) listCollections(ctx context.Context, _ *mcp.CallToolRequest, in listCollectionsInput) (*mcp.CallToolResult, listCollectionsOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[listCollectionsOutput]("%v", err)
	}

	query := `query ListCollections($first: Int, $after: String, $order: CollectionsOrder, $featured: Boolean, $userId: ID, $postId: ID) {
  collections(first: $first, after: $after, order: $order, featured: $featured, userId: $userId, postId: $postId) {
    pageInfo { endCursor hasNextPage }
    edges { node {` + collectionSummaryFields + `} }
  }
}`
	vars := map[string]any{"first": clampFirst(in.First, 10, 20)}
	if in.Featured != nil {
		vars["featured"] = *in.Featured
	}
	setIf(vars, "userId", in.UserID)
	setIf(vars, "postId", in.PostID)
	setIf(vars, "after", in.After)
	if order := strings.ToUpper(strings.TrimSpace(in.Order)); order != "" {
		vars["order"] = order
	}

	var data struct {
		Collections struct {
			PageInfo pageInfo `json:"pageInfo"`
			Edges    []struct {
				Node collection `json:"node"`
			} `json:"edges"`
		} `json:"collections"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[listCollectionsOutput]("list collections: %v", err)
	}

	out := listCollectionsOutput{
		Collections: make([]collection, 0, len(data.Collections.Edges)),
		EndCursor:   data.Collections.PageInfo.EndCursor,
		HasNextPage: data.Collections.PageInfo.HasNextPage,
	}
	for _, e := range data.Collections.Edges {
		out.Collections = append(out.Collections, e.Node)
	}
	out.Count = len(out.Collections)
	return jsonResult(out)
}

// --- producthunt_get_collection ---

type collectionRefInput struct {
	ID    string `json:"id,omitempty" jsonschema:"the collection ID; provide this or slug"`
	Slug  string `json:"slug,omitempty" jsonschema:"the collection slug from its URL; provide this or id"`
	First int    `json:"first,omitempty" jsonschema:"how many contained posts to include, default 10, max 20"`
}

type collectionDetail struct {
	collection
	Posts []post `json:"posts"`
}

func (c *Client) getCollection(ctx context.Context, _ *mcp.CallToolRequest, in collectionRefInput) (*mcp.CallToolResult, collectionDetail, error) {
	if err := c.ready(); err != nil {
		return toolErr[collectionDetail]("%v", err)
	}
	if in.ID == "" && in.Slug == "" {
		return toolErr[collectionDetail]("provide id or slug")
	}

	query := `query GetCollection($id: ID, $slug: String, $first: Int) {
  collection(id: $id, slug: $slug) {
    id
    name
    tagline
    description
    followersCount
    url
    coverImage
    createdAt
    user { id name username url }
    posts(first: $first) { edges { node {` + postSummaryFields + `} } }
  }
}`
	vars := map[string]any{"first": clampFirst(in.First, 10, 20)}
	setIf(vars, "id", in.ID)
	setIf(vars, "slug", in.Slug)

	var data struct {
		Collection *struct {
			collection
			Posts struct {
				Edges []struct {
					Node post `json:"node"`
				} `json:"edges"`
			} `json:"posts"`
		} `json:"collection"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[collectionDetail]("get collection: %v", err)
	}
	if data.Collection == nil {
		return toolErr[collectionDetail]("no collection found for the given id/slug")
	}

	out := collectionDetail{collection: data.Collection.collection, Posts: make([]post, 0, len(data.Collection.Posts.Edges))}
	for _, e := range data.Collection.Posts.Edges {
		out.Posts = append(out.Posts, e.Node)
	}
	return jsonResult(out)
}
