package memory

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"strings"
	"testing"
	"time"

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
	s := NewStore(context.Background(), pool, fakeEmbedder{}, nil)
	// Drain background embeds before the pool closes so no worker touches a
	// closed pool. Cleanups run last-registered-first, so this runs before the
	// pool.Close registered above.
	t.Cleanup(func() {
		s.flushEmbeds()
		s.Close()
	})
	return s
}

// mustKey mints an API key and returns its id (the memory partition key).
func mustKey(t *testing.T, s *Store) string {
	t.Helper()
	_, id, err := s.CreateAPIKey(context.Background(), "test")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return id
}

func TestWriteGetDelete(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	user := mustKey(t, s)

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

// slowEmbedder wraps fakeEmbedder with a delay, standing in for the real
// OpenAI round-trip so a test can observe that a write does not wait on it.
type slowEmbedder struct{ delay time.Duration }

func (s slowEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return fakeEmbedder{}.Embed(ctx, text)
}

// TestWriteReturnsBeforeEmbedding pins the whole point of the async path: a write
// returns immediately (no embedding round-trip on the request path), and the
// vector is filled in by a background worker a moment later.
func TestWriteReturnsBeforeEmbedding(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE memories`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// embedDelay is set far above any plausible INSERT round-trip (the DB may be
	// remote) so the two are cleanly separable: if the write returns in a small
	// fraction of it, the write demonstrably did not wait on the embedding.
	const embedDelay = 3 * time.Second
	s := NewStore(ctx, pool, slowEmbedder{delay: embedDelay}, nil)
	t.Cleanup(func() { s.flushEmbeds(); s.Close() })

	user := mustKey(t, s)
	start := time.Now()
	m, err := s.Write(ctx, user, "async pipeline embeds off the request path", nil, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= embedDelay/2 {
		t.Fatalf("write appears to block on embedding: took %v (embed delay is %v)", elapsed, embedDelay)
	}

	// Immediately, the memory exists and is keyword-searchable, but not yet
	// vector-searchable.
	if _, err := s.Get(ctx, user, m.ID); err != nil {
		t.Fatalf("memory not readable right after write: %v", err)
	}

	// After the background worker completes, it joins vector search.
	s.flushEmbeds()
	hits, err := s.Search(ctx, user, SearchParams{Query: "async pipeline request path", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var vectorized bool
	for _, h := range hits {
		if h.ID == m.ID && (h.Source == "vector" || h.Source == "both") {
			vectorized = true
		}
	}
	if !vectorized {
		t.Fatalf("memory never became vector-searchable after flush; hits=%+v", hits)
	}
}

// TestScopingIsolation is the security-critical test: one user must never be
// able to read, update, or delete another user's memory, even with a valid id.
func TestScopingIsolation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	alice, bob := mustKey(t, s), mustKey(t, s)

	m, err := s.Write(ctx, alice, "alice private api key rotation notes", nil, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	s.flushEmbeds() // make the memory vector-searchable before asserting on search

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
	user := mustKey(t, s)

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
	s.flushEmbeds() // wait for background embedding so the vector arm participates

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
	user := mustKey(t, s)

	if _, err := s.Write(ctx, user, "the cat sat on the mat", nil, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	s.flushEmbeds() // so the row is embedded and the test exercises the threshold, not a missing vector

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
	user := mustKey(t, s)

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

// TestMemoryCapEviction pins the ring-buffer behaviour: a key keeps only the
// newest maxMemoriesPerKey memories; older ones are erased as new ones arrive.
func TestMemoryCapEviction(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	user := mustKey(t, s)

	const total = maxMemoriesPerKey + 5
	var ids []string
	for i := 0; i < total; i++ {
		m, err := s.Write(ctx, user, fmt.Sprintf("memory number %d", i), nil, nil)
		if err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		ids = append(ids, m.ID)
	}

	all, err := s.List(ctx, user, nil, 1000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != maxMemoriesPerKey {
		t.Fatalf("want %d memories after writing %d, got %d", maxMemoriesPerKey, total, len(all))
	}

	present := map[string]bool{}
	for _, m := range all {
		present[m.ID] = true
	}
	// The 5 oldest were evicted; the newest survived.
	for i := 0; i < 5; i++ {
		if present[ids[i]] {
			t.Fatalf("oldest memory (index %d) should have been evicted", i)
		}
	}
	if !present[ids[total-1]] {
		t.Fatal("newest memory was evicted")
	}
}

// TestEvictionIsPerKey: one key hitting the cap must not evict another key's
// memories.
func TestEvictionIsPerKey(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a, b := mustKey(t, s), mustKey(t, s)

	bMem, err := s.Write(ctx, b, "b's single memory", nil, nil)
	if err != nil {
		t.Fatalf("write b: %v", err)
	}
	for i := 0; i < maxMemoriesPerKey+5; i++ {
		if _, err := s.Write(ctx, a, fmt.Sprintf("a memory %d", i), nil, nil); err != nil {
			t.Fatalf("write a %d: %v", i, err)
		}
	}

	if _, err := s.Get(ctx, b, bMem.ID); err != nil {
		t.Fatalf("b's memory was collaterally evicted by a's writes: %v", err)
	}
}
