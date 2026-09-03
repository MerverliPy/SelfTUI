package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"selftui/internal/ollama"
)

const DefaultMaxIterations = 12

// Msg is an event consumed by the UI. Concrete messages intentionally contain
// presentation-neutral data so the agent loop can be tested without Bubble Tea.
type Msg interface{}

type TokenMsg struct{ Text string }
type ToolStartMsg struct {
	Name  string
	Input string
}
type ToolResultMsg struct {
	Name    string
	OK      bool
	Summary string
}
type FallbackMsg struct{ Reason string }
type AgentDoneMsg struct{ Err string }

// Request is one agent turn. Messages should contain the current conversation
// but not the system prompt; Runner adds that exactly once.
type Request struct {
	Model        string
	Messages     []ollama.ChatMessage
	SystemPrompt string
	Temperature  float64
	TopP         float64
	NumCtx       int
}

type Runner struct {
	client        *ollama.Client
	workspaceRoot string
	systemPrompt  string
	maxIterations int
}

func NewRunner(client *ollama.Client, workspaceRoot, systemPrompt string, maxIterations int) *Runner {
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}
	return &Runner{
		client: client, workspaceRoot: workspaceRoot,
		systemPrompt: systemPrompt, maxIterations: maxIterations,
	}
}

// Run executes only the read-only tool loop. The first tools request doubles
// as a capability probe: models that reject tools with HTTP 400 are explicitly
// downgraded to plain chat instead of silently pretending to be an agent.
// AgentDoneMsg is emitted exactly once, after the loop or any error.
func (r *Runner) Run(ctx context.Context, req Request, emit func(Msg)) error {
	if emit == nil {
		emit = func(Msg) {}
	}
	err := r.run(ctx, req, emit)
	done := AgentDoneMsg{}
	if err != nil {
		done.Err = err.Error()
	}
	emit(done)
	return err
}

func (r *Runner) run(ctx context.Context, req Request, emit func(Msg)) error {
	if r == nil || r.client == nil {
		return errors.New("agent: nil Ollama client")
	}
	if req.Model == "" {
		return errors.New("agent: empty model name")
	}
	if len(req.Messages) == 0 {
		return errors.New("agent: empty message list")
	}
	messages := make([]ollama.ChatMessage, 0, len(req.Messages)+1)
	prompt := req.SystemPrompt
	if prompt == "" {
		prompt = r.systemPrompt
	}
	if prompt != "" {
		messages = append(messages, ollama.ChatMessage{Role: ollama.RoleSystem, Content: prompt})
	}
	messages = append(messages, req.Messages...)
	options := &ollama.ChatOptions{Temperature: req.Temperature, TopP: req.TopP, NumCtx: req.NumCtx}
	tools := ReadOnlyTools()

	for iteration := 0; iteration < r.maxIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var content strings.Builder
		var calls []ollama.ToolCall
		streamedLen := 0
		err := r.client.ChatStream(ctx, ollama.ChatRequest{
			Model: req.Model, Messages: messages, Stream: true, Tools: tools, Options: options,
		}, func(ev ollama.ChatEvent) {
			// Thinking is intentionally consumed and discarded. It is neither
			// user-visible output nor a valid tool-call transport.
			if ev.Message.Content != "" {
				content.WriteString(ev.Message.Content)
				// Content-embedded tool JSON is held back so it never flashes
				// into the transcript. Ordinary prose remains genuinely
				// streaming, preserving the M2 interaction while the runner
				// waits for the terminal tool-call decision.
				if !looksLikeEmbeddedJSON(content.String()) {
					emit(TokenMsg{Text: ev.Message.Content})
					streamedLen = content.Len()
				}
			}
			calls = mergeToolCalls(calls, ev.Message.ToolCalls)
		})
		if err != nil {
			if iteration == 0 && isToolUnsupported(err) {
				emit(FallbackMsg{Reason: "model does not support tools; using plain chat"})
				return r.runPlainChat(ctx, req, messages, options, emit)
			}
			return err
		}

		if len(calls) == 0 {
			calls = parseEmbeddedToolCalls(content.String())
		}
		if len(calls) == 0 {
			if iteration == 0 {
				emit(FallbackMsg{Reason: "model returned no tool call; showing plain chat response"})
			}
			if streamedLen < content.Len() {
				emit(TokenMsg{Text: content.String()[streamedLen:]})
			}
			return nil
		}

		// Preserve the assistant tool-call turn in the next request, but never
		// emit embedded JSON as final text.
		messages = append(messages, ollama.ChatMessage{
			Role: ollama.RoleAssistant, Content: "", ToolCalls: calls,
		})
		for _, call := range calls {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := call.Function.Name
			input := string(call.Function.Arguments)
			emit(ToolStartMsg{Name: name, Input: input})
			result, toolErr := executeTool(r.workspaceRoot, call)
			if toolErr != nil {
				emit(ToolResultMsg{Name: name, OK: false, Summary: toolErr.Error()})
				messages = append(messages, ollama.ChatMessage{
					Role: ollama.RoleTool, ToolName: name,
					Content: "error: " + toolErr.Error(),
				})
				continue
			}
			emit(ToolResultMsg{Name: name, OK: true, Summary: result})
			messages = append(messages, ollama.ChatMessage{
				Role: ollama.RoleTool, ToolName: name, Content: result,
			})
		}
	}
	return fmt.Errorf("agent: maximum tool iterations (%d) reached", r.maxIterations)
}

func (r *Runner) runPlainChat(ctx context.Context, req Request, messages []ollama.ChatMessage, options *ollama.ChatOptions, emit func(Msg)) error {
	return r.client.ChatStream(ctx, ollama.ChatRequest{
		Model: req.Model, Messages: messages, Stream: true, Options: options,
	}, func(ev ollama.ChatEvent) {
		if ev.Message.Content != "" {
			emit(TokenMsg{Text: ev.Message.Content})
		}
	})
}

func executeTool(root string, call ollama.ToolCall) (string, error) {
	switch call.Function.Name {
	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(call.Function.Arguments, &args); err != nil {
			return "", fmt.Errorf("read_file: %w", err)
		}
		if args.Path == "" {
			return "", errors.New("read_file: path is required")
		}
		return ReadFile(root, args.Path)
	case "list_dir":
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(call.Function.Arguments, &args); err != nil {
			return "", fmt.Errorf("list_dir: %w", err)
		}
		if args.Path == "" {
			return "", errors.New("list_dir: path is required")
		}
		return ListDir(root, args.Path)
	case "grep":
		var args struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if err := decodeArgs(call.Function.Arguments, &args); err != nil {
			return "", fmt.Errorf("grep: %w", err)
		}
		if args.Pattern == "" || args.Path == "" {
			return "", errors.New("grep: pattern and path are required")
		}
		return Grep(root, args.Pattern, args.Path)
	default:
		return "", fmt.Errorf("tool %q is not allowed in read-only agent", call.Function.Name)
	}
}

func looksLikeEmbeddedJSON(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "```")
}

func mergeToolCalls(existing, incoming []ollama.ToolCall) []ollama.ToolCall {
	for i, candidate := range incoming {
		if i >= len(existing) {
			existing = append(existing, candidate)
			continue
		}
		current := &existing[i]
		if current.Function.Name != candidate.Function.Name {
			existing = append(existing, candidate)
			continue
		}
		if string(current.Function.Arguments) == string(candidate.Function.Arguments) {
			continue // the stream repeated a complete call
		}
		if json.Valid(current.Function.Arguments) && json.Valid(candidate.Function.Arguments) {
			// Distinct complete calls may be emitted in separate events.
			existing = append(existing, candidate)
			continue
		}
		current.Function.Arguments = mergeArguments(current.Function.Arguments, candidate.Function.Arguments)
	}
	return existing
}

func mergeArguments(current, next json.RawMessage) json.RawMessage {
	if len(current) == 0 || string(current) == "null" {
		return next
	}
	if len(next) == 0 || string(next) == "null" {
		return current
	}
	if json.Valid(current) && json.Valid(next) {
		// Complete native calls are sometimes repeated with the same call
		// metadata; differing complete values represent a later call, not a
		// fragment. The positional merger has already retained the first.
		return current
	}
	return append(append(json.RawMessage(nil), current...), next...)
}

func isToolUnsupported(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "http 400") || strings.Contains(text, "does not support tools") || strings.Contains(text, "tool calling is not supported")
}

func parseEmbeddedToolCalls(text string) []ollama.ToolCall {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "```json"))
		text = strings.TrimSpace(strings.TrimSuffix(text, "```"))
	}
	var raw any
	if json.Unmarshal([]byte(text), &raw) != nil {
		return nil
	}
	return embeddedCalls(raw)
}

func embeddedCalls(raw any) []ollama.ToolCall {
	if list, ok := raw.([]any); ok {
		var calls []ollama.ToolCall
		for _, item := range list {
			if call := embeddedCall(item); call.Function.Name != "" {
				calls = append(calls, call)
			}
		}
		return calls
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if list, ok := obj["tool_calls"].([]any); ok {
		var calls []ollama.ToolCall
		for _, item := range list {
			if call := embeddedCall(item); call.Function.Name != "" {
				calls = append(calls, call)
			}
		}
		return calls
	}
	if call := embeddedCall(obj); call.Function.Name != "" {
		return []ollama.ToolCall{call}
	}
	return nil
}

func embeddedCall(raw any) ollama.ToolCall {
	obj, ok := raw.(map[string]any)
	if !ok {
		return ollama.ToolCall{}
	}
	name, _ := obj["name"].(string)
	if name == "" {
		name, _ = obj["tool"].(string)
	}
	if name == "" {
		if fn, ok := obj["function"].(map[string]any); ok {
			name, _ = fn["name"].(string)
			obj = fn
		}
	}
	args := obj["arguments"]
	if args == nil {
		args = obj["args"]
	}
	if args == nil {
		args = obj["parameters"]
	}
	b, _ := json.Marshal(args)
	if encoded, ok := args.(string); ok && strings.HasPrefix(strings.TrimSpace(encoded), "{") {
		b = json.RawMessage(encoded)
	}
	return ollama.ToolCall{Function: ollama.ToolCallFunction{Name: name, Arguments: b}}
}
