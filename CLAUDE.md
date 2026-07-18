# go-mcp-server

Multi-namespace MCP server in Go. Three independent MCP servers mounted on one
HTTP mux over Streamable HTTP: `/memory/mcp`, `/skills/mcp`, `/event/mcp`.
Namespaces self-register via `init()`; `cmd/server/main.go` never changes when
one is added. Architecture detail in [README.md](README.md).

## Commands

- `make run` — run the server (loads `.env`)
- `make test` — all tests; DB-backed tests skip without `DATABASE_URL`
- `make check` — fmt + vet + lint + test (run before committing)
- `make tunnel` — expose locally via ngrok

Requires Go 1.26+, Postgres+pgvector (`DATABASE_URL`), and `OPENAI_API_KEY` for
the memory namespace. Never commit `.env`.

## Stateful knowledge — retrieve before you act, persist after

This project has a per-user knowledge store backed by semantic + keyword search
(the `memory` MCP server in [.mcp.json](.mcp.json), RAG over Postgres+pgvector).
Treat it as long-term memory of the user and the work:

- **Retrieve** relevant knowledge in real time before acting — who the user is,
  their preferences and stack, and any prior decisions, summaries, or context
  about the topic at hand. Pull it instead of asking, or assuming a blank slate.
- **Persist** new knowledge after substantial work — a concise summary of what
  was done and decided — so a future session can retrieve it.

The **`stateful-memory` skill** ([.claude/skills/stateful-memory/SKILL.md](.claude/skills/stateful-memory/SKILL.md))
is the operating protocol: when and how to retrieve, when to persist, the
summary format, and the tag taxonomy. It loads automatically when a task could
draw on stored knowledge — starting work, referencing past work, or being asked
something you may have recorded before. Follow it whenever you touch the
`memory_*` tools.
