package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"selftui/internal/ollama"
)

const DefaultMaxIterations = 12

// Approval-window bounds for the per-call mutation confirmation (write_file,
// edit_file). The v0.1 executor removal left only these approval mechanics;
// command execution itself no longer ships (see docs/run-command-containment.md).
const (
	defaultConfirmTimeout = 30 * time.Second
	maxConfirmTimeout     = 60 * time.Second
)

// H-03 execution budgets: one tool call's decoded arguments may not exceed
// maxToolArgBytes, and one Run may execute at most maxToolCallsPerRun calls
// across every model iteration. A batch that would cross the call ceiling is
// rejected in full before any of its calls execute, so a misbehaving or
// malicious endpoint cannot force unbounded per-iteration batches or
// unbounded fragment accumulation (see the external audit H-03).
const (
	maxToolArgBytes    = 1 << 20 // 1 MiB per decoded tool-call argument
	maxToolCallsPerRun = 64
)

// H-03 boundary errors (agent side). The ollama package owns the 16 MiB
// cumulative raw chat-stream ceiling and the 4 MiB per-event wire cap; these
// three errors gate per-call argument size, per-run call count, and ambiguous
// fragment merging. Callers and tests match on the stable text.
var (
	errToolArgumentTooLarge = errors.New("tool call argument exceeds 1048576 bytes")
	errToolCallLimit        = errors.New("tool call limit (64) exceeded for this run")
	errAmbiguousToolStream  = errors.New("ambiguous tool call fragments: no stable call id or index")
)

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

// ToolConfirmMsg pauses a mutation until the user explicitly responds. The
// reply channel is deliberately private so only Respond can release it.
type ToolConfirmMsg struct {
	Name      string
	Input     string
	Workspace string
	Timeout   time.Duration
	reply     chan bool
}

func (m ToolConfirmMsg) Respond(approved bool) {
	select {
	case m.reply <- approved:
	default:
	}
}

type FallbackMsg struct{ Reason string }

// AgentDoneMsg is emitted exactly once, after the loop or any error. Reason
// is the terminal Ollama done_reason of the final stream ("stop", "length",
// "tool_calls") so the UI can show why the turn ended (M7-B); it stays empty
// on errors where no terminal event arrived.
type AgentDoneMsg struct {
	Err    string
	Reason string
}

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

	// policy is nil when tools are disabled (the NewRunner compatibility
	// constructor): the runner then never sends tool definitions and behaves
	// as plain chat. NewRunnerWithPolicy arms the five v0.1 tools and gates
	// every requested path with the policy before a tool executes.
	policy *ToolPolicy
}

// NewRunner builds a runner with workspace tools DISABLED. It remains the
// compatibility constructor for callers that predate the tool-trust policy;
// the disabled default is deliberate — tools only exist when the user
// explicitly enables them (config tools_enabled) and the code passes a real
// policy through NewRunnerWithPolicy.
func NewRunner(client *ollama.Client, workspaceRoot, systemPrompt string, maxIterations int) *Runner {
	return NewRunnerWithPolicy(client, workspaceRoot, systemPrompt, maxIterations, nil)
}

// NewRunnerWithPolicy builds the runner used when workspace tools are
// enabled. policy must be non-nil: it decides which tools the model may
// request (Tools) and which paths those tools may touch (AuthorizePath). A
// nil policy here is equivalent to NewRunner (plain chat).
func NewRunnerWithPolicy(client *ollama.Client, workspaceRoot, systemPrompt string, maxIterations int, policy *ToolPolicy) *Runner {
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}
	return &Runner{
		client: client, workspaceRoot: workspaceRoot,
		systemPrompt: systemPrompt, maxIterations: maxIterations,
		policy: policy,
	}
}

// Run executes one agent turn. With tools disabled (NewRunner, no policy) it
// streams plain chat — no tools field reaches the wire. With tools armed
// (NewRunnerWithPolicy) it runs the tool loop (read-only first, then the
// confirmed mutation set) with the plain-chat fallback. The first tools
// request doubles as a capability probe: models that reject tools with HTTP
// 400 are explicitly downgraded to plain chat instead of silently pretending
// to be an agent. AgentDoneMsg is emitted exactly once, after the loop or
// any error, carrying the terminal done_reason of the final stream when one
// arrived (M7-B).
func (r *Runner) Run(ctx context.Context, req Request, emit func(Msg)) error {
	if emit == nil {
		emit = func(Msg) {}
	}
	var reason string
	err := r.run(ctx, req, emit, &reason)
	done := AgentDoneMsg{Reason: reason}
	if err != nil {
		done.Err = err.Error()
	}
	emit(done)
	return err
}

func (r *Runner) run(ctx context.Context, req Request, emit func(Msg), reasonOut *string) error {
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

	// Tools disabled (no policy): plain chat only. The request must not even
	// carry a tools field, and nothing the model says — including an
	// unsolicited tool_calls-shaped reply — may turn into an execution.
	tools := r.tools()
	if len(tools) == 0 {
		return r.runPlainChat(ctx, req, messages, req.NumCtx, options, emit, reasonOut)
	}

	// H-03 run-wide call budget: executedCalls counts every tool executed
	// across all iterations and is checked per batch before any call runs.
	executedCalls := 0

	for iteration := 0; iteration < r.maxIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var content strings.Builder
		var calls []ollama.ToolCall
		var mergeErr error
		streamedLen := 0
		var lastReason string
		err := r.client.ChatStream(ctx, ollama.ChatRequest{
			Model: req.Model, Messages: BudgetMessages(messages, req.NumCtx), Stream: true, Tools: tools, Options: options,
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
			if ev.Done && ev.DoneReason != "" {
				lastReason = ev.DoneReason
			}
			if mergeErr != nil {
				return // the stream already failed to merge: stop accumulating
			}
			calls, mergeErr = mergeToolCalls(calls, ev.Message.ToolCalls)
		})
		if err != nil {
			if iteration == 0 && isToolUnsupported(err) {
				emit(FallbackMsg{Reason: "model does not support tools; using plain chat"})
				return r.runPlainChat(ctx, req, messages, req.NumCtx, options, emit, reasonOut)
			}
			return err
		}
		if mergeErr != nil {
			return fmt.Errorf("agent: %w", mergeErr)
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
			*reasonOut = lastReason
			return nil
		}

		// H-03 batch gate: a batch that violates the per-call argument
		// ceiling, or that would cross the run-wide call budget, is rejected
		// in full — no call in it executes, and nothing from it reaches the
		// confirmation dialogs or the transcript.
		for _, call := range calls {
			if len(call.Function.Arguments) > maxToolArgBytes {
				return fmt.Errorf("agent: %w", errToolArgumentTooLarge)
			}
		}
		if executedCalls+len(calls) > maxToolCallsPerRun {
			return fmt.Errorf("agent: %w", errToolCallLimit)
		}
		executedCalls += len(calls)

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
			result, toolErr := r.executeTool(ctx, call, emit)
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

// runPlainChat streams one tools-free request. It is the whole loop when
// tools are disabled and the fallback when an enabled model rejects the tool
// surface. The terminal done_reason is recorded so the plain-chat footer
// shows why the turn ended, mirroring the tool loop.
func (r *Runner) runPlainChat(ctx context.Context, req Request, messages []ollama.ChatMessage, numCtx int, options *ollama.ChatOptions, emit func(Msg), reasonOut *string) error {
	return r.client.ChatStream(ctx, ollama.ChatRequest{
		// The plain-chat fallback must honor the same context budget as the
		// tool loop: a giant first message is truncated, never sent raw
		// (M6 context-truncation edge).
		Model: req.Model, Messages: BudgetMessages(messages, numCtx), Stream: true, Options: options,
	}, func(ev ollama.ChatEvent) {
		if ev.Message.Content != "" {
			emit(TokenMsg{Text: ev.Message.Content})
		}
		if ev.Done && ev.DoneReason != "" && reasonOut != nil {
			*reasonOut = ev.DoneReason
		}
	})
}

// tools returns the definitions to advertise: the policy's five v0.1 tools
// when one is set, nothing otherwise.
func (r *Runner) tools() []ollama.ToolDefinition {
	if r == nil || r.policy == nil {
		return nil
	}
	return r.policy.Tools()
}

// authorizePath applies the policy's path rules when tools are armed; a nil
// policy (disabled runner) has no rules to apply. The check happens before a
// tool's own validation, confirmation, or executor so a sensitive request
// never surfaces an approval dialog.
func (r *Runner) authorizePath(path string) error {
	if r.policy == nil {
		return nil
	}
	return r.policy.AuthorizePath(path)
}

func (r *Runner) executeTool(ctx context.Context, call ollama.ToolCall, emit func(Msg)) (string, error) {
	root := r.workspaceRoot
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
		if err := r.authorizePath(args.Path); err != nil {
			return "", err
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
		if err := r.authorizePath(args.Path); err != nil {
			return "", err
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
		if err := r.authorizePath(args.Path); err != nil {
			return "", err
		}
		// The top-level check above gates the requested path; Grep itself
		// re-authorizes every workspace-relative descendant it would open so
		// recursive roots (e.g. ".") cannot read denied files (C-01).
		return Grep(ctx, root, args.Pattern, args.Path, r.authorizePath)
	case "write_file":
		var args struct {
			Path      string `json:"path"`
			Content   string `json:"content"`
			Overwrite bool   `json:"overwrite"`
		}
		if err := decodeArgs(call.Function.Arguments, &args); err != nil {
			return "", fmt.Errorf("write_file: %w", err)
		}
		if args.Path == "" {
			return "", errors.New("write_file: path is required")
		}
		if err := r.authorizePath(args.Path); err != nil {
			return "", err
		}
		if err := r.confirm(ctx, call, 0, emit); err != nil {
			return "", err
		}
		if err := WriteFile(root, args.Path, args.Content, args.Overwrite); err != nil {
			return "", err
		}
		return "wrote " + args.Path, nil
	case "edit_file":
		var args struct {
			Path string `json:"path"`
			Old  string `json:"old"`
			New  string `json:"new"`
		}
		if err := decodeArgs(call.Function.Arguments, &args); err != nil {
			return "", fmt.Errorf("edit_file: %w", err)
		}
		if args.Path == "" || args.Old == "" {
			return "", errors.New("edit_file: path and old are required")
		}
		if err := r.authorizePath(args.Path); err != nil {
			return "", err
		}
		if err := r.confirm(ctx, call, 0, emit); err != nil {
			return "", err
		}
		if err := EditFile(root, args.Path, args.Old, args.New); err != nil {
			return "", err
		}
		return "edited " + args.Path, nil
	default:
		// The boundary is a closed set: any other name — including the
		// pre-v0.1 run_command executor, which does not ship — is rejected.
		return "", fmt.Errorf("tool %q is not allowed", call.Function.Name)
	}
}

func (r *Runner) confirm(ctx context.Context, call ollama.ToolCall, seconds int, emit func(Msg)) error {
	timeout := defaultConfirmTimeout
	if seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	if timeout > maxConfirmTimeout {
		return fmt.Errorf("%s: timeout exceeds %s", call.Function.Name, maxConfirmTimeout)
	}
	msg := ToolConfirmMsg{Name: call.Function.Name, Input: string(call.Function.Arguments), Workspace: r.workspaceRoot, Timeout: timeout, reply: make(chan bool, 1)}
	emit(msg)
	select {
	case approved := <-msg.reply:
		if !approved {
			return fmt.Errorf("%s: not approved", call.Function.Name)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func looksLikeEmbeddedJSON(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "```")
}

// mergeToolCalls folds the tool_calls of one stream event into the calls
// accumulated so far for this iteration (H-03). Fragments are concatenated
// only while both sides are clearly incomplete JSON; any mixture of a
// complete call and a fragment at the same slot is ambiguous — the wire type
// carries no stable per-call id/index to tell a retransmission from a second
// call — so the merge refuses instead of corrupting the arguments. Repeated
// complete calls deduplicate; distinct complete calls at one slot (parallel
// calls) both survive. Every call and every fragment accumulation is bounded
// by maxToolArgBytes, keeping merging linear and memory bounded.
func mergeToolCalls(existing, incoming []ollama.ToolCall) ([]ollama.ToolCall, error) {
	for i, candidate := range incoming {
		if len(candidate.Function.Arguments) > maxToolArgBytes {
			return nil, errToolArgumentTooLarge
		}
		if i >= len(existing) {
			existing = append(existing, candidate)
			continue
		}
		current := &existing[i]
		if current.Function.Name != candidate.Function.Name {
			// A different tool at this slot is a new call, not a fragment of
			// the previous one.
			existing = append(existing, candidate)
			continue
		}
		cur, cand := current.Function.Arguments, candidate.Function.Arguments
		if len(cand) == 0 || string(cand) == "null" {
			continue // the event carried no argument text for this call
		}
		if len(cur) == 0 || string(cur) == "null" {
			current.Function.Arguments = append(current.Function.Arguments[:0], cand...)
			continue
		}
		if string(cur) == string(cand) {
			continue // the stream repeated a complete call (or an identical fragment)
		}
		switch {
		case json.Valid(cur) && json.Valid(cand):
			// Two distinct complete calls delivered at the same slot: the
			// first stays in place, the later one becomes a new call.
			existing = append(existing, candidate)
		case json.Valid(cur) || json.Valid(cand):
			// One side complete, the other a fragment: without a stable
			// id/index a blind concatenation would corrupt the arguments, and
			// guessing which side is authoritative can misassociate calls.
			return nil, errAmbiguousToolStream
		default:
			// Both fragments of the same call: bounded linear accumulation.
			if len(cur)+len(cand) > maxToolArgBytes {
				return nil, errToolArgumentTooLarge
			}
			current.Function.Arguments = append(append(json.RawMessage(nil), cur...), cand...)
		}
	}
	return existing, nil
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
