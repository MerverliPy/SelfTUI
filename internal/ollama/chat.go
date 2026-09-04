package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Role identifies the speaker of a message in POST /api/chat conversations
// (PLAN.md §5). Plain chat (M2) sends user/assistant pairs; the agent tool
// loop (M3) adds system on top.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ChatMessage is one exchange in a /api/chat conversation.
type ChatMessage struct {
	Role      Role       `json:"role"`
	Content   string     `json:"content"`
	Thinking  string     `json:"thinking,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

// ToolCall is a model-requested function invocation. Arguments are kept as
// raw JSON so the agent can validate the concrete schema at the execution
// boundary rather than trusting the model's types.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDefinition describes one function exposed to Ollama's chat endpoint.
type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatOptions are the sampling / context controls sent under "options".
// Zero values are omitted so the server default applies.
type ChatOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

// ChatRequest is the POST /api/chat body. M2 plain chat leaves Tools empty;
// M3a supplies the explicit read-only tool definitions.
type ChatRequest struct {
	Model    string           `json:"model"`
	Messages []ChatMessage    `json:"messages"`
	Stream   bool             `json:"stream"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
	// Options is a pointer so that an all-zero options block is omitted from
	// the wire (Go's omitempty does not apply to structs).
	Options *ChatOptions `json:"options,omitempty"`
}

// ChatEvent is one decoded NDJSON response from /api/chat. Thinking is
// deliberately separate from Content: qwen3 may stream reasoning that must
// not be shown as the assistant's final answer or fed to a tool parser.
type ChatEvent struct {
	Message    ChatMessage
	Thinking   string
	Done       bool
	DoneReason string
	Error      string
}

// Chat streams POST /api/chat. The response is NDJSON: every event carries a
// message delta in message.content; onContent (nil allowed) receives each
// non-empty delta in arrival order until the done:true event, which ends the
// call normally.
//
// Like Pull, Chat is a long operation and goes through the stream client
// (no request timeout): the caller's context is the only deadline, so the UI
// can cancel mid-generation. Mirroring pull streams, an {"error": ...} line
// inside the stream (HTTP 200) is the in-band error channel.
//
// Streams are bounded (phase 5): a single NDJSON event larger than 4 MiB, or
// cumulative content+thinking beyond 16 MiB, aborts the call, and a body that
// delivers no bytes for the 90s idle window times out (no total request
// deadline, so long generations with steady deltas keep running).
func (c *Client) Chat(ctx context.Context, req ChatRequest, onContent func(string)) error {
	return c.ChatStream(ctx, req, func(ev ChatEvent) {
		if ev.Message.Content != "" && onContent != nil {
			onContent(ev.Message.Content)
		}
	})
}

// ChatStream streams and decodes POST /api/chat. The callback runs in arrival
// order for every NDJSON event, including the terminal done event. It is the
// caller's responsibility to keep the callback non-blocking when ChatStream
// is running in a producer goroutine.
func (c *Client) ChatStream(ctx context.Context, req ChatRequest, onEvent func(ChatEvent)) error {
	const path = "/api/chat"
	if req.Model == "" {
		return fmt.Errorf("ollama POST %s: empty model name", path)
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("ollama POST %s: empty message list", path)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("ollama POST %s: encode request: %w", path, err)
	}
	resp, cancel, err := c.postStream(ctx, path, body)
	if err != nil {
		return err
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		return apiError(http.MethodPost, path, resp.StatusCode, raw)
	}

	// Decode through the shared NDJSON stream decoder: a single event larger
	// than 4 MiB, or cumulative content+thinking over 16 MiB, aborts the
	// stream; a body silent for the idle window times out (no total request
	// deadline, so long generations with steady deltas keep running).
	dec := newNDJSONStream(resp.Body, cancel, c.streamIdle, path)
	var contentBytes int64
	for {
		raw, err := dec.next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("ollama POST %s: stream ended without done", path)
		}
		if err != nil {
			return err
		}
		var wire struct {
			Message struct {
				Role      Role       `json:"role"`
				Content   string     `json:"content"`
				Thinking  string     `json:"thinking"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
			Thinking   string `json:"thinking"`
			Error      string `json:"error"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			return fmt.Errorf("ollama POST %s: decode stream: %w", path, err)
		}
		if wire.Error != "" {
			return fmt.Errorf("ollama POST %s: %s", path, wire.Error)
		}
		contentBytes += int64(len(wire.Message.Content) + len(wire.Message.Thinking) + len(wire.Thinking))
		if contentBytes > maxChatStreamBytes {
			return fmt.Errorf("ollama POST %s: %w", path, errChatStreamTooLarge)
		}
		ev := ChatEvent{
			Message: ChatMessage{
				Role:      wire.Message.Role,
				Content:   wire.Message.Content,
				Thinking:  wire.Message.Thinking,
				ToolCalls: wire.Message.ToolCalls,
			},
			Thinking:   wire.Thinking,
			Done:       wire.Done,
			DoneReason: wire.DoneReason,
		}
		if onEvent != nil {
			onEvent(ev)
		}
		if ev.Done {
			return nil
		}
	}
}
