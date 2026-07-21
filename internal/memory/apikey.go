package memory

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
)

// APIKeyHeader carries the caller's key. The transport already rejects requests
// without a registered key (mcpx.RequireAPIKey); memory re-reads the header
// because it needs the key's *identity* to scope rows, not just admission.
const APIKeyHeader = auth.Header

var errNoKey = auth.ErrNoKey

// GenerateAPIKey mints a new key of the form mcp_<32 hex>.
func GenerateAPIKey() string { return auth.GenerateKey() }

// callerKey extracts and format-checks the caller's key from the request. It
// does not confirm the key exists -- that is the resolver's job (ResolveKey).
func callerKey(req *mcp.CallToolRequest) (string, error) {
	extra := req.GetExtra()
	if extra == nil || extra.Header == nil {
		return "", errNoKey
	}
	raw := strings.TrimSpace(extra.Header.Get(APIKeyHeader))
	if raw == "" {
		return "", errNoKey
	}
	if !auth.ValidFormat(raw) {
		return "", auth.ErrMalformed
	}
	return raw, nil
}
