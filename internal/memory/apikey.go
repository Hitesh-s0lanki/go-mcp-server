package memory

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// APIKeyHeader carries the caller's key. Identity comes from the transport, not
// from a tool argument: a tool argument is chosen by the model, so any prompt
// could read or write another key's memories by naming a different one. A header
// is set by the client's configuration, which the model cannot influence.
//
// This is scoping, not full authentication: anyone who can reach the endpoint
// and knows a key can use it. The key must exist in the api_keys table (mint one
// with GenerateAPIKey / the store's CreateAPIKey); unknown keys are rejected.
const APIKeyHeader = "X-API-Key" //nolint:gosec // G101 false positive: HTTP header name, not a credential

// keyPattern is the minted format: mcp_ followed by 32 lowercase hex characters
// (a UUID with its dashes stripped). It is a cheap pre-check before the database
// lookup; the api_keys table is the real authority on whether a key is valid.
var keyPattern = regexp.MustCompile(`^mcp_[0-9a-f]{32}$`)

var errNoKey = errors.New(
	"missing " + APIKeyHeader + " header: the memory namespace is per-key, " +
		"so every call must present an API key",
)

// GenerateAPIKey mints a new key of the form mcp_<32 hex>. The body is a
// random RFC 4122 v4 UUID with its dashes removed, produced from crypto/rand.
func GenerateAPIKey() string {
	var b [16]byte
	// crypto/rand.Read never returns a short read or error in practice; ignoring
	// err matches the standard library's own uuid-style helpers.
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10x
	return "mcp_" + hex.EncodeToString(b[:])
}

// callerKey extracts and format-checks the caller's key from the request. It
// does not confirm the key exists -- that is the store's job (ResolveKey).
func callerKey(req *mcp.CallToolRequest) (string, error) {
	extra := req.GetExtra()
	if extra == nil || extra.Header == nil {
		return "", errNoKey
	}
	raw := strings.TrimSpace(extra.Header.Get(APIKeyHeader))
	if raw == "" {
		return "", errNoKey
	}
	if !keyPattern.MatchString(raw) {
		return "", errors.New(APIKeyHeader + " is not a valid API key (want mcp_<32 hex>)")
	}
	return raw, nil
}
