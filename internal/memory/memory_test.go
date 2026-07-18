package memory

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeEmbedder maps text to a deterministic vector by hashing tokens into
// buckets and L2-normalizing. It needs no API key, and because shared words land
// in shared buckets it preserves the property under test: related text scores
// higher than unrelated text.
type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, errors.New("cannot embed empty text")
	}
	v := make([]float32, Dimensions)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[h.Sum32()%Dimensions] += 1
	}
	var norm float64
	for _, f := range v {
		norm += float64(f) * float64(f)
	}
	if norm == 0 {
		return nil, errors.New("no tokens to embed")
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
	return v, nil
}

// testStore connects to DATABASE_URL and returns a store with a clean table.
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(context.Background(), `TRUNCATE memories`); err != nil {
		t.Fatalf("truncate (did you apply migrations/0001_memories.sql?): %v", err)
	}
	return NewStore(pool, fakeEmbedder{})
}

func TestWriteGetDelete(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	const user = "a@example.com"

	m, err := s.Write(ctx, user, "the deploy pipeline runs on buildkite", []string{"ops"}, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if m.ID == "" {
		t.Fatal("write returned empty id")
	}

	got, err := s.Get(ctx, user, m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != m.Content {
		t.Fatalf("content mismatch: %q vs %q", got.Content, m.Content)
	}

	if err := s.Delete(ctx, user, m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, user, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

// TestScopingIsolation is the security-critical test: one user must never be
// able to read, update, or delete another user's memory, even with a valid id.
func TestScopingIsolation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	const alice, bob = "alice@example.com", "bob@example.com"

	m, err := s.Write(ctx, alice, "alice private api key rotation notes", nil, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := s.Get(ctx, bob, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob read alice's memory by id: err=%v", err)
	}
	if _, err := s.Update(ctx, bob, m.ID, "hijacked"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob updated alice's memory: err=%v", err)
	}
	if err := s.Delete(ctx, bob, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob deleted alice's memory: err=%v", err)
	}

	hits, err := s.Search(ctx, bob, SearchParams{Query: "alice private api key", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("bob's search returned alice's memories: %+v", hits)
	}

	// Alice still has it.
	if _, err := s.Get(ctx, alice, m.ID); err != nil {
		t.Fatalf("alice lost her own memory: %v", err)
	}
}

func TestHybridSearch(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	const user = "a@example.com"

	seed := []string{
		"the deploy pipeline runs on buildkite every morning",
		"postgres connection pooling uses pgbouncer in production",
		"the cat sat on the mat",
	}
	for _, c := range seed {
		if _, err := s.Write(ctx, user, c, nil, nil); err != nil {
			t.Fatalf("seed write: %v", err)
		}
	}

	hits, err := s.Search(ctx, user, SearchParams{
		Query: "deploy pipeline buildkite", Limit: 5,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits for a query that shares wording with a stored memory")
	}
	if !strings.Contains(hits[0].Content, "buildkite") {
		t.Fatalf("top hit should be the buildkite memory, got %q", hits[0].Content)
	}
}

// TestSearchBelowThresholdReturnsNothing pins the design decision that an empty
// result is a real answer ("no memory of this") rather than a failure.
func TestSearchBelowThresholdReturnsNothing(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	const user = "a@example.com"

	if _, err := s.Write(ctx, user, "the cat sat on the mat", nil, nil); err != nil {
		t.Fatalf("write: %v", err)
	}

	hits, err := s.Search(ctx, user, SearchParams{
		Query:    "kubernetes ingress controller tls termination",
		Limit:    5,
		MinScore: 0.9, // far above anything unrelated can reach
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("want no hits above a 0.9 floor, got %d: %+v", len(hits), hits)
	}
}

func TestListFiltersByTag(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	const user = "a@example.com"

	if _, err := s.Write(ctx, user, "ops runbook", []string{"ops"}, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.Write(ctx, user, "grocery list", []string{"personal"}, nil); err != nil {
		t.Fatalf("write: %v", err)
	}

	all, err := s.List(ctx, user, nil, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 memories unfiltered, got %d", len(all))
	}

	ops, err := s.List(ctx, user, []string{"ops"}, 10)
	if err != nil {
		t.Fatalf("list tagged: %v", err)
	}
	if len(ops) != 1 || ops[0].Content != "ops runbook" {
		t.Fatalf("tag filter wrong: %+v", ops)
	}
}
