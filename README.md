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
go run ./cmd/server
```

The server listens on `:8080` (override with `PORT`). Point an MCP client at, e.g., `http://localhost:8080/memory/mcp`.

## Adding a namespace

1. Create `internal/<name>/` with a `register.go` that calls `mcpx.Register` from `init()`.
2. Blank-import the package in `cmd/server/main.go`.

That's it — the mux picks it up on the next start.

## Development

```bash
go build ./...
go test ./...
go vet ./...
```

## License

MIT
