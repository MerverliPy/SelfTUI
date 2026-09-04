package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"selftui/internal/ollama"
)

func toolEvent(call string) string {
	return fmt.Sprintf(`{"message":{"role":"assistant","tool_calls":[%s]},"done":true,"done_reason":"tool_calls"}`+"\n", call)
}

func nativeCall(name string, args string) string {
	return fmt.Sprintf(`{"function":{"name":%q,"arguments":%s}}`, name, args)
}

func finalEvent(text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"message":{"role":"assistant","content":%s},"done":true,"done_reason":"stop"}`+"\n", b)
}

func TestReadOnlyToolsStayInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello agent\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadFile(root, "hello.txt"); err != nil || got != "hello agent\nsecond line\n" {
		t.Errorf("ReadFile = %q, %v", got, err)
	}
	if got, err := ListDir(root, "."); err != nil || !strings.Contains(got, "hello.txt") {
		t.Errorf("ListDir = %q, %v", got, err)
	}
	if got, err := Grep(context.Background(), root, "agent", ".", func(string) error { return nil }); err != nil || !strings.Contains(got, "hello.txt:1:hello agent") {
		t.Errorf("Grep = %q, %v", got, err)
	}
	for _, path := range []string{"../outside", filepath.Join(root, "..", "outside")} {
		if _, err := ReadFile(root, path); err == nil {
			t.Errorf("ReadFile(%q) unexpectedly succeeded", path)
		}
	}
}

func TestReadOnlyToolsRejectSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ReadFile(root, "link"); err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("symlink read error = %v, want workspace escape", err)
	}
}

func TestRunnerExecutesNativeToolAndStreamsFinal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# hello\nagent-readable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var secondMessages []ollama.ChatMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if calls == 1 {
			if len(req.Tools) != 5 {
				t.Errorf("tools = %d, want 5 (read/list/grep/write/edit; no run_command in v0.1)", len(req.Tools))
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			io.WriteString(w, toolEvent(nativeCall("read_file", `{"path":"README.md"}`)))
			return
		}
		secondMessages = req.Messages
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, finalEvent("I read the file."))
	}))
	t.Cleanup(srv.Close)

	var events []Msg
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "system", 4, &ToolPolicy{})
	err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "read README.md"}},
	}, func(msg Msg) { events = append(events, msg) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Fatalf("chat calls = %d, want 2", calls)
	}
	if len(secondMessages) < 3 || secondMessages[len(secondMessages)-1].Role != ollama.RoleTool {
		t.Fatalf("second messages = %+v, want tool result last", secondMessages)
	}
	if !strings.Contains(secondMessages[len(secondMessages)-1].Content, "agent-readable") {
		t.Errorf("tool result = %q", secondMessages[len(secondMessages)-1].Content)
	}
	var got string
	for _, event := range events {
		switch msg := event.(type) {
		case ToolStartMsg:
			if msg.Name != "read_file" {
				t.Errorf("tool start = %+v", msg)
			}
		case ToolResultMsg:
			if !msg.OK || !strings.Contains(msg.Summary, "agent-readable") {
				t.Errorf("tool result = %+v", msg)
			}
		case TokenMsg:
			got += msg.Text
		}
	}
	if got != "I read the file." {
		t.Errorf("final tokens = %q", got)
	}
}

func TestMergeToolCallsAssemblesStreamedArguments(t *testing.T) {
	calls := mergeToolCalls(nil, []ollama.ToolCall{{Function: ollama.ToolCallFunction{Name: "read_file", Arguments: json.RawMessage(`{"path":"`)}}})
	calls = mergeToolCalls(calls, []ollama.ToolCall{{Function: ollama.ToolCallFunction{Name: "read_file", Arguments: json.RawMessage(`README.md"}`)}}})
	if len(calls) != 1 || string(calls[0].Function.Arguments) != `{"path":"README.md"}` {
		t.Errorf("merged calls = %+v, want one complete argument object", calls)
	}
}

func TestRunnerParsesContentEmbeddedToolJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/x-ndjson")
		if calls == 1 {
			io.WriteString(w, finalEvent(`{"name":"grep","arguments":{"pattern":"needle","path":"x.txt"}}`))
			return
		}
		io.WriteString(w, finalEvent("found it"))
	}))
	t.Cleanup(srv.Close)
	var got string
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{})
	if err := r.Run(context.Background(), Request{Model: "coder", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "find needle"}}}, func(msg Msg) {
		if token, ok := msg.(TokenMsg); ok {
			got += token.Text
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 || got != "found it" {
		t.Errorf("calls=%d final=%q, want 2 and found it", calls, got)
	}
}

func TestRunnerExplicitPlainChatFallbackOnToolRejection(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"model does not support tools"}`)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, finalEvent("plain answer"))
	}))
	t.Cleanup(srv.Close)
	var fallback bool
	var got string
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), t.TempDir(), "", 2, &ToolPolicy{})
	if err := r.Run(context.Background(), Request{Model: "gemma3:12b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}}}, func(msg Msg) {
		switch event := msg.(type) {
		case FallbackMsg:
			fallback = strings.Contains(event.Reason, "plain chat")
		case TokenMsg:
			got += event.Text
		}
	}); err != nil {
		t.Fatalf("Run fallback: %v", err)
	}
	if calls != 2 || !fallback || got != "plain answer" {
		t.Errorf("calls=%d fallback=%v text=%q", calls, fallback, got)
	}
}

func TestRunnerBoundsIterations(t *testing.T) {
	root := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, toolEvent(nativeCall("list_dir", `{"path":"."}`)))
	}))
	t.Cleanup(srv.Close)
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 2, &ToolPolicy{})
	err := r.Run(context.Background(), Request{Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "list"}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "maximum tool iterations (2)") {
		t.Errorf("error = %v, want bounded iteration error", err)
	}
}

func TestRunnerReportsTerminalDoneReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		reason string
	}{
		{"stop", finalEvent("done by stop"), "stop"},
		{"length", `{"message":{"role":"assistant","content":"cut short"},"done":true,"done_reason":"length"}` + "\n", "length"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)
			var done AgentDoneMsg
			r := NewRunner(ollama.New(srv.URL, ""), t.TempDir(), "", 3)
			if err := r.Run(context.Background(), Request{
				Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}},
			}, func(msg Msg) {
				if d, ok := msg.(AgentDoneMsg); ok {
					done = d
				}
			}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if done.Reason != tc.reason {
				t.Errorf("AgentDoneMsg.Reason = %q, want %q", done.Reason, tc.reason)
			}
			if done.Err != "" {
				t.Errorf("AgentDoneMsg.Err = %q, want empty", done.Err)
			}
		})
	}
}

// TestRunnerGrepOverRootHidesPolicyDeniedDescendants is the C-01 regression
// driven through the real runner: a native grep tool call with path "." over
// a workspace seeded with every denied sensitive class must return only
// ordinary matches plus the .env.example template. Before the fix, recursive
// grep authorized only the requested root, so policy-denied descendants
// (their content and their paths) leaked into the tool result.
func TestRunnerGrepOverRootHidesPolicyDeniedDescendants(t *testing.T) {
	root := seedSensitiveGrepWorkspace(t)
	calls := 0
	var summary string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/x-ndjson")
		if calls == 1 {
			io.WriteString(w, toolEvent(nativeCall("grep", `{"pattern":"GOOD_|LEAK_","path":"."}`)))
			return
		}
		io.WriteString(w, finalEvent("searched"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{})
	if err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "find markers"}},
	}, func(msg Msg) {
		if result, ok := msg.(ToolResultMsg); ok && result.Name == "grep" && result.OK {
			summary = result.Summary
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary == "" {
		t.Fatalf("grep tool produced no successful result (calls=%d)", calls)
	}
	assertGrepLeakFree(t, summary)
}

func TestRunnerCancellationReturnsPromptly(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		// Drain the request body before parking: only after the body reaches
		// EOF does net/http arm its client-close-detection background read, so
		// a handler that never reads r.Body can park forever once the client
		// disconnects (rare race -> 10m package timeout in CI under 2 vCPU).
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRunner(ollama.New(srv.URL, ""), t.TempDir(), "", 2)
	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx, Request{Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "wait"}}}, nil)
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("request did not start")
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}
