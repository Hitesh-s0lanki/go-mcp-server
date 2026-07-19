package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// chatEndpoint is the OpenAI chat completions endpoint. Hand-rolled for the same
// reason memory's embedder is: one endpoint, and the vendor SDK would be a large
// dependency for a single POST.
const chatEndpoint = "https://api.openai.com/v1/chat/completions"

// defaultAgentModel drives the skill-finder loop. gpt-4o-mini is cheap and a
// capable tool-caller; override with SKILLS_AGENT_MODEL. Temperature is left at
// the API default so the model can be swapped for one that rejects overrides.
const defaultAgentModel = "gpt-4o-mini"

// OpenAIChat is a minimal chat-completions client with function/tool calling.
type OpenAIChat struct {
	APIKey string
	Model  string
	Client *http.Client

	// endpoint is the completions URL; unexported and set to chatEndpoint in
	// production, overridden only by tests.
	endpoint string
}

// NewOpenAIChat builds a chat client. model may be empty, in which case
// defaultAgentModel is used.
func NewOpenAIChat(apiKey, model string) *OpenAIChat {
	if model == "" {
		model = defaultAgentModel
	}
	return &OpenAIChat{
		APIKey:   apiKey,
		Model:    model,
		Client:   &http.Client{Timeout: 90 * time.Second},
		endpoint: chatEndpoint,
	}
}

// chatMessage is one entry in the conversation. Content and ToolCalls are
// mutually exclusive per role: an assistant turn carries either text or tool
// calls; a tool turn carries the result keyed by ToolCallID.
type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // a JSON string, decoded by the caller
	} `json:"function"`
}

// chatTool is a function the model may call. Parameters is a raw JSON Schema.
type chatTool struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends one turn and returns the assistant message. tools may be nil.
func (c *OpenAIChat) Complete(ctx context.Context, messages []chatMessage, tools []chatTool) (*chatMessage, error) {
	body, err := json.Marshal(chatRequest{Model: c.Model, Messages: messages, Tools: tools})
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode chat response (status %d): %w", resp.StatusCode, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("openai chat: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai chat: status %d", resp.StatusCode)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("openai chat: no choices in response")
	}
	return &out.Choices[0].Message, nil
}
