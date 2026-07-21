// Package producthunt hosts the /producthunt/mcp namespace: read tools over the
// Product Hunt API v2 — posts, topics, collections, users and comments — backed
// by the official GraphQL endpoint (https://api.producthunt.com/v2/api/graphql).
//
// The API is a single GraphQL endpoint authenticated with a bearer token. Two
// credential styles are supported, resolved in order:
//
//  1. PRODUCTHUNT_TOKEN — a non-expiring developer token from the Product Hunt
//     API dashboard (https://www.producthunt.com/v2/oauth/applications). Simplest;
//     no network round-trip. PRODUCTHUNT_DEVELOPER_TOKEN / PRODUCTHUNT_ACCESS_TOKEN
//     are accepted as aliases.
//  2. PRODUCTHUNT_CLIENT_ID + PRODUCTHUNT_CLIENT_SECRET — the OAuth2
//     client-credentials ("client-only") flow. The namespace fetches a
//     public-scope token lazily on first use and caches it.
//
// Like the gsc and skills namespaces (and unlike memory), missing credentials
// are not a mount failure: the namespace mounts regardless and every tool
// reports the configuration problem. producthunt_capabilities surfaces the auth
// status. Only public read access is exposed — no mutations.
package producthunt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// graphqlEndpoint is the single Product Hunt v2 GraphQL endpoint.
	graphqlEndpoint = "https://api.producthunt.com/v2/api/graphql"
	// oauthTokenURL is the OAuth2 token endpoint used by the client-credentials flow.
	//nolint:gosec // G101 false positive: this is an endpoint URL, not a credential
	oauthTokenURL = "https://api.producthunt.com/v2/oauth/token"
)

// Client wraps the Product Hunt GraphQL endpoint plus the namespace's auth
// configuration. It is built once at mount time. When no credentials are
// configured the namespace still mounts (so the rest of the server boots) but
// err is set and every tool reports it rather than panicking.
type Client struct {
	httpClient *http.Client
	log        *slog.Logger

	// endpoint is the GraphQL URL. Defaults to graphqlEndpoint; overridable in
	// tests to point at a stub server.
	endpoint string

	// token is a static developer/access token (PRODUCTHUNT_TOKEN). When set it
	// is used directly with no token-fetch round-trip.
	token string

	// clientID/clientSecret drive the OAuth2 client-credentials flow, used only
	// when no static token is configured.
	clientID     string
	clientSecret string

	// authSource is a human-readable note about how credentials resolved,
	// surfaced by producthunt_capabilities. Never contains secrets.
	authSource string

	// err is the configuration error, if any (no auth configured).
	err error

	// mu guards the client-credentials token cache below.
	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// NewClient reads the environment and builds the client. It never fails: a
// missing credential is captured on err so the namespace can still mount.
func NewClient(log *slog.Logger) *Client {
	c := &Client{
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		log:          log,
		endpoint:     graphqlEndpoint,
		token:        firstNonEmpty(os.Getenv("PRODUCTHUNT_TOKEN"), os.Getenv("PRODUCTHUNT_DEVELOPER_TOKEN"), os.Getenv("PRODUCTHUNT_ACCESS_TOKEN")),
		clientID:     os.Getenv("PRODUCTHUNT_CLIENT_ID"),
		clientSecret: os.Getenv("PRODUCTHUNT_CLIENT_SECRET"),
	}
	switch {
	case c.token != "":
		c.authSource = "developer/access token (PRODUCTHUNT_TOKEN)"
	case c.clientID != "" && c.clientSecret != "":
		c.authSource = "OAuth client credentials (PRODUCTHUNT_CLIENT_ID/PRODUCTHUNT_CLIENT_SECRET)"
	default:
		c.err = fmt.Errorf("no Product Hunt credentials configured: set PRODUCTHUNT_TOKEN (a developer token from " +
			"the API dashboard), or PRODUCTHUNT_CLIENT_ID and PRODUCTHUNT_CLIENT_SECRET for the client-credentials flow")
		if log != nil {
			log.Warn("producthunt namespace mounted without credentials; tools will report the configuration error")
		}
	}
	return c
}

// ready returns the configuration error if the client has no usable credentials.
func (c *Client) ready() error {
	if c == nil || (c.token == "" && (c.clientID == "" || c.clientSecret == "")) {
		if c != nil && c.err != nil {
			return c.err
		}
		return fmt.Errorf("no Product Hunt credentials configured (set PRODUCTHUNT_TOKEN or PRODUCTHUNT_CLIENT_ID/PRODUCTHUNT_CLIENT_SECRET)")
	}
	return nil
}

// ensureToken returns a usable bearer token, fetching one via the
// client-credentials flow (and caching it) when only client credentials are set.
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	if c.token != "" {
		return c.token, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	payload, err := json.Marshal(map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"grant_type":    "client_credentials",
	})
	if err != nil {
		return "", fmt.Errorf("encode token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request client-credentials token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("client-credentials token request failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
		Scope       string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("client-credentials token response contained no access_token")
	}

	// Product Hunt client-credentials tokens are long-lived; honour expires_in
	// when present, otherwise re-fetch hourly. Keep a small safety margin.
	ttl := time.Hour
	if tr.ExpiresIn > 0 {
		ttl = time.Duration(tr.ExpiresIn) * time.Second
	}
	c.cachedToken = tr.AccessToken
	c.tokenExpiry = time.Now().Add(ttl - time.Minute)
	return c.cachedToken, nil
}

// graphqlRequest is the JSON body posted to the GraphQL endpoint.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphql executes a GraphQL query against the Product Hunt endpoint and
// unmarshals the "data" object into out. GraphQL-level errors (and HTTP faults)
// are returned as a Go error with a clean message.
func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(graphqlRequest{Query: query, Variables: vars})
	if err != nil {
		return fmt.Errorf("encode GraphQL request: %w", err)
	}
	endpoint := c.endpoint
	if endpoint == "" {
		endpoint = graphqlEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build GraphQL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call Product Hunt API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read Product Hunt response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		reset := resp.Header.Get("X-Rate-Limit-Reset")
		if reset != "" {
			return fmt.Errorf("rate limit exceeded; retry after %s seconds (X-Rate-Limit-Reset)", reset)
		}
		return fmt.Errorf("rate limit exceeded; retry later")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("credentials rejected by Product Hunt (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode Product Hunt response (HTTP %s): %w", resp.Status, err)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("GraphQL error: %s", strings.Join(msgs, "; "))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("unexpected Product Hunt API response (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode GraphQL data: %w", err)
	}
	return nil
}

// --- small helpers shared across tool files ---

// jsonResult builds a tool result whose text content is the pretty-printed JSON
// of out, alongside the same value as structured output.
func jsonResult[T any](out T) (*mcp.CallToolResult, T, error) {
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return toolErr[T]("encode result: %v", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, out, nil
}

// toolErr builds a non-protocol tool error (IsError result) with a formatted
// message, so the model sees a clean explanation instead of a transport fault.
func toolErr[T any](format string, args ...any) (*mcp.CallToolResult, T, error) {
	var zero T
	msg := fmt.Sprintf(format, args...)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, zero, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// clampFirst normalizes a page size: <=0 falls back to def, and values above max
// are capped (Product Hunt rejects large page sizes).
func clampFirst(n, def, maximum int) int {
	if n <= 0 {
		return def
	}
	if n > maximum {
		return maximum
	}
	return n
}

// setIf adds key=val to vars only when val is non-empty. Used to build GraphQL
// variable maps so omitted optional arguments stay null rather than "".
func setIf(vars map[string]any, key, val string) {
	if val != "" {
		vars[key] = val
	}
}
