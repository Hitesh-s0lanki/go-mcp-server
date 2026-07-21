-- Give api_keys an owner so the web app can scope keys to the Clerk user who
-- minted them. The MCP server itself stays owner-agnostic: it resolves a key to
-- its id regardless of who owns it, so this column is purely for the dashboard's
-- "list/create/delete my keys" view and the max-2-per-user cap.
--
-- Nullable on purpose: the dev/dummy key and any key minted before this column
-- existed have no Clerk owner and must keep working. They simply belong to no
-- user and never appear in anyone's dashboard.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS clerk_user_id text;

-- The dashboard lists and counts keys by owner; index that lookup.
CREATE INDEX IF NOT EXISTS api_keys_owner_idx ON api_keys (clerk_user_id);
