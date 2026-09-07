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

// multiToolEvent streams one NDJSON event whose message carries many native
// tool calls at once (the shape a hostile or pathological endpoint uses to
// request an unbounded batch in a single iteration).
func multiToolEvent(calls []string) string {
	return `{"message":{"role":"assistant","tool_calls":[` + strings.Join(calls, ",") + `]},"done":true,"done_reason":"tool_calls"}` + "\n"
}

func TestReadOnlyToolsStayInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello agent\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadFile(context.Background(), root, "hello.txt"); err != nil || got != "hello agent\nsecond line\n" {
		t.Errorf("ReadFile = %q, %v", got, err)
	}
	if got, err := ListDir(context.Background(), root, "."); err != nil || !strings.Contains(got, "hello.txt") {
		t.Errorf("ListDir = %q, %v", got, err)
	}
	if got, err := Grep(context.Background(), root, "agent", ".", func(string) error { return nil }); err != nil || !strings.Contains(got, "hello.txt:1:hello agent") {
		t.Errorf("Grep = %q, %v", got, err)
	}
	for _, path := range []string{"../outside", filepath.Join(root, "..", "outside")} {
		if _, err := ReadFile(context.Background(), root, path); err == nil {
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
	if _, err := ReadFile(context.Background(), root, "link"); err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("symlink read error = %v, want workspace escape", err)
	}
}

// TestRunnerRejectsOversizedNativeToolArgument is the H-03 regression for the
// 1 MiB decoded-argument ceiling: one native tool call whose arguments exceed
// 1 MiB must abort the run before the tool executes. Before the fix the runner
// had no per-call argument bound, so the call was executed (the giant path
// failed deep inside the filesystem layer) and the loop continued.
func TestRunnerRejectsOversizedNativeToolArgument(t *testing.T) {
	root := t.TempDir()
	requests := 0
	executed := 0
	// ~1 MiB + slack of text inside a valid JSON arguments object: the decoded
	// arguments cross the 1 MiB per-call ceiling while the single NDJSON event
	// stays far below the 4 MiB per-event wire cap.
	huge := strings.Repeat("a", (1<<20)+512)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := json.NewDecoder(r.Body).Decode(new(ollama.ChatRequest)); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		if requests == 1 {
			io.WriteString(w, toolEvent(nativeCall("read_file", fmt.Sprintf(`{"path":"%s"}`, huge))))
			return
		}
		io.WriteString(w, finalEvent("done"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 4, &ToolPolicy{})
	err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "read it"}},
	}, func(msg Msg) {
		if _, ok := msg.(ToolStartMsg); ok {
			executed++
		}
	})
	if err == nil {
		t.Fatal("want per-call argument limit error, got nil")
	}
	if !strings.Contains(err.Error(), "tool call argument exceeds 1048576 bytes") {
		t.Errorf("error = %v, want per-call argument ceiling message", err)
	}
	if executed != 0 {
		t.Errorf("tool executions = %d, want 0 (oversized call must not execute)", executed)
	}
	if requests != 1 {
		t.Errorf("chat requests = %d, want 1 (run must stop at the crossing iteration)", requests)
	}
}

// TestRunnerRejectsBatchOverCallLimit is the H-03 regression for a single
// batch that alone exceeds the 64-call per-run ceiling: none of its calls may
// execute. Before the fix the runner executed every call in the returned
// batch, bounded only by max_tool_iterations.
func TestRunnerRejectsBatchOverCallLimit(t *testing.T) {
	root := t.TempDir()
	requests := 0
	executed := 0
	calls := make([]string, 70)
	for i := range calls {
		calls[i] = nativeCall("list_dir", `{"path":"."}`)
	}
	batch := multiToolEvent(calls)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := json.NewDecoder(r.Body).Decode(new(ollama.ChatRequest)); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, batch)
	}))
	t.Cleanup(srv.Close)

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{})
	err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "list everything"}},
	}, func(msg Msg) {
		if _, ok := msg.(ToolStartMsg); ok {
			executed++
		}
	})
	if err == nil || !strings.Contains(err.Error(), "tool call limit (64)") {
		t.Errorf("error = %v, want run-wide call limit error", err)
	}
	if executed != 0 {
		t.Errorf("tool executions = %d, want 0 (crossing batch must not execute)", executed)
	}
	if requests != 1 {
		t.Errorf("chat requests = %d, want 1 (run must stop at the crossing iteration)", requests)
	}
}

// TestRunnerBoundsToolCallsPerRun is the H-03 regression for the 64-call
// ceiling across model iterations: a later batch that would push the run past
// the cap is rejected in full, so exactly the calls from accepted batches run.
// Before the fix there was no run-wide call count, so every batch executed
// until max_tool_iterations was exhausted.
func TestRunnerBoundsToolCallsPerRun(t *testing.T) {
	root := t.TempDir()
	requests := 0
	executed := 0
	calls := make([]string, 40)
	for i := range calls {
		calls[i] = nativeCall("list_dir", `{"path":"."}`)
	}
	batch := multiToolEvent(calls)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if err := json.NewDecoder(r.Body).Decode(new(ollama.ChatRequest)); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, batch)
	}))
	t.Cleanup(srv.Close)

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 5, &ToolPolicy{})
	err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "list everything"}},
	}, func(msg Msg) {
		if _, ok := msg.(ToolStartMsg); ok {
			executed++
		}
	})
	if err == nil || !strings.Contains(err.Error(), "tool call limit (64)") {
		t.Errorf("error = %v, want run-wide call limit error", err)
	}
	if executed != 40 {
		t.Errorf("tool executions = %d, want 40 (only the first accepted batch runs)", executed)
	}
	if requests != 2 {
		t.Errorf("chat requests = %d, want 2 (crossing second batch rejected)", requests)
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
			if len(req.Tools) != 6 {
				t.Errorf("tools = %d, want 6 (read/list/grep/write/edit/run_command)", len(req.Tools))
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
	calls, err := mergeToolCalls(nil, []ollama.ToolCall{{Function: ollama.ToolCallFunction{Name: "read_file", Arguments: json.RawMessage(`{"path":"`)}}})
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	calls, err = mergeToolCalls(calls, []ollama.ToolCall{{Function: ollama.ToolCallFunction{Name: "read_file", Arguments: json.RawMessage(`README.md"}`)}}})
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if len(calls) != 1 || string(calls[0].Function.Arguments) != `{"path":"README.md"}` {
		t.Errorf("merged calls = %+v, want one complete argument object", calls)
	}
}

// TestMergeToolCallsAdversarial pins the H-03 merge contract directly at the
// boundary: two simultaneous calls, repeated complete calls, fragmented
// arguments, same-name calls, and ambiguous fragments all resolve
// deterministically — and a mixture of a complete call with a fragment at the
// same slot is refused rather than merged by guesswork.
func TestMergeToolCallsAdversarial(t *testing.T) {
	call := func(name, args string) ollama.ToolCall {
		return ollama.ToolCall{Function: ollama.ToolCallFunction{Name: name, Arguments: json.RawMessage(args)}}
	}
	readA := call("read_file", `{"path":"a.txt"}`)
	readB := call("read_file", `{"path":"b.txt"}`)
	listDot := call("list_dir", `{"path":"."}`)

	t.Run("two simultaneous calls survive", func(t *testing.T) {
		got, err := mergeToolCalls(nil, []ollama.ToolCall{readA, listDot})
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if len(got) != 2 || got[0].Function.Name != "read_file" || got[1].Function.Name != "list_dir" {
			t.Errorf("calls = %+v, want read_file then list_dir", got)
		}
	})

	t.Run("repeated complete call deduplicates", func(t *testing.T) {
		got, err := mergeToolCalls([]ollama.ToolCall{readA}, []ollama.ToolCall{readA})
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("calls = %+v, want the single repeated call deduplicated", got)
		}
	})

	t.Run("same-name complete calls both survive", func(t *testing.T) {
		got, err := mergeToolCalls([]ollama.ToolCall{readA}, []ollama.ToolCall{readB})
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if len(got) != 2 || string(got[1].Function.Arguments) != `{"path":"b.txt"}` {
			t.Errorf("calls = %+v, want a.txt kept and b.txt appended", got)
		}
	})

	t.Run("fragmented arguments concatenate", func(t *testing.T) {
		got, err := mergeToolCalls([]ollama.ToolCall{call("read_file", `{"path":"`)}, []ollama.ToolCall{call("read_file", `a.txt"}`)})
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if len(got) != 1 || string(got[0].Function.Arguments) != `{"path":"a.txt"}` {
			t.Errorf("args = %q, want the two fragments joined", got[0].Function.Arguments)
		}
	})

	t.Run("ambiguous fragment then complete is refused", func(t *testing.T) {
		// A fragment at the same slot followed by a complete value cannot be
		// told apart from a retransmission of the same call; concatenating
		// would corrupt the arguments, so the merge must fail instead.
		_, err := mergeToolCalls([]ollama.ToolCall{call("read_file", `{"path":"`)}, []ollama.ToolCall{readA})
		if err == nil || !strings.Contains(err.Error(), "ambiguous tool call fragments") {
			t.Errorf("error = %v, want ambiguous-fragments rejection", err)
		}
	})

	t.Run("complete then fragment is refused", func(t *testing.T) {
		_, err := mergeToolCalls([]ollama.ToolCall{readA}, []ollama.ToolCall{call("read_file", `{"path":"`)})
		if err == nil || !strings.Contains(err.Error(), "ambiguous tool call fragments") {
			t.Errorf("error = %v, want ambiguous-fragments rejection", err)
		}
	})

	t.Run("null arguments carry no fragment", func(t *testing.T) {
		got, err := mergeToolCalls([]ollama.ToolCall{readA}, []ollama.ToolCall{call("read_file", "null")})
		if err != nil {
			t.Fatalf("merge: %v", err)
		}
		if len(got) != 1 || string(got[0].Function.Arguments) != `{"path":"a.txt"}` {
			t.Errorf("calls = %+v, want a.txt preserved", got)
		}
	})

	t.Run("fragment accumulation is capped at 1 MiB", func(t *testing.T) {
		big := strings.Repeat("x", maxToolArgBytes-16)
		_, err := mergeToolCalls([]ollama.ToolCall{call("write_file", `{"content":"`+big)}, []ollama.ToolCall{call("write_file", strings.Repeat("y", 64)+"\"}")})
		if err == nil || !strings.Contains(err.Error(), "tool call argument exceeds 1048576 bytes") {
			t.Errorf("error = %v, want per-call argument ceiling", err)
		}
	})

	t.Run("single oversized complete call is refused", func(t *testing.T) {
		_, err := mergeToolCalls(nil, []ollama.ToolCall{call("read_file", `{"path":"`+strings.Repeat("a", maxToolArgBytes+64)+"\"}")})
		if err == nil || !strings.Contains(err.Error(), "tool call argument exceeds 1048576 bytes") {
			t.Errorf("error = %v, want per-call argument ceiling", err)
		}
	})
}

// Content-embedded tool JSON is only honored when the model explicitly frames
// the turn as a tool-call batch — an {"tool_calls":[...]} envelope, optionally
// inside a ```json fence. Ordinary JSON output (a bare {"name":...} object, a
// top-level array) is prose: it must render as text, never execute.
func TestRunnerParsesContentEmbeddedToolJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// runToolCase drives one Run over a scripted two-turn endpoint.
	runToolCase := func(t *testing.T, firstContent string) (calls int, got string) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/x-ndjson")
			if calls == 1 {
				io.WriteString(w, finalEvent(firstContent))
				return
			}
			io.WriteString(w, finalEvent("found it"))
		}))
		t.Cleanup(srv.Close)
		r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{})
		err := r.Run(context.Background(), Request{Model: "coder", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "find needle"}}}, func(msg Msg) {
			if token, ok := msg.(TokenMsg); ok {
				got += token.Text
			}
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return calls, got
	}

	t.Run("tool_calls envelope executes", func(t *testing.T) {
		calls, got := runToolCase(t, `{"tool_calls":[{"name":"grep","arguments":{"pattern":"needle","path":"x.txt"}}]}`)
		if calls != 2 || got != "found it" {
			t.Errorf("calls=%d final=%q, want 2 and found it", calls, got)
		}
	})

	t.Run("fenced tool_calls envelope executes", func(t *testing.T) {
		calls, got := runToolCase(t, "```json\n{\"tool_calls\":[{\"name\":\"grep\",\"arguments\":{\"pattern\":\"needle\",\"path\":\"x.txt\"}}]}\n```")
		if calls != 2 || got != "found it" {
			t.Errorf("calls=%d final=%q, want 2 and found it", calls, got)
		}
	})

	t.Run("bare name/arguments object is prose, not a tool call", func(t *testing.T) {
		bare := `{"name":"grep","arguments":{"pattern":"needle","path":"x.txt"}}`
		calls, got := runToolCase(t, bare)
		if calls != 1 || got != bare {
			t.Errorf("calls=%d final=%q, want the bare object rendered as text and no second request", calls, got)
		}
	})

	t.Run("top-level call array is prose, not a tool call", func(t *testing.T) {
		array := `[{"name":"grep","arguments":{"pattern":"needle","path":"x.txt"}}]`
		calls, got := runToolCase(t, array)
		if calls != 1 || got != array {
			t.Errorf("calls=%d final=%q, want the array rendered as text and no second request", calls, got)
		}
	})
}

// TestLooksLikeEmbeddedJSONPinned pins the streaming-holdback predicate
// (runner.go looksLikeEmbeddedJSON): content that may be a tool-call
// transport — a JSON object/array prefix or a fence — is held back for the
// whole turn (the accepted tradeoff, even for ordinary fenced code), while
// plain prose — and any turn that merely contains JSON mid-text — streams.
// Changing this predicate changes what users see mid-stream; a change here
// must be a conscious tradeoff, not a refactor accident.
func TestLooksLikeEmbeddedJSONPinned(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"empty", "", false},
		{"plain prose streams", "Let me look for that file.", false},
		{"prose after leading blank lines", "\n\nSure thing.", false},
		{"json object prefix held back", `{"tool_calls":[{"name":"grep"}]}`, true},
		{"json array prefix held back", `["candidate transport"]`, true},
		{"fenced envelope held back", "```json\n{\"tool_calls\":[]}\n```", true},
		{"ordinary fenced code held back (whole-turn tradeoff)", "```go\nfmt.Println(\"hi\")\n```", true},
		{"whitespace then json held back", "\n  {\"a\":1}", true},
		{"whitespace then fence held back", "\n\t```", true},
		{"mid-text json is prose, streams", `Here is the JSON: {"a":1}`, false},
		{"brace after words is prose", "weird { start", false},
		{"bare fence alone held back", "```\n}", true}, // still opens a fence: transport-shaped
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeEmbeddedJSON(tc.text); got != tc.want {
				t.Errorf("looksLikeEmbeddedJSON(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
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

// TestRunnerCancelsToolExecutionOnCanceledContext pins the M-06 boundary at
// the runner: executeTool receives the run context and every filesystem tool
// must honor it. A cancellation that lands while a tool call is being
// dispatched (observed at ToolStartMsg, before the executor runs) must reach
// the tool — the file is never opened, no fabricated ToolResultMsg success
// is emitted — and Run returns the context error promptly. Pre-fix,
// read_file/list_dir were invoked without the context, so the tool executed
// anyway and reported a successful read after the cancel.
func TestRunnerCancelsToolExecutionOnCanceledContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("must not be read\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, toolEvent(nativeCall("read_file", `{"path":"secret.txt"}`)))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 4, &ToolPolicy{})
	var toolOK bool
	var summary string
	err := r.Run(ctx, Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "read it"}},
	}, func(msg Msg) {
		switch m := msg.(type) {
		case ToolStartMsg:
			// Cancellation lands before the tool's executor starts: the tool
			// boundary must refuse to run, never execute behind the cancel.
			cancel()
		case ToolResultMsg:
			if m.Name == "read_file" {
				toolOK = m.OK
				summary = m.Summary
			}
		}
	})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Run error = %v, want a context-canceled error", err)
	}
	if toolOK {
		t.Fatalf("read_file reported OK after cancellation (summary %q): the tool executed despite a canceled context", summary)
	}
}

func TestRunnerPropagatesFinalChunkMetrics(t *testing.T) {
	// N3: the done event carries the final chunk's generation metrics so the
	// UI can show measured tok/s and exact prompt tokens.
	body := `{"message":{"role":"assistant","content":"hi"},"done":false}
{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":1043,"prompt_eval_duration":22818421,"eval_count":337,"eval_duration":3412523782}
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, body)
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
	if done.Reason != "stop" {
		t.Errorf("Reason = %q, want stop", done.Reason)
	}
	want := ollama.ChatMetrics{PromptTokens: 1043, PromptNanos: 22818421, Tokens: 337, Nanos: 3412523782}
	if done.Metrics != want {
		t.Errorf("Metrics = %+v, want %+v", done.Metrics, want)
	}
}

func TestRunnerMetricsAbsentStaysZero(t *testing.T) {
	// A final chunk without metrics keeps AgentDoneMsg.Metrics zero (older
	// hosts) — the done shape is unchanged (M7-B byte-compat).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, finalEvent("plain answer"))
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
	if done.Metrics != (ollama.ChatMetrics{}) {
		t.Errorf("Metrics = %+v, want zero", done.Metrics)
	}
}

func TestRunnerMetricsLastFinalChunkWins(t *testing.T) {
	// A multi-iteration tool loop reports the metrics of the stream that
	// ENDED the turn, not the intermediate tool-call stream.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/x-ndjson")
		if calls == 1 {
			io.WriteString(w, `{"message":{"role":"assistant","tool_calls":[{"function":{"name":"list_dir","arguments":{"path":"."}}}]},"done":true,"done_reason":"tool_calls","eval_count":11,"eval_duration":11e9}`+"\n")
			return
		}
		io.WriteString(w, `{"message":{"role":"assistant","content":"listed"},"done":true,"done_reason":"stop","prompt_eval_count":2222,"eval_count":33,"eval_duration":3.3e9}`+"\n")
	}))
	t.Cleanup(srv.Close)
	var done AgentDoneMsg
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), t.TempDir(), "", 3, &ToolPolicy{})
	if err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "list"}},
	}, func(msg Msg) {
		if d, ok := msg.(AgentDoneMsg); ok {
			done = d
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (tool call + final answer)", calls)
	}
	if done.Reason != "stop" {
		t.Errorf("Reason = %q, want stop", done.Reason)
	}
	want := ollama.ChatMetrics{PromptTokens: 2222, Tokens: 33, Nanos: 3.3e9}
	if done.Metrics != want {
		t.Errorf("Metrics = %+v, want the final stream's %+v", done.Metrics, want)
	}
}
