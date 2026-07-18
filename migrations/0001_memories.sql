-- Memory namespace: per-user memories with hybrid (vector + full-text) search.
--
-- Embeddings are OpenAI text-embedding-3-small => vector(1536). The dimension is
-- baked into the column type: changing the embedding model means a migration and
-- a full re-embed, so it is not a runtime knob.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity comes from the transport (X-User-Email header), never from a tool
    -- argument -- otherwise the model could choose whose memories to read.
    -- Normalized to lowercase in Go before it reaches here.
    user_email  text        NOT NULL,

    content     text        NOT NULL,
    embedding   vector(1536) NOT NULL,
    tags        text[]      NOT NULL DEFAULT '{}',
    metadata    jsonb       NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    -- Generated so the keyword index can never drift from content.
    content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED
);

-- Every query is scoped by user, so this leads.
CREATE INDEX IF NOT EXISTS memories_user_idx        ON memories (user_email);
CREATE INDEX IF NOT EXISTS memories_user_created_idx ON memories (user_email, created_at DESC);

-- Keyword half of hybrid search.
CREATE INDEX IF NOT EXISTS memories_tsv_idx  ON memories USING GIN (content_tsv);
CREATE INDEX IF NOT EXISTS memories_tags_idx ON memories USING GIN (tags);

-- Vector half. Cosine distance (<=>) to match the similarity = 1 - distance
-- convention used in the store. HNSW beats IVFFlat here: no training step, and
-- better recall at low latency for a corpus this size.
CREATE INDEX IF NOT EXISTS memories_embedding_idx
    ON memories USING hnsw (embedding vector_cosine_ops);
