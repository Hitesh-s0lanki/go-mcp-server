package keysapi_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
	"github.com/Hitesh-s0lanki/go-mcp-server/internal/keysapi"
)

// TestStoreOwnerScopingAndCap covers the guarantees that make the key API safe:
// keys are scoped to their Clerk owner, the per-user cap is enforced at write
// time, and a delete only touches the caller's own keys. It needs a database
// (skips without DATABASE_URL), like the rest of the DB-backed suite.
func TestStoreOwnerScopingAndCap(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	store := keysapi.NewStore(pool)

	// Unique owners per run so parallel/repeat runs never collide.
	owner := "user_test_" + auth.GenerateKey()
	other := "user_test_" + auth.GenerateKey()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM api_keys WHERE clerk_user_id = ANY($1)`, []string{owner, other})
	})

	// Create up to the cap.
	first, err := store.Create(ctx, owner, "one")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if !auth.ValidFormat(first.Key) {
		t.Errorf("minted key has wrong format: %q", first.Key)
	}
	if !strings.HasPrefix(first.Masked, "mcp_") || strings.Contains(first.Masked, first.Key[4:]) {
		t.Errorf("masked preview leaks the secret: %q", first.Masked)
	}
	if _, err := store.Create(ctx, owner, "two"); err != nil {
		t.Fatalf("create second: %v", err)
	}

	// The third trips the cap.
	if _, err := store.Create(ctx, owner, "three"); !errors.Is(err, keysapi.ErrKeyLimit) {
		t.Fatalf("third create: got %v, want ErrKeyLimit", err)
	}

	// List is owner-scoped.
	list, err := store.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("owner list: got %d keys, want 2", len(list))
	}
	if got, err := store.List(ctx, other); err != nil || len(got) != 0 {
		t.Fatalf("other owner list: got %d (err %v), want 0", len(got), err)
	}

	// A different owner cannot delete this owner's key.
	if removed, err := store.Delete(ctx, other, first.ID); err != nil || removed {
		t.Fatalf("cross-owner delete: removed=%v err=%v, want removed=false", removed, err)
	}

	// The owner can, which frees a slot back under the cap.
	if removed, err := store.Delete(ctx, owner, first.ID); err != nil || !removed {
		t.Fatalf("owner delete: removed=%v err=%v, want removed=true", removed, err)
	}
	if _, err := store.Create(ctx, owner, "three-again"); err != nil {
		t.Fatalf("create after freeing a slot: %v", err)
	}

	// A non-UUID id is a clean miss, not a 500-worthy error.
	if removed, err := store.Delete(ctx, owner, "not-a-uuid"); err != nil || removed {
		t.Fatalf("bad-id delete: removed=%v err=%v, want removed=false err=nil", removed, err)
	}
}
