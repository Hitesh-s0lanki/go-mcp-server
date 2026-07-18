-- Replace per-user (email) scoping with API-key scoping, and cap memories per
-- key. Memories are now partitioned by an api_keys row, not by an email header.

-- Keys minted by the app: format mcp_<32 lowercase hex> (a UUID with its dashes
-- removed). The id is the surrogate every memory references.
CREATE TABLE IF NOT EXISTS api_keys (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key        text        NOT NULL UNIQUE,
    label      text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Dummy key for development so the memory namespace works out of the box.
-- Referenced by .mcp.json (X-API-Key header). Replace with a minted key
-- (`make apikey`) before anything real.
INSERT INTO api_keys (key, label)
VALUES ('mcp_d398e9b902cc4cf3ab438dbe8cf76715', 'dummy-dev-key')
ON CONFLICT (key) DO NOTHING;

-- Repoint memories from user_email to api_key_id.
ALTER TABLE memories ADD COLUMN IF NOT EXISTS api_key_id uuid;

-- Assign any pre-existing rows to the dummy key so the NOT NULL + FK can apply
-- without data loss (dev tables may hold rows from the email era).
UPDATE memories
SET api_key_id = (SELECT id FROM api_keys WHERE key = 'mcp_d398e9b902cc4cf3ab438dbe8cf76715')
WHERE api_key_id IS NULL;

ALTER TABLE memories
    ALTER COLUMN api_key_id SET NOT NULL;

-- Deleting a key erases its memories.
ALTER TABLE memories
    DROP CONSTRAINT IF EXISTS memories_api_key_fk,
    ADD CONSTRAINT memories_api_key_fk
        FOREIGN KEY (api_key_id) REFERENCES api_keys (id) ON DELETE CASCADE;

-- Drop the old email scoping (indexes first, then the column).
DROP INDEX IF EXISTS memories_user_idx;
DROP INDEX IF EXISTS memories_user_created_idx;
ALTER TABLE memories DROP COLUMN IF EXISTS user_email;

-- New scoping indexes. The (api_key_id, created_at DESC) index also serves the
-- per-key eviction query that keeps only the newest N memories.
CREATE INDEX IF NOT EXISTS memories_key_idx         ON memories (api_key_id);
CREATE INDEX IF NOT EXISTS memories_key_created_idx ON memories (api_key_id, created_at DESC);
