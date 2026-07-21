package producthunt

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerUserTools wires the user-profile tools.
func registerUserTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_get_user",
		Description: "Get a Product Hunt user's profile by 'id' or 'username', with a sample of the posts they " +
			"have made. Useful to look up a maker or hunter surfaced by the post tools.",
	}, c.getUser)

	mcp.AddTool(s, &mcp.Tool{
		Name: "producthunt_viewer",
		Description: "Return the profile of the user the current token belongs to (the GraphQL 'viewer'). " +
			"Requires a user-scoped token; with a client-credentials token there is no viewer and this reports so.",
	}, c.viewer)
}

type user struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Username        string `json:"username"`
	Headline        string `json:"headline,omitempty"`
	URL             string `json:"url,omitempty"`
	TwitterUsername string `json:"twitterUsername,omitempty"`
	WebsiteURL      string `json:"websiteUrl,omitempty"`
	ProfileImage    string `json:"profileImage,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
}

const userFields = `
  id
  name
  username
  headline
  url
  twitterUsername
  websiteUrl
  profileImage
  createdAt
`

type userDetail struct {
	user
	MadePosts []post `json:"made_posts,omitempty"`
}

// --- producthunt_get_user ---

type userRefInput struct {
	ID       string `json:"id,omitempty" jsonschema:"the user ID; provide this or username"`
	Username string `json:"username,omitempty" jsonschema:"the user's username; provide this or id"`
	Posts    int    `json:"posts,omitempty" jsonschema:"how many of the user's made posts to include, default 5, max 20"`
}

func (c *Client) getUser(ctx context.Context, _ *mcp.CallToolRequest, in userRefInput) (*mcp.CallToolResult, userDetail, error) {
	if err := c.ready(); err != nil {
		return toolErr[userDetail]("%v", err)
	}
	if in.ID == "" && in.Username == "" {
		return toolErr[userDetail]("provide id or username")
	}

	query := `query GetUser($id: ID, $username: String, $posts: Int) {
  user(id: $id, username: $username) {` + userFields + `
    madePosts(first: $posts) { edges { node {` + postSummaryFields + `} } }
  }
}`
	vars := map[string]any{"posts": clampFirst(in.Posts, 5, 20)}
	setIf(vars, "id", in.ID)
	setIf(vars, "username", in.Username)

	var data struct {
		User *struct {
			user
			MadePosts struct {
				Edges []struct {
					Node post `json:"node"`
				} `json:"edges"`
			} `json:"madePosts"`
		} `json:"user"`
	}
	if err := c.graphql(ctx, query, vars, &data); err != nil {
		return toolErr[userDetail]("get user: %v", err)
	}
	if data.User == nil {
		return toolErr[userDetail]("no user found for the given id/username")
	}

	out := userDetail{user: data.User.user, MadePosts: make([]post, 0, len(data.User.MadePosts.Edges))}
	for _, e := range data.User.MadePosts.Edges {
		out.MadePosts = append(out.MadePosts, e.Node)
	}
	return jsonResult(out)
}

// --- producthunt_viewer ---

func (c *Client) viewer(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, user, error) {
	if err := c.ready(); err != nil {
		return toolErr[user]("%v", err)
	}

	query := `query Viewer {
  viewer {
    user {` + userFields + `}
  }
}`
	var data struct {
		Viewer *struct {
			User *user `json:"user"`
		} `json:"viewer"`
	}
	if err := c.graphql(ctx, query, nil, &data); err != nil {
		return toolErr[user]("get viewer: %v", err)
	}
	if data.Viewer == nil || data.Viewer.User == nil {
		return toolErr[user]("no viewer for this token (a user-scoped token is required; client-credentials tokens have no viewer)")
	}
	return jsonResult(*data.Viewer.User)
}
