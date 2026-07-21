// Package gsc hosts the /gsc/mcp namespace: Google Search Console tools —
// search-analytics reporting, URL inspection, sitemap and property management —
// backed by the official Search Console API (searchconsole/v1).
//
// It is a Go port of the Python mcp-gsc server
// (https://github.com/AminForou/mcp-gsc), adapted for a headless,
// Streamable-HTTP MCP server: authentication is service-account / Application
// Default Credentials only (no interactive OAuth browser flow), and mutating
// operations are gated behind GSC_ALLOW_DESTRUCTIVE.
package gsc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/option"
	searchconsole "google.golang.org/api/searchconsole/v1"
)

// Client wraps the Search Console service plus the namespace's configuration.
// It is built once at mount time. When credentials cannot be resolved the
// namespace still mounts (so the rest of the server boots) but svc is nil and
// err explains why; every tool reports that error rather than panicking.
type Client struct {
	svc *searchconsole.Service

	// dataState is the default freshness passed to search-analytics queries:
	// "all" (default, matches the GSC dashboard, includes fresh data) or
	// "final" (finalized data only, ~2-3 day lag). Empty means unset.
	dataState string

	// allowDestructive gates the mutating tools (add/delete site, submit/delete
	// sitemap). Off by default so a misconfigured client cannot alter a property.
	allowDestructive bool

	// authSource is a human-readable note about how credentials were resolved,
	// surfaced by gsc_capabilities. Never contains secrets.
	authSource string

	// err is the configuration error, if any. Non-nil ⇒ svc is nil.
	err error
}

// NewClient resolves credentials and builds the Search Console service.
//
// Credential resolution, in order:
//  1. GSC_CREDENTIALS_PATH — path to a service-account JSON key file.
//  2. Application Default Credentials — GOOGLE_APPLICATION_CREDENTIALS, gcloud
//     user creds, or the GCE/Cloud Run metadata server.
//
// The service account (or ADC principal) must be added as a user on each
// Search Console property, or hold domain-wide delegation, to see any data.
//
// A resolution failure is captured on the returned Client rather than returned
// as an error, so the namespace mounts either way and reports the problem
// per-call and via gsc_capabilities.
func NewClient(ctx context.Context, log *slog.Logger) *Client {
	c := &Client{
		dataState:        firstNonEmpty(os.Getenv("GSC_DATA_STATE"), "all"),
		allowDestructive: isTruthy(os.Getenv("GSC_ALLOW_DESTRUCTIVE")),
	}

	// Full webmasters scope (not readonly) so the mutating tools work when
	// GSC_ALLOW_DESTRUCTIVE is set; read-only tools are unaffected by the extra
	// grant.
	const scope = searchconsole.WebmastersScope

	if path := os.Getenv("GSC_CREDENTIALS_PATH"); path != "" {
		// WithCredentialsFile is deprecated only against *untrusted* credential
		// config; here the path comes from the operator's own env var, which is
		// exactly the trusted case the deprecation note excludes.
		//nolint:staticcheck // operator-supplied credential path, not untrusted input
		svc, err := searchconsole.NewService(ctx, option.WithCredentialsFile(path), option.WithScopes(scope))
		if err != nil {
			c.err = fmt.Errorf("load service-account credentials from GSC_CREDENTIALS_PATH %q: %w", path, err)
			return c
		}
		c.svc = svc
		c.authSource = "service account (GSC_CREDENTIALS_PATH)"
		return c
	}

	// No explicit key file: fall back to Application Default Credentials.
	svc, err := searchconsole.NewService(ctx, option.WithScopes(scope))
	if err != nil {
		c.err = fmt.Errorf(
			"no GSC_CREDENTIALS_PATH set and Application Default Credentials unavailable: %w "+
				"(set GSC_CREDENTIALS_PATH to a service-account key, or configure ADC)", err)
		if log != nil {
			log.Warn("gsc namespace mounted without credentials; tools will report the configuration error", "err", err)
		}
		return c
	}
	c.svc = svc
	c.authSource = "Application Default Credentials"
	return c
}

// ready returns the configuration error if the client is not usable.
func (c *Client) ready() error {
	if c == nil || c.svc == nil {
		if c != nil && c.err != nil {
			return c.err
		}
		return fmt.Errorf("no Google Search Console credentials configured (set GSC_CREDENTIALS_PATH or configure ADC)")
	}
	return nil
}

// requireDestructive returns an error unless mutating operations are enabled.
func (c *Client) requireDestructive(op string) error {
	if !c.allowDestructive {
		return fmt.Errorf("%s is a mutating operation and is disabled; set GSC_ALLOW_DESTRUCTIVE=true to enable it", op)
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

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// splitLines splits a newline/comma-delimited list into trimmed, non-empty
// entries. Used by the batch URL tools, which accept URLs one per line.
func splitLines(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if t := strings.TrimSpace(f); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// round2 rounds a float to 2 decimals for stable, readable output.
func round2(f float64) float64 {
	return float64(int64(f*100+0.5*sign(f))) / 100
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// sortSliceStable is a generic stable sort taking a less(a, b) predicate.
func sortSliceStable[T any](s []T, less func(a, b T) bool) {
	sort.SliceStable(s, func(i, j int) bool { return less(s[i], s[j]) })
}
