package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"golang.org/x/sync/errgroup"
)

// rrfK is the smoothing constant in Reciprocal Rank Fusion. 60 is the value from
// the original Cormack et al. paper and is the common default.
const rrfK = 60

// ErrNotFound is returned when a memory does not exist, or exists but belongs to
// another user. The two cases are deliberately indistinguishable: a distinct
// "exists but forbidden" error would let a caller probe for other users' IDs.
var ErrNotFound = errors.New("memory not found")

// Memory is a stored memory.
type Memory struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	Tags      []string          `json:"tags,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Hit is a search result. Score is cosine similarity in [0,1] when the hit came
// from vector search; see Store.Search for the fusion caveat.
type Hit struct {
	Memory
	Score  float32 `json:"score"`
	Source string  `json:"source"` // "vector", "keyword", or "both"
}

// Store is the Postgres-backed memory store. Every method is scoped by
// userEmail; there is no unscoped read path by construction.
type Store struct {
	db  *pgxpool.Pool
	emb Embedder
}

// NewStore builds a store.
func NewStore(db *pgxpool.Pool, emb Embedder) *Store {
	return &Store{db: db, emb: emb}
}

// Write stores a memory and returns it.
func (s *Store) Write(ctx context.Context, userEmail, content string, tags []string, metadata map[string]string) (*Memory, error) {
	vec, err := s.emb.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embed content: %w", err)
	}
	if tags == nil {
		tags = []string{}
	}
	if metadata == nil {
		metadata = map[string]string{}
	}

	var m Memory
	err = s.db.QueryRow(ctx, `
		INSERT INTO memories (user_email, content, embedding, tags, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, content, tags, metadata, created_at, updated_at`,
		userEmail, content, pgvector.NewVector(vec), tags, metadata,
	).Scan(&m.ID, &m.Content, &m.Tags, &m.Metadata, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}
	return &m, nil
}

// Get returns one memory by ID, scoped to the caller.
func (s *Store) Get(ctx context.Context, userEmail, id string) (*Memory, error) {
	var m Memory
	err := s.db.QueryRow(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at
		FROM memories
		WHERE id = $1 AND user_email = $2`,
		id, userEmail,
	).Scan(&m.ID, &m.Content, &m.Tags, &m.Metadata, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	return &m, nil
}

// Update replaces a memory's content and re-embeds it.
func (s *Store) Update(ctx context.Context, userEmail, id, content string) (*Memory, error) {
	vec, err := s.emb.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embed content: %w", err)
	}

	var m Memory
	err = s.db.QueryRow(ctx, `
		UPDATE memories
		SET content = $3, embedding = $4, updated_at = now()
		WHERE id = $1 AND user_email = $2
		RETURNING id, content, tags, metadata, created_at, updated_at`,
		id, userEmail, content, pgvector.NewVector(vec),
	).Scan(&m.ID, &m.Content, &m.Tags, &m.Metadata, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update memory: %w", err)
	}
	return &m, nil
}

// Delete removes a memory.
func (s *Store) Delete(ctx context.Context, userEmail, id string) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memories WHERE id = $1 AND user_email = $2`, id, userEmail)
	if err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List browses memories newest-first. It runs no embedding call, so it is the
// cheap path when the caller has no query.
func (s *Store) List(ctx context.Context, userEmail string, tags []string, limit int) ([]Memory, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at
		FROM memories
		WHERE user_email = $1
		  AND ($2::text[] IS NULL OR cardinality($2::text[]) = 0 OR tags && $2::text[])
		ORDER BY created_at DESC
		LIMIT $3`,
		userEmail, tags, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	var out []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.Tags, &m.Metadata, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SearchParams configures a hybrid search.
type SearchParams struct {
	Query    string
	Limit    int
	MinScore float32
	Tags     []string
}

// Search runs vector and keyword search concurrently and fuses the results.
//
// Concurrency shape: keyword search needs no embedding, so it starts
// immediately while the embedding call (the only real network latency on this
// path) is still in flight. errgroup cancels the sibling if either fails, so a
// dead vector index does not leave the keyword query hanging.
//
// Scoring caveat: RRF produces a rank-derived score with no similarity scale, so
// MinScore is applied to the *cosine* score before fusion. Hits found only by
// keyword search have no cosine score and are therefore not subject to it --
// they are exact lexical matches, which is the point of having them.
func (s *Store) Search(ctx context.Context, userEmail string, p SearchParams) ([]Hit, error) {
	if p.Limit <= 0 {
		p.Limit = 5
	}
	// Over-fetch each arm so fusion has material to work with.
	fetch := p.Limit * 4

	var (
		vecHits []Hit
		kwHits  []Hit
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		vec, err := s.emb.Embed(gctx, p.Query)
		if err != nil {
			return fmt.Errorf("embed query: %w", err)
		}
		vecHits, err = s.vectorSearch(gctx, userEmail, vec, p.Tags, fetch, p.MinScore)
		return err
	})

	g.Go(func() error {
		var err error
		kwHits, err = s.keywordSearch(gctx, userEmail, p.Query, p.Tags, fetch)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	hits := fuse(vecHits, kwHits)
	if len(hits) > p.Limit {
		hits = hits[:p.Limit]
	}
	return hits, nil
}

// vectorSearch is the semantic arm. MinScore is applied here, in cosine space,
// where a threshold is actually meaningful.
func (s *Store) vectorSearch(ctx context.Context, userEmail string, vec []float32, tags []string, limit int, minScore float32) ([]Hit, error) {
	// pgvector's cosine distance (<=>) is in [0,2]; similarity = 1 - distance.
	rows, err := s.db.Query(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at,
		       1 - (embedding <=> $2) AS score
		FROM memories
		WHERE user_email = $1
		  AND ($3::text[] IS NULL OR cardinality($3::text[]) = 0 OR tags && $3::text[])
		  AND 1 - (embedding <=> $2) >= $4
		ORDER BY embedding <=> $2
		LIMIT $5`,
		userEmail, pgvector.NewVector(vec), tags, minScore, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Content, &h.Tags, &h.Metadata,
			&h.CreatedAt, &h.UpdatedAt, &h.Score); err != nil {
			return nil, fmt.Errorf("scan vector hit: %w", err)
		}
		h.Source = "vector"
		out = append(out, h)
	}
	return out, rows.Err()
}

// keywordSearch is the lexical arm. It catches what embeddings miss: error
// codes, identifiers, exact function names.
func (s *Store) keywordSearch(ctx context.Context, userEmail, query string, tags []string, limit int) ([]Hit, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at,
		       ts_rank(content_tsv, websearch_to_tsquery('english', $2)) AS score
		FROM memories
		WHERE user_email = $1
		  AND content_tsv @@ websearch_to_tsquery('english', $2)
		  AND ($3::text[] IS NULL OR cardinality($3::text[]) = 0 OR tags && $3::text[])
		ORDER BY score DESC
		LIMIT $4`,
		userEmail, query, tags, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()

	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Content, &h.Tags, &h.Metadata,
			&h.CreatedAt, &h.UpdatedAt, &h.Score); err != nil {
			return nil, fmt.Errorf("scan keyword hit: %w", err)
		}
		h.Source = "keyword"
		out = append(out, h)
	}
	return out, rows.Err()
}

// fuse merges the two ranked lists with Reciprocal Rank Fusion.
//
// The reported Score stays the cosine similarity where one exists, because that
// is the number a caller can reason about. The RRF value is used only for
// ordering.
func fuse(vecHits, kwHits []Hit) []Hit {
	type entry struct {
		hit Hit
		rrf float64
	}
	merged := make(map[string]*entry, len(vecHits)+len(kwHits))

	for rank, h := range vecHits {
		merged[h.ID] = &entry{hit: h, rrf: 1.0 / float64(rrfK+rank+1)}
	}
	for rank, h := range kwHits {
		score := 1.0 / float64(rrfK+rank+1)
		if e, ok := merged[h.ID]; ok {
			e.rrf += score
			e.hit.Source = "both" // found by both arms: a strong signal
			continue
		}
		merged[h.ID] = &entry{hit: h, rrf: score}
	}

	out := make([]Hit, 0, len(merged))
	for _, e := range merged {
		out = append(out, e.hit)
	}
	// Sort by fused rank, descending. Insertion sort is fine: len is bounded by
	// limit*8, and it keeps the dependency surface at zero.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && merged[out[j].ID].rrf > merged[out[j-1].ID].rrf; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
