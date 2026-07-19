# go-mcp-server

Multi-namespace [Model Context Protocol](https://modelcontextprotocol.io) server in Go. Each namespace is an independent MCP server mounted on one HTTP mux over the **Streamable HTTP** transport:

| Route          | Namespace | Purpose                          |
| -------------- | --------- | -------------------------------- |
| `/memory/mcp`  | memory    | Memory storage & recall tools    |
| `/skills/mcp`  | skills    | Web search & scrape (Firecrawl)  |
| `/event/mcp`   | event     | Event-related tools              |

Namespaces self-register at startup, so adding a new one is a single package with an `init()` — `main.go` never changes. Router is stdlib `net/http`; a domain never reaches across another domain except through `internal/mcpx`.

## Stack

- **Go 1.26+**
- **[github.com/modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)** — official MCP SDK (Streamable HTTP handler, tool registration)
- **stdlib `net/http`** — mux + middleware
- **`log/slog`** — structured logging

## Layout

```
cmd/server/main.go     # config, graceful shutdown
internal/
  mcpx/                # registry, Handler(), Chain(); integration test
  memory/              # per-user memories, hybrid RAG (Postgres + pgvector)
  skills/              # skills_find agent + skills_download + web primitives
  event/               # register.go  (dummy event_ping tool)
```

`internal/mcpx/integration_test.go` drives a real MCP client through
`initialize` + `tools/call` against the namespaces over an in-process
Streamable HTTP server.

### skills namespace

An on-demand skill agent, a complete-skill downloader, and the raw web
primitives they are built on. **Nothing is stored** — every call reflects live
GitHub, so the catalogue is always current and the server holds no skill data.

- **`skills_find`** — the headline tool, and the go-to for obtaining *any* skill.
  Give it a natural-language requirement ("edit a PDF form", "build an MCP
  server") and it runs a live OpenAI tool-calling loop over the Firecrawl tools:
  search GitHub → pick the best Agent Skill → fetch its full `SKILL.md` → return
  the complete, ready-to-use skill with source links. Pass **multiple**
  requirements via `requirements: [...]` and they are resolved **in parallel**
  (one agent each, bounded fan-out), so batching is much faster than one call
  per need.
- **`skills_download`** — once you've located a skill, this downloads the
  **whole package**: every file in the skill folder (`SKILL.md` + scripts +
  reference files, recursively) via the GitHub contents API, fetched
  concurrently. Takes a GitHub URL (repo/tree/blob/raw) or `owner/repo/path`.
- **`firecrawl_search`** — web search returning ranked results (url, title,
  description); set `scrape=true` to also inline each page as markdown.
- **`firecrawl_scrape`** — fetch one URL as clean markdown (optionally raw HTML).

```
skills_find(requirements[])          skills_download(source)
   │  one OpenAI agent per need          │  GitHub contents API
   │  (parallel, bounded)                ├─ list skill dir (recursive)
   ├─▶ search_github ─┐                  └─ fetch every raw file (concurrent)
   └─▶ fetch_url ─────┴ Firecrawl        ▼
   ▼                                  complete skill: all files, in full
complete SKILL.md + sources
```

Config: `FIRECRAWL_API_KEY` powers the web tools (optional — without it Firecrawl
uses a lower unauthenticated rate limit, so those tools mount either way).
`skills_find` additionally needs `OPENAI_API_KEY` (and honours
`SKILLS_AGENT_MODEL`, default `gpt-4o-mini`); it is skipped when that key is
absent. `skills_download` needs no key — an optional `GITHUB_TOKEN` just raises
GitHub's unauthenticated rate limit. None are hard startup dependencies (unlike
memory, which requires Postgres).

## Quick start

```bash
cp .env.example .env
make run
```

The server listens on `:8080` (override with `PORT`). Point an MCP client at, e.g., `http://localhost:8080/memory/mcp`.

## Adding a namespace

1. Create `internal/<name>/` with a `register.go` that calls `mcpx.Register` from `init()`.
2. Blank-import the package in `cmd/server/main.go`.

That's it — the mux picks it up on the next start.

## Development

`make` on its own lists every target.

| Target       | Does                                      |
| ------------ | ----------------------------------------- |
| `make run`   | Run the server (loads `.env` if present)  |
| `make build` | Compile to `bin/`                         |
| `make test`  | Run all tests (`testv` for verbose)       |
| `make cover` | Coverage report in the browser            |
| `make check` | fmt + vet + lint + test — run before a PR |
| `make lint`  | `golangci-lint` (skipped if not installed)|
| `make lintfix`| Lint with `--fix` for auto-fixable issues |
| `make health`| Curl `/healthz` on a running server       |
| `make tunnel`| Expose the local server publicly via ngrok|
| `make clean` | Remove `bin/` and coverage artifacts      |

### Public tunnel

To point a hosted MCP client at your local server, run the server and the
tunnel in two terminals:

```bash
MCP_ALLOW_EXTERNAL_HOST=true make run   # terminal 1
make tunnel                             # terminal 2
```

> **`MCP_ALLOW_EXTERNAL_HOST=true` is required behind a tunnel.** The MCP
> transport has DNS-rebinding protection that rejects any request arriving on a
> loopback address with a non-loopback `Host` header — precisely what ngrok
> does. Without it every MCP request fails with
> `403 Forbidden: invalid Host header`, while `/healthz` still returns 200
> (it doesn't go through the transport), which makes the server look healthy.
> Set it only when a trusted proxy is in front. It lives in `.env.example`.

Namespaces are then reachable at `https://<NGROK_URL>/memory/mcp` and friends.
Override the defaults if needed: `make tunnel NGROK_URL=your.ngrok-free.app PORT=9000`.

## Connecting a client

`.mcp.json` registers all three namespaces as Streamable HTTP servers, pointing
at the ngrok domain by default. Clients that read project-scoped MCP config
(e.g. Claude Code) pick it up automatically.

To point at a local server instead of the tunnel, override the base URL:

```bash
MCP_BASE_URL=http://localhost:8080 claude
```

### Linting

Config lives in `.golangci.yml` (golangci-lint **v2** schema). On top of the
defaults (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`) it enables
checks that matter for an HTTP server — `bodyclose`, `noctx`, `errorlint`,
`nilerr`, `gosec` — plus `sloglint` for the logging style and `revive` for
exported-symbol docs.

Formatting is `gofmt` + `goimports` driven through `golangci-lint fmt`, with
this module's imports grouped into their own block. Run `make fmt`, not bare
`gofmt`, so the import grouping stays consistent.

```bash
brew install golangci-lint   # if you don't have it
make lint
```

## License

MIT
