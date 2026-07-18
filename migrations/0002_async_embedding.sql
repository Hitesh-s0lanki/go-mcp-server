-- Async embedding: a write returns as soon as the row is inserted, and a
-- background worker fills in the vector a moment later. The column must
-- therefore tolerate a transient NULL between INSERT and the worker's UPDATE.
--
-- Search is unaffected: the keyword arm matches a pending row immediately, and
-- the vector arm's `1 - (embedding <=> q) >= min_score` predicate evaluates to
-- NULL (excluded) for a NULL embedding, so a not-yet-embedded row simply does
-- not appear in vector results until its worker completes. The HNSW index skips
-- NULLs as well.
ALTER TABLE memories ALTER COLUMN embedding DROP NOT NULL;

-- The startup backfill scans for rows whose embedding was never computed (worker
-- crash, or process exit mid-flight) and re-enqueues them. This partial index
-- keeps that scan cheap regardless of how large the table grows, since it only
-- ever indexes the handful of rows that are momentarily pending.
CREATE INDEX IF NOT EXISTS memories_pending_embedding_idx
    ON memories (created_at) WHERE embedding IS NULL;
