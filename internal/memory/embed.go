package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Dimensions is the vector width of text-embedding-3-small. It must match the
// vector(1536) column in migrations/0001_memories.sql.
const Dimensions = 1536

// EmbedModel is the model this package is calibrated against. Changing it means
// a schema migration (if the width differs) and a full re-embed -- old and new
// vectors are not comparable.
const EmbedModel = "text-embedding-3-small"

// Embedder turns text into a vector. It is an interface so the model can be
// swapped (Voyage, a local ONNX model) without touching the store or the tools.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// OpenAIEmbedder calls the OpenAI embeddings endpoint.
//
// This is a hand-rolled client rather than the full OpenAI SDK: it is a single
// endpoint, and the SDK would be a large dependency for one POST. Retries are
// deliberately absent -- the caller's context deadline governs, and the tool
// layer surfaces failures to the model rather than hiding latency in silent
// retries.
type OpenAIEmbedder struct {
	APIKey string
	Client *http.Client

	// cache memoizes query embeddings. Agents re-issue near-identical queries
	// constantly, and this is the only call on the read path with real network
	// latency (~50-100ms).
	mu    sync.RWMutex
	cache map[string][]float32
}

// NewOpenAIEmbedder builds an embedder. It returns an error when the API key is
// absent so the failure surfaces at startup, not on the first search.
func NewOpenAIEmbedder(apiKey string) (*OpenAIEmbedder, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set (required for %s)", EmbedModel)
	}
	return &OpenAIEmbedder{
		APIKey: apiKey,
		Client: &http.Client{Timeout: 30 * time.Second},
		cache:  make(map[string][]float32),
	}, nil
}

type embedRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Embed returns the vector for text, using a cached value when available.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("cannot embed empty text")
	}

	e.mu.RLock()
	if v, ok := e.cache[text]; ok {
		e.mu.RUnlock()
		return v, nil
	}
	e.mu.RUnlock()

	body, err := json.Marshal(embedRequest{Input: text, Model: EmbedModel})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	var out embedResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode embed response (status %d): %w", resp.StatusCode, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("embed api: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed api: status %d", resp.StatusCode)
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embed api: empty embedding in response")
	}

	vec := out.Data[0].Embedding
	// Guard the schema contract: a model returning a different width would fail
	// deep inside pgvector with a far less obvious error.
	if len(vec) != Dimensions {
		return nil, fmt.Errorf("embed api: got %d dimensions, schema expects %d",
			len(vec), Dimensions)
	}

	e.mu.Lock()
	e.cache[text] = vec
	e.mu.Unlock()

	return vec, nil
}
