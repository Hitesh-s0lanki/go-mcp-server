---
name: stateful-memory
description: >-
  Retrieve stored knowledge and stay stateful across sessions using this
  project's per-user memory tools (memory_search, memory_write, memory_list,
  memory_get, memory_update, memory_delete from the "memory" server in
  .mcp.json). Use whenever a task could draw on knowledge you may have saved
  before: at session start to recall who the user is, their preferences and
  stack; before any task that references prior work, a past decision, or a named
  project/feature/person ("continue the X", "the Y we discussed"); when the user
  asks something you might have recorded; when the user states a durable fact or
  preference about themselves or how they want to work (save it immediately); and
  after finishing substantial work, to persist a summary for future retrieval.
---

# Stateful knowledge & memory protocol

This project runs a per-user knowledge store with semantic + keyword retrieval
(RAG) — the `memory` MCP server in [.mcp.json](../../../.mcp.json), backed by
Postgres + pgvector. Use it to carry knowledge across sessions: who the user is,
their preferences and stack, and summaries of prior work and decisions, so a
session starts by *retrieving* relevant context instead of from a blank slate —
and *persists* what it learns for next time.

**Identity is automatic.** The `X-API-Key` header (set in `.mcp.json`) scopes
every call to one API key. Never pass an identity as a tool argument — there is
no such argument. Each key retains only its most recent 20 memories; older ones
are evicted as new ones are written, so store what matters and keep it current.

Tools: `memory_search` (semantic + keyword), `memory_write`, `memory_get` (full
content by id), `memory_update`, `memory_delete`, `memory_list` (browse by tag,
no query — cheaper, runs no embedding).

## When to RECALL (read)

Search memory instead of asking the user what you could remember:

- **Session start**, once: `memory_list` with `tags: ["user-profile"]` to load
  who they are (role, stack, preferences).
- **Before a task that references prior work** — a named project/feature/person,
  or "continue/finish/the … we discussed". `memory_search` for it first.
- **When the user assumes you already know something.** Search before asking.

Do **not** recall on trivial turns (a one-line fix, a yes/no question) or
re-search a topic already loaded this session. Over-recall wastes latency and
tokens.

```
memory_search { "query": "authentication middleware design decisions", "limit": 5 }
```

An empty result is a real answer — "no memory of this" — not a failure. Don't
retry; proceed, and consider writing a memory once the work is done.

## When to REMEMBER (write)

**After any substantial task** (a feature, a debugging session, an architecture
decision, a multi-step investigation), write a summary. Search first — if a
summary of the same thing exists, `memory_update` it rather than duplicating.

```
memory_search { "query": "hybrid search implementation", "tags": ["task-summary"] }
# no match -> memory_write ; match -> memory_update the existing id
memory_write {
  "content": "[TASK] Hybrid memory search\nWhat: we built retrieval that combines semantic meaning with keyword matching, fusing the two result lists.\nOutcome: shipped and verified against Neon + OpenAI.\nDecisions: similarity floor 0.35, calibrated for text-embedding-3-small; identity comes from the X-API-Key header, not a tool argument.\nFiles: internal/memory/store.go, tools.go.\nFollow-ups: no reranker yet.",
  "tags": ["task-summary", "project:go-mcp-server", "topic:memory"]
}
```

**When you learn a durable fact about the user** (role, preferences, how they
like to work, key constraints), store it under `user-profile` — **the moment
they state it**, not only after a task. This is the easiest trigger to miss: a
preference dropped mid-conversation ("treat me as a senior engineer", "I prefer
X") is durable knowledge; write it right away rather than letting it pass.

```
memory_write {
  "content": "Prefers Go stdlib over frameworks; wants tests run against real infra, not mocks.",
  "tags": ["user-profile"]
}
```

Do **not** store: secrets, API keys, tokens, throwaway detail, or anything the
repo or git history already records.

## Summary format (keep it short — it is re-read into context on every recall)

```
[TASK] <one-line title>
What: <what was done, 1-2 lines>
Outcome: <shipped / blocked / decided>
Decisions: <choices a future session must not re-litigate>
Files: <primary files touched>
Follow-ups: <what's left, if anything>
```

Write in plain sentences, not terse shorthand. Recall is semantic: a summary
phrased the way you'd later *ask* about it ("we built hybrid search combining
meaning and keywords") is retrieved far more reliably than jargon
("vector+BM25+RRF"). Short and readable beats short and cryptic.

## Tag taxonomy (consistent tags make retrieval precise)

- `user-profile` — durable facts about the user
- `task-summary` — one per completed substantial task
- `project:<name>` — e.g. `project:go-mcp-server`
- `topic:<area>` — e.g. `topic:memory`, `topic:auth`
- `decision` — a standalone architectural decision worth recalling

## Efficiency rules

1. Recall **once per topic per session**, then reuse what you loaded.
2. **Update, don't duplicate** — search before every write.
3. Keep summaries tight; they cost tokens on every future recall.
4. Prefer `memory_list` (no embedding call, cheap) when browsing by tag;
   `memory_search` (embeds the query) when you need semantic match.
5. Use `memory_search` for snippets + ids; `memory_get` only when you need one
   memory's full content.
