package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeChatServer serves POST /api/chat. The returned request pointer holds
// the decoded request body after the handler runs; body is what the server
// streams back to the client.
func fakeChatServer(t *testing.T, body string) (*Client, *ChatRequest) {
	t.Helper()
	captured := new(ChatRequest)
	reqBody, _ := json.Marshal(*captured) // silence unused-variable lint if any
	_ = reqBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("got %s %s, want POST /api/chat", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(captured); err != nil {
			t.Fatalf("decode chat request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, ""), captured
}

// chatStream samples a two-line stream: one content delta, then done.
const chatStream = `{"model":"qwen3:8b","created_at":"2026-09-04T00:00:00Z","message":{"role":"assistant","content":"Hi there"},"done":false}
{"model":"qwen3:8b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
`

func TestChatSendsRequest(t *testing.T) {
	got := ""
	c, _ := fakeChatServer(t, chatStream)
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hello"}},
		Stream:   true,
		Options:  &ChatOptions{Temperature: 0.7, TopP: 0.9, NumCtx: 4096},
	}, func(s string) { got += s })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "Hi there" {
		t.Errorf("content = %q, want %q (done event carries no content)", got, "Hi there")
	}
}

func TestChatRequestPayload(t *testing.T) {
	var req *ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body struct {
			Model    string        `json:"model"`
			Messages []ChatMessage `json:"messages"`
			Stream   bool          `json:"stream"`
			Options  *ChatOptions  `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		req = &ChatRequest{Model: body.Model, Messages: body.Messages, Stream: body.Stream, Options: body.Options}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, chatStream)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hello"}, {Role: RoleAssistant, Content: "hi"}},
		Stream:   true,
		Options:  &ChatOptions{Temperature: 0.7, TopP: 0.9, NumCtx: 4096},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if req == nil {
		t.Fatal("server saw no request")
	}
	if req.Model != "qwen3:8b" || !req.Stream {
		t.Errorf("model=%q stream=%v, want qwen3:8b stream=true", req.Model, req.Stream)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != RoleUser || req.Messages[1].Role != RoleAssistant {
		t.Errorf("messages = %+v", req.Messages)
	}
	if req.Options.Temperature != 0.7 || req.Options.TopP != 0.9 || req.Options.NumCtx != 4096 {
		t.Errorf("options = %+v", req.Options)
	}
}

func TestChatStreamsDeltas(t *testing.T) {
	// Deliberately split a word across events so the test proves deltas are
	// appended in order, not replaced.
	c, _ := fakeChatServer(t, `{"message":{"role":"assistant","content":"He"},"done":false}
{"message":{"role":"assistant","content":"llo "},"done":false}
{"message":{"role":"assistant","content":"world!"},"done":false}
{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
`)
	var got []string
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, func(s string) { got = append(got, s) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("onContent called %d times, want 3: %v", len(got), got)
	}
	if strings.Join(got, "") != "Hello world!" {
		t.Errorf("joined = %q, want %q", strings.Join(got, ""), "Hello world!")
	}
}

func TestChatInbandError(t *testing.T) {
	c, _ := fakeChatServer(t, `{"message":{"role":"assistant","content":"hi"},"done":false}
{"error":"model 'nope' not found"}
`)
	calls := 0
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "nope",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, func(string) { calls++ })
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "model 'nope' not found") {
		t.Errorf("error = %v, want surfaced in-band API message", err)
	}
	if calls != 1 {
		t.Errorf("onContent called %d times, want 1 (before the error line)", calls)
	}
}

func TestChatHTTPError(t *testing.T) {
	// Non-2xx response with an Ollama error payload (model not found).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"model 'ghost:tag' not found"}`)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "ghost:tag",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "model 'ghost:tag' not found") {
		t.Errorf("error = %v, want server error message", err)
	}
}

func TestChatEmptyModelRejected(t *testing.T) {
	c, _ := fakeChatServer(t, chatStream)
	err := c.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err == nil {
		t.Fatal("Chat with empty model: want error, got nil")
	}
}

func TestChatEmptyMessagesRejected(t *testing.T) {
	c, _ := fakeChatServer(t, chatStream)
	err := c.Chat(context.Background(), ChatRequest{Model: "qwen3:8b"}, nil)
	if err == nil {
		t.Fatal("Chat with no messages: want error, got nil")
	}
}

func TestChatBadJSONStream(t *testing.T) {
	c, _ := fakeChatServer(t, `{"message":{"role":"assistant","content":"hi"},"done":false}
this-is-not-json
`)
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err == nil {
		t.Fatal("want decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode stream") {
		t.Errorf("error = %v, want decode-stream context", err)
	}
}

func TestChatEOFWithoutDone(t *testing.T) {
	c, _ := fakeChatServer(t, `{"message":{"role":"assistant","content":"hi"},"done":false}
`)
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err == nil {
		t.Fatal("want error when the stream ends without done, got nil")
	}
	if !strings.Contains(err.Error(), "stream ended without done") {
		t.Errorf("error = %v, want ended-without-done message", err)
	}
}

func TestChatContextCancelled(t *testing.T) {
	// The server streams one delta then stalls until the client disconnects.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"message":{"role":"assistant","content":"hi"},"done":false}`+"\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- c.Chat(ctx, ChatRequest{
			Model:    "qwen3:8b",
			Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
		}, nil)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want error after cancel, got nil")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Chat did not return after context cancel")
	}
}

func TestChatStreamDecodesThinkingAndToolCalls(t *testing.T) {
	c, _ := fakeChatServer(t, `{"message":{"role":"assistant","thinking":"reason","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"README.md"}}}]},"done":true,"done_reason":"tool_calls"}`)
	var got ChatEvent
	err := c.ChatStream(context.Background(), ChatRequest{
		Model: "qwen3:8b", Messages: []ChatMessage{{Role: RoleUser, Content: "inspect"}},
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunction{Name: "read_file"}}},
	}, func(ev ChatEvent) { got = ev })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if !got.Done || got.DoneReason != "tool_calls" || got.Thinking != "" {
		t.Errorf("event = %+v, want terminal tool event", got)
	}
	if got.Message.Thinking != "reason" || len(got.Message.ToolCalls) != 1 {
		t.Errorf("message = %+v, want thinking and one tool call", got.Message)
	}
	if got.Message.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("tool = %+v", got.Message.ToolCalls[0])
	}
}

func TestChatSendsTools(t *testing.T) {
	var got []ToolDefinition
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		got = req.Tools
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"message":{"role":"assistant","content":"ok"},"done":true}`+"\n")
	}))
	t.Cleanup(srv.Close)
	if err := New(srv.URL, "").Chat(context.Background(), ChatRequest{
		Model: "qwen3:8b", Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunction{Name: "read_file", Parameters: map[string]any{"type": "object"}}}},
	}, nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(got) != 1 || got[0].Function.Name != "read_file" {
		t.Errorf("tools = %+v", got)
	}
}

func TestChatSendsBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, chatStream)
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "sekrit")
	if err := c.Chat(context.Background(), ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotAuth != "Bearer sekrit" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer sekrit")
	}
}

func TestChatOptionsOmittedWhenZero(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		raw = string(b)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, chatStream)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")

	if err := c.Chat(context.Background(), ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}, nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.Contains(raw, "options") {
		t.Errorf("zero options should be omitted from the request, got: %s", raw)
	}
}
