// Package keysapi is the Clerk-authenticated HTTP API the web dashboard uses to
// list, create, and revoke a user's MCP API keys.
//
// It is the authority for key ownership: every key it mints records the Clerk
// user who created it (clerk_user_id) and the per-user cap is enforced here.
// The MCP namespaces never see any of this — they only admit a presented key
// via internal/auth. Identity comes from the verified Clerk token
// (internal/clerkauth), never from the request body, so a caller can only ever
// touch their own keys.
package keysapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
	"github.com/Hitesh-s0lanki/go-mcp-server/internal/clerkauth"
)

// MaxKeysPerUser is the most keys a single Clerk user may hold at once. It
// mirrors the web dashboard's limit; keep the two in sync.
const MaxKeysPerUser = 2

// maxLabelRunes bounds the human label so a caller cannot store arbitrary blobs.
const maxLabelRunes = 60

// ErrKeyLimit is returned by Store.Create when the caller is already at the cap.
var ErrKeyLimit = fmt.Errorf("you can have at most %d API keys", MaxKeysPerUser)

// Summary is a key as shown in the list: the secret itself is never included,
// only a masked preview. JSON tags match the web dashboard's expected shape.
type Summary struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Masked    string `json:"masked"`
	CreatedAt string `json:"createdAt"`
}

// Created is the one-time response to minting a key: it carries the full secret,
// which is the only time it is ever exposed.
type Created struct {
	Summary
	Key string `json:"key"`
}

// Store is the owner-scoped data access for api_keys.
type Store struct{ db *pgxpool.Pool }

// NewStore builds a Store over the given pool.
func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// mask turns a full key into a recognisable-but-useless preview: the "mcp_"
// prefix and the last four characters. It mirrors the web dashboard's mask so a
// key looks identical wherever it is shown.
func mask(key string) string {
	if len(key) < 4 {
		return key
	}
	return "mcp_" + strings.Repeat("•", 8) + key[len(key)-4:]
}

// List returns the keys owned by clerkUserID, newest first. Secrets are masked.
func (s *Store) List(ctx context.Context, clerkUserID string) ([]Summary, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, label, key, created_at
		FROM api_keys
		WHERE clerk_user_id = $1
		ORDER BY created_at DESC`, clerkUserID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	out := []Summary{}
	for rows.Next() {
		var id, label, key string
		var createdAt time.Time
		if err := rows.Scan(&id, &label, &key, &createdAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, Summary{
			ID:        id,
			Label:     label,
			Masked:    mask(key),
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		})
	}
	return out, rows.Err()
}

// Create mints a key for clerkUserID and returns the full secret ONCE. The cap
// is enforced inside the INSERT — the row is written only if the user is under
// the limit, so the count and the insert are one statement rather than a
// check-then-act with a gap. Zero rows back means the guard tripped.
func (s *Store) Create(ctx context.Context, clerkUserID, label string) (*Created, error) {
	key := auth.GenerateKey()

	var id string
	var createdAt time.Time
	err := s.db.QueryRow(ctx, `
		INSERT INTO api_keys (key, label, clerk_user_id)
		SELECT $1, $2, $3
		WHERE (
			SELECT count(*) FROM api_keys WHERE clerk_user_id = $3
		) < $4
		RETURNING id, created_at`, key, label, clerkUserID, MaxKeysPerUser).Scan(&id, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrKeyLimit
	}
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	return &Created{
		Summary: Summary{
			ID:        id,
			Label:     label,
			Masked:    mask(key),
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		},
		Key: key,
	}, nil
}

// Delete revokes a key, but only if it belongs to clerkUserID — the owner guard
// makes it impossible to delete another user's key by guessing its id. Reports
// whether a row was actually removed. Deleting cascades to that key's memories.
func (s *Store) Delete(ctx context.Context, clerkUserID, id string) (bool, error) {
	// Reject a non-UUID id as "not found" rather than letting Postgres raise a
	// 22P02 that would surface as a 500. A caller only ever holds real ids.
	if _, perr := uuid.Parse(id); perr != nil {
		return false, nil //nolint:nilerr // a non-UUID id is a clean miss, not a server error
	}
	tag, err := s.db.Exec(ctx,
		`DELETE FROM api_keys WHERE id = $1 AND clerk_user_id = $2`, id, clerkUserID)
	if err != nil {
		return false, fmt.Errorf("delete api key: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Handler serves the key-management routes. Mount each method behind
// clerkauth.RequireUser so UserID is always present.
type Handler struct {
	store *Store
	log   *slog.Logger
}

// NewHandler builds a Handler over the given store.
func NewHandler(store *Store, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{store: store, log: log}
}

// List handles GET /api/keys.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	owner, ok := clerkauth.UserID(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keys, err := h.store.List(r.Context(), owner)
	if err != nil {
		h.log.Error("list api keys failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to list keys")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys, "max": MaxKeysPerUser})
}

// Create handles POST /api/keys. Body: {label?: string}.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	owner, ok := clerkauth.UserID(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// An empty or invalid body is fine — the label just defaults to "".
	var body struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	label := clampLabel(body.Label)

	created, err := h.store.Create(r.Context(), owner, label)
	if errors.Is(err, ErrKeyLimit) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		h.log.Error("create api key failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to create key")
		return
	}
	// 201 + the full secret. This is the only response that ever carries it.
	writeJSON(w, http.StatusCreated, map[string]any{"key": created})
}

// Delete handles DELETE /api/keys/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	owner, ok := clerkauth.UserID(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	removed, err := h.store.Delete(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		h.log.Error("delete api key failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to revoke key")
		return
	}
	if !removed {
		writeErr(w, http.StatusNotFound, "key not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// clampLabel trims and bounds a label to maxLabelRunes, rune-safe so a multibyte
// character is never split.
func clampLabel(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxLabelRunes {
		return strings.TrimSpace(string(r[:maxLabelRunes]))
	}
	return s
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
