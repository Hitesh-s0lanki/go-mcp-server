// Package auth holds API-key authentication: the header contract, the key
// format, and the resolver that maps a presented key to its api_keys row.
//
// It sits below both the transport and the domain packages. mcpx uses it to
// admit or reject a request before dispatch; the memory namespace uses the same
// resolver to turn the caller's key into the partition id that scopes its rows.
// auth imports neither, which is what keeps mcpx free of domain dependencies.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Header carries the caller's key. Identity comes from the transport, not from
// a tool argument: a tool argument is chosen by the model, so any prompt could
// name a different key and read or write another caller's data. A header is set
// by the client's configuration, which the model cannot influence.
//
// This is admission control plus scoping, not user authentication: anyone who
// can reach the endpoint and knows a key can use it.
const Header = "X-API-Key" //nolint:gosec // G101 false positive: HTTP header name, not a credential

// keyPattern is the minted format: mcp_ followed by 32 lowercase hex characters
// (a UUID with its dashes stripped). It is a cheap pre-check before the database
// lookup; the api_keys table is the real authority on whether a key is valid.
var keyPattern = regexp.MustCompile(`^mcp_[0-9a-f]{32}$`)

var (
	// ErrNoKey means the header was absent or empty.
	ErrNoKey = errors.New("missing " + Header + " header")
	// ErrMalformed means the header was present but not in the minted format.
	ErrMalformed = errors.New(Header + " is not a valid API key (want mcp_<32 hex>)")
	// ErrInvalidKey means the key is well-formed but not in the api_keys table.
	// Keys must be minted; unknown keys are rejected, not auto-provisioned.
	ErrInvalidKey = errors.New("unknown or invalid API key")
)

// GenerateKey mints a new key of the form mcp_<32 hex>. The body is a random
// RFC 4122 v4 UUID with its dashes removed, produced from crypto/rand.
func GenerateKey() string {
	var b [16]byte
	// crypto/rand.Read never returns a short read or error in practice; ignoring
	// err matches the standard library's own uuid-style helpers.
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10x
	return "mcp_" + hex.EncodeToString(b[:])
}

// ValidFormat reports whether key matches the minted format. It says nothing
// about whether the key exists -- that is Resolver.Resolve's job.
func ValidFormat(key string) bool { return keyPattern.MatchString(key) }

// Resolver maps a key string to its api_key_id, memoizing the result. Keys are
// effectively immutable once minted, so a cache hit avoids a database round
// trip on every request.
type Resolver struct {
	db *pgxpool.Pool

	mu    sync.RWMutex
	cache map[string]string
}

// NewResolver builds a resolver over the given pool.
func NewResolver(db *pgxpool.Pool) *Resolver {
	return &Resolver{db: db, cache: make(map[string]string)}
}

// Resolve returns the api_key_id for key. It returns ErrMalformed if the key is
// not in the minted format and ErrInvalidKey if it is absent from the table.
func (r *Resolver) Resolve(ctx context.Context, key string) (string, error) {
	if !ValidFormat(key) {
		return "", ErrMalformed
	}

	r.mu.RLock()
	id, ok := r.cache[key]
	r.mu.RUnlock()
	if ok {
		return id, nil
	}

	err := r.db.QueryRow(ctx, `SELECT id FROM api_keys WHERE key = $1`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidKey
	}
	if err != nil {
		return "", fmt.Errorf("resolve api key: %w", err)
	}

	r.mu.Lock()
	r.cache[key] = id
	r.mu.Unlock()
	return id, nil
}

// Create mints a key, stores it, and returns both the key string (what the
// caller presents in the header) and its id (the memory partition key). label
// is free-form for humans; it does not affect scoping.
func (r *Resolver) Create(ctx context.Context, label string) (key, id string, err error) {
	key = GenerateKey()
	err = r.db.QueryRow(ctx,
		`INSERT INTO api_keys (key, label) VALUES ($1, $2) RETURNING id`,
		key, label,
	).Scan(&id)
	if err != nil {
		return "", "", fmt.Errorf("create api key: %w", err)
	}

	r.mu.Lock()
	r.cache[key] = id
	r.mu.Unlock()
	return key, id, nil
}
