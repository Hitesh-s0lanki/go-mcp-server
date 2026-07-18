// Package memory hosts the /memory/mcp namespace: per-user memories with
// hybrid (vector + keyword) retrieval backed by Postgres and pgvector.
package memory

import (
	"errors"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/mcpx"
)

func init() { mcpx.Register(namespace{}) }

type namespace struct{}

func (namespace) Path() string { return "/memory/mcp" }

func (namespace) Server(deps *mcpx.Deps) (*mcp.Server, error) {
	if deps.DB == nil {
		return nil, errors.New("DATABASE_URL is not set (the memory namespace needs Postgres + pgvector)")
	}

	emb, err := NewOpenAIEmbedder(os.Getenv("OPENAI_API_KEY"))
	if err != nil {
		return nil, err
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "memory", Version: "0.1.0"}, nil)
	registerTools(srv, NewStore(deps.DB, emb))
	return srv, nil
}
