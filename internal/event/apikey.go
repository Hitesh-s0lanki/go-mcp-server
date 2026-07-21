package event

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
)

// APIKeyHeader carries the caller's key. The transport already rejects requests
// without a registered key (mcpx.RequireAPIKey); event re-reads the header
// because it needs the key's *identity* to scope events, not just admission --
// the same reason the memory namespace re-resolves it to scope rows.
const APIKeyHeader = auth.Header

// ownerHeader is the Kafka record header under which every published event is
// stamped with its owner's api_key_id. Consume validates this header against the
// caller's id so a user only ever receives its own events (unique event for
// unique user). The stored value is the non-secret api_key_id, never the key
// itself -- a topic is readable by anyone with cluster access, so the credential
// must never land in a record.
const ownerHeader = "x-mcp-owner"

var errNoKey = auth.ErrNoKey

// keyResolver maps a presented api key to its api_key_id. The concrete
// implementation is *auth.Resolver (a cached Postgres lookup); the interface
// keeps the owner-scoping logic unit-testable without a database.
type keyResolver interface {
	Resolve(ctx context.Context, key string) (string, error)
}

// callerKey extracts and format-checks the caller's key from the request. It
// does not confirm the key exists -- that is the resolver's job. Identity comes
// from the transport header, never a tool argument: a tool argument is chosen by
// the model, so any prompt could name a different key and read another caller's
// events; a header is set by client configuration the model cannot influence.
func callerKey(req *mcp.CallToolRequest) (string, error) {
	if req == nil {
		return "", errNoKey
	}
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
