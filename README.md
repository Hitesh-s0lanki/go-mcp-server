# go-mcp-server

Multi-namespace [Model Context Protocol](https://modelcontextprotocol.io) server in Go. Each namespace is an independent MCP server mounted on one HTTP mux over the **Streamable HTTP** transport:

| Route          | Namespace | Purpose                          |
| -------------- | --------- | -------------------------------- |
| `/memory/mcp`  | memory    | Memory storage & recall tools    |
| `/skills/mcp`  | skills    | Skill loading & discovery tools  |
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
  memory/              # register.go  (dummy memory_ping tool)
  skills/              # register.go  (dummy skills_ping tool)
  event/               # register.go  (dummy event_ping tool)
```

Each namespace currently exposes one dummy `*_ping` tool for verifying the
connection. `internal/mcpx/integration_test.go` drives a real MCP client
through `initialize` + `tools/call` against all three over an in-process
Streamable HTTP server.

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
