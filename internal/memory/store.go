package memory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"golang.org/x/sync/errgroup"
)

// rrfK is the smoothing constant in Reciprocal Rank Fusion. 60 is the value from
// the original Cormack et al. paper and is the common default.
const rrfK = 60

// maxMemoriesPerKey caps how many memories one API key retains. On the write
// that would exceed it, the oldest memories for that key are evicted so only the
// newest maxMemoriesPerKey survive -- a per-key ring buffer.
const maxMemoriesPerKey = 20

// Background-embedding tuning.
//
// A write inserts the row and hands the embedding off to a pool of workers, so
// the ~200-500ms OpenAI round-trip never sits on the request path. These bound
// the pool.
const (
	// embedQueueSize is the buffered hand-off between a write and the workers.
	// A write never blocks on it: an over-full queue spills to a detached
	// goroutine (see enqueueEmbed), so this only caps steady-state buffering.
	embedQueueSize = 1024
	// embedJobTimeout caps one embed+update. It is generous because the request
	// is already gone; the only cost of a slow job is a longer window before the
	// memory becomes vector-searchable.
	embedJobTimeout = 45 * time.Second
	// embedAttempts retries a transient embed failure. The synchronous path
	// deliberately had none, but here a dropped job would leave a row unindexed
	// until the next restart's backfill, so a couple of cheap retries pay off.
	embedAttempts = 3
	embedRetryGap = 500 * time.Millisecond
)

// ErrNotFound is returned when a memory does not exist, or exists but belongs to
// another key. The two cases are deliberately indistinguishable: a distinct
// "exists but forbidden" error would let a caller probe for other keys' IDs.
var ErrNotFound = errors.New("memory not found")

// ErrInvalidKey is returned when a presented API key is not in the api_keys
// table. Keys must be minted (GenerateAPIKey / CreateAPIKey); unknown keys are
// rejected rather than auto-provisioned.
var ErrInvalidKey = errors.New("unknown or invalid API key")

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
// apiKeyID; there is no unscoped read path by construction.
type Store struct {
	db  *pgxpool.Pool
	emb Embedder
	log *slog.Logger

	// Background embedding. A write returns after the INSERT and enqueues the
	// content here; workers compute the vector and UPDATE the row.
	embedQ  chan embedJob
	embedWG sync.WaitGroup // tracks outstanding jobs, for flushEmbeds/Close
	baseCtx context.Context
	cancel  context.CancelFunc

	// keyCache memoizes api-key-string -> api_key_id. Keys are effectively
	// immutable once minted, so a hit avoids a lookup on every tool call.
	keyMu    sync.RWMutex
	keyCache map[string]string
}

// embedJob is one unit of deferred embedding work. content is captured so the
// worker can detect a since-changed row and skip a stale write (last-write-wins,
// see runEmbed).
type embedJob struct {
	id      string
	content string
}

// NewStore builds a store and starts its background embedding workers. ctx is
// the process/shutdown context: when it is cancelled the workers stop. Passing a
// nil logger is fine; it falls back to the default.
func NewStore(ctx context.Context, db *pgxpool.Pool, emb Embedder, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	base, cancel := context.WithCancel(ctx)
	s := &Store{
		db:       db,
		emb:      emb,
		log:      log,
		embedQ:   make(chan embedJob, embedQueueSize),
		baseCtx:  base,
		cancel:   cancel,
		keyCache: make(map[string]string),
	}

	// One worker per core (min 2) drains the queue concurrently. Embedding is
	// network-bound, so cores is a floor, not a ceiling; the buffered queue plus
	// the spill goroutine in enqueueEmbed absorb any burst above this.
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	for i := 0; i < workers; i++ {
		go s.embedWorker()
	}

	// Recover rows left unembedded by a prior crash or shutdown.
	go s.backfillPending()

	return s
}

// Close stops the background workers. In-flight and queued jobs are abandoned
// (the next start's backfill re-embeds anything still pending); it does not wait
// for them. Tests that need a clean drain call flushEmbeds first.
func (s *Store) Close() { s.cancel() }

// CreateAPIKey mints a new key, stores it, and returns both the key string (what
// the caller presents in the X-API-Key header) and its id (the memory partition
// key). label is free-form for humans; it does not affect scoping.
func (s *Store) CreateAPIKey(ctx context.Context, label string) (key, id string, err error) {
	key = GenerateAPIKey()
	err = s.db.QueryRow(ctx,
		`INSERT INTO api_keys (key, label) VALUES ($1, $2) RETURNING id`,
		key, label,
	).Scan(&id)
	if err != nil {
		return "", "", fmt.Errorf("create api key: %w", err)
	}
	s.keyMu.Lock()
	s.keyCache[key] = id
	s.keyMu.Unlock()
	return key, id, nil
}

// ResolveKey maps a key string to its api_key_id, caching the result. An unknown
// key returns ErrInvalidKey -- keys are not auto-provisioned.
func (s *Store) ResolveKey(ctx context.Context, key string) (string, error) {
	s.keyMu.RLock()
	id, ok := s.keyCache[key]
	s.keyMu.RUnlock()
	if ok {
		return id, nil
	}

	err := s.db.QueryRow(ctx, `SELECT id FROM api_keys WHERE key = $1`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidKey
	}
	if err != nil {
		return "", fmt.Errorf("resolve api key: %w", err)
	}

	s.keyMu.Lock()
	s.keyCache[key] = id
	s.keyMu.Unlock()
	return id, nil
}

// enqueueEmbed hands a memory off for background embedding. It never blocks the
// caller: if the buffered queue is full it spawns a detached goroutine instead,
// so a burst of writes degrades to more concurrency rather than to latency.
func (s *Store) enqueueEmbed(id, content string) {
	if s.baseCtx.Err() != nil {
		return // shutting down; the next start's backfill will pick this up
	}
	s.embedWG.Add(1)
	job := embedJob{id: id, content: content}
	select {
	case s.embedQ <- job:
	default:
		go s.runEmbed(job)
	}
}

// embedWorker drains the queue until the store is closed.
func (s *Store) embedWorker() {
	for {
		select {
		case <-s.baseCtx.Done():
			return
		case job := <-s.embedQ:
			s.runEmbed(job)
		}
	}
}

// runEmbed computes one embedding and writes it back. It is the single owner of
// the embedWG counter added in enqueueEmbed.
//
// The UPDATE is guarded by `content = $3`: if the row was edited or deleted
// since this job was queued, it matches zero rows and the job is dropped -- the
// edit enqueued its own job with the current content, so the freshest write
// wins. `embedding IS NULL` keeps a redundant job from re-writing a vector a
// sibling worker already set.
func (s *Store) runEmbed(job embedJob) {
	defer s.embedWG.Done()

	var vec []float32
	var err error
	for attempt := 1; attempt <= embedAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(s.baseCtx, embedJobTimeout)
		vec, err = s.emb.Embed(ctx, job.content)
		cancel()
		if err == nil {
			break
		}
		if s.baseCtx.Err() != nil {
			return // shutting down: stop retrying
		}
		if attempt < embedAttempts {
			select {
			case <-time.After(embedRetryGap):
			case <-s.baseCtx.Done():
				return
			}
		}
	}
	if err != nil {
		s.log.Error("background embed failed; memory left keyword-only until re-embed",
			"id", job.id, "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(s.baseCtx, embedJobTimeout)
	defer cancel()
	_, err = s.db.Exec(ctx, `
		UPDATE memories
		SET embedding = $2
		WHERE id = $1 AND content = $3 AND embedding IS NULL`,
		job.id, pgvector.NewVector(vec), job.content)
	if err != nil && !errors.Is(err, context.Canceled) {
		s.log.Error("background embed write-back failed", "id", job.id, "err", err)
	}
}

// backfillPending re-enqueues rows whose embedding was never computed -- the
// tail left by a crash or a shutdown that dropped queued jobs. It runs once at
// startup. Cross-user by nature: this is table maintenance, not a read path.
func (s *Store) backfillPending() {
	ctx, cancel := context.WithTimeout(s.baseCtx, embedJobTimeout)
	defer cancel()
	rows, err := s.db.Query(ctx,
		`SELECT id, content FROM memories WHERE embedding IS NULL ORDER BY created_at`)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.log.Error("backfill scan for pending embeddings failed", "err", err)
		}
		return
	}
	defer rows.Close()

	var jobs []embedJob
	for rows.Next() {
		var j embedJob
		if err := rows.Scan(&j.id, &j.content); err != nil {
			s.log.Error("backfill scan row failed", "err", err)
			return
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		s.log.Error("backfill row iteration failed", "err", err)
		return
	}
	if len(jobs) == 0 {
		return
	}
	s.log.Info("re-embedding memories left pending by a prior run", "count", len(jobs))
	for _, j := range jobs {
		s.enqueueEmbed(j.id, j.content)
	}
}

// flushEmbeds blocks until every enqueued embedding job has finished. It exists
// for tests and graceful drains; it does not stop the workers.
func (s *Store) flushEmbeds() { s.embedWG.Wait() }

// Write stores a memory and returns it immediately. The embedding is computed
// off the request path by a background worker, so this call costs one local
// INSERT rather than an OpenAI round-trip. The returned memory has no vector
// yet: it is findable by id, tag, and keyword at once, and by semantic search
// as soon as its worker completes.
func (s *Store) Write(ctx context.Context, apiKeyID, content string, tags []string, metadata map[string]string) (*Memory, error) {
	if tags == nil {
		tags = []string{}
	}
	if metadata == nil {
		metadata = map[string]string{}
	}

	// Insert and evict run in one transaction so a reader never sees the key
	// briefly holding maxMemoriesPerKey+1 rows, and a crash can't leave the
	// eviction half-done.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var m Memory
	// embedding is omitted from the column list, so it defaults to NULL until a
	// worker fills it in.
	err = tx.QueryRow(ctx, `
		INSERT INTO memories (api_key_id, content, tags, metadata)
		VALUES ($1, $2, $3, $4)
		RETURNING id, content, tags, metadata, created_at, updated_at`,
		apiKeyID, content, tags, metadata,
	).Scan(&m.ID, &m.Content, &m.Tags, &m.Metadata, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}

	// Ring-buffer eviction: keep only the newest maxMemoriesPerKey for this key.
	// (created_at, id) is a total order, so eviction is deterministic even when
	// two rows share a timestamp.
	if _, err := tx.Exec(ctx, `
		DELETE FROM memories
		WHERE api_key_id = $1
		  AND id NOT IN (
		      SELECT id FROM memories
		      WHERE api_key_id = $1
		      ORDER BY created_at DESC, id DESC
		      LIMIT $2
		  )`,
		apiKeyID, maxMemoriesPerKey,
	); err != nil {
		return nil, fmt.Errorf("evict old memories: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit write: %w", err)
	}

	s.enqueueEmbed(m.ID, m.Content)
	return &m, nil
}

// Get returns one memory by ID, scoped to the caller.
func (s *Store) Get(ctx context.Context, apiKeyID, id string) (*Memory, error) {
	var m Memory
	err := s.db.QueryRow(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at
		FROM memories
		WHERE id = $1 AND api_key_id = $2`,
		id, apiKeyID,
	).Scan(&m.ID, &m.Content, &m.Tags, &m.Metadata, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	return &m, nil
}

// Update replaces a memory's content and returns immediately. The stale
// embedding is cleared and recomputed in the background, on the same path as
// Write: the memory keeps matching by keyword throughout, and rejoins vector
// search once its worker finishes. Nulling (rather than keeping) the old vector
// is deliberate -- an embedding of the previous content would surface the memory
// for the wrong queries during the gap.
func (s *Store) Update(ctx context.Context, apiKeyID, id, content string) (*Memory, error) {
	var m Memory
	err := s.db.QueryRow(ctx, `
		UPDATE memories
		SET content = $3, embedding = NULL, updated_at = now()
		WHERE id = $1 AND api_key_id = $2
		RETURNING id, content, tags, metadata, created_at, updated_at`,
		id, apiKeyID, content,
	).Scan(&m.ID, &m.Content, &m.Tags, &m.Metadata, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update memory: %w", err)
	}

	s.enqueueEmbed(m.ID, m.Content)
	return &m, nil
}

// Delete removes a memory.
func (s *Store) Delete(ctx context.Context, apiKeyID, id string) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memories WHERE id = $1 AND api_key_id = $2`, id, apiKeyID)
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
func (s *Store) List(ctx context.Context, apiKeyID string, tags []string, limit int) ([]Memory, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at
		FROM memories
		WHERE api_key_id = $1
		  AND ($2::text[] IS NULL OR cardinality($2::text[]) = 0 OR tags && $2::text[])
		ORDER BY created_at DESC
		LIMIT $3`,
		apiKeyID, tags, limit,
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
func (s *Store) Search(ctx context.Context, apiKeyID string, p SearchParams) ([]Hit, error) {
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
		vecHits, err = s.vectorSearch(gctx, apiKeyID, vec, p.Tags, fetch, p.MinScore)
		return err
	})

	g.Go(func() error {
		var err error
		kwHits, err = s.keywordSearch(gctx, apiKeyID, p.Query, p.Tags, fetch)
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
func (s *Store) vectorSearch(ctx context.Context, apiKeyID string, vec []float32, tags []string, limit int, minScore float32) ([]Hit, error) {
	// pgvector's cosine distance (<=>) is in [0,2]; similarity = 1 - distance.
	rows, err := s.db.Query(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at,
		       1 - (embedding <=> $2) AS score
		FROM memories
		WHERE api_key_id = $1
		  AND ($3::text[] IS NULL OR cardinality($3::text[]) = 0 OR tags && $3::text[])
		  AND 1 - (embedding <=> $2) >= $4
		ORDER BY embedding <=> $2
		LIMIT $5`,
		apiKeyID, pgvector.NewVector(vec), tags, minScore, limit,
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
func (s *Store) keywordSearch(ctx context.Context, apiKeyID, query string, tags []string, limit int) ([]Hit, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, content, tags, metadata, created_at, updated_at,
		       ts_rank(content_tsv, websearch_to_tsquery('english', $2)) AS score
		FROM memories
		WHERE api_key_id = $1
		  AND content_tsv @@ websearch_to_tsquery('english', $2)
		  AND ($3::text[] IS NULL OR cardinality($3::text[]) = 0 OR tags && $3::text[])
		ORDER BY score DESC
		LIMIT $4`,
		apiKeyID, query, tags, limit,
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
