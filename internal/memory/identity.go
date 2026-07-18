package memory

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UserEmailHeader carries the caller's identity.
//
// Identity is deliberately taken from the transport rather than from a tool
// argument. A tool argument is chosen by the model, so any prompt could read or
// write another user's memories just by naming a different address. A header is
// set by the client's configuration, which the model cannot influence.
//
// This is scoping, not authentication: anyone who can reach the endpoint can set
// the header themselves. Treat it as a partition key. Before this is exposed
// beyond a trusted network, the email must come from a verified credential --
// RequestExtra.TokenInfo (OAuth bearer) is where that will land, and only this
// function needs to change.
const UserEmailHeader = "X-User-Email"

var errNoIdentity = errors.New(
	"missing " + UserEmailHeader + " header: the memory namespace is per-user, " +
		"so every call must identify its caller",
)

// callerEmail extracts and normalizes the caller's identity from the request.
// The returned address is lowercased and trimmed so that "A@B.com" and
// "a@b.com " address the same memories.
func callerEmail(req *mcp.CallToolRequest) (string, error) {
	extra := req.GetExtra()
	if extra == nil || extra.Header == nil {
		return "", errNoIdentity
	}

	raw := strings.TrimSpace(extra.Header.Get(UserEmailHeader))
	if raw == "" {
		return "", errNoIdentity
	}

	// Parse before normalizing: RFC 5322 allows forms ("Name <a@b.com>") that we
	// do not want to store verbatim as a partition key.
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", errors.New(UserEmailHeader + " is not a valid email address")
	}

	return strings.ToLower(addr.Address), nil
}
