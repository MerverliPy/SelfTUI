package agent

// N5 debug-drawer loop traces (PLAN.md §12 N5): the runner logs its loop
// decisions — tool calls (name + bounded args summary), tool outcomes,
// context budgeting, plain-chat fallbacks, and budget rejections — to the
// shared logger when one is attached. Nil loggers stay silent (every
// pre-N5 test relies on that).

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

func newTraceLogger() (*log.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	l := log.NewWithOptions(&buf, log.Options{ReportCaller: false, Level: log.DebugLevel})
	return l, &buf
}

func TestRunnerLogsToolLoopDecisions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/x-ndjson")
		if calls == 1 {
			io.WriteString(w, toolEvent(nativeCall("grep", `{"pattern":"needle","path":"x.txt"}`)))
			return
		}
		io.WriteString(w, finalEvent("found it"))
	}))
	t.Cleanup(srv.Close)

	l, buf := newTraceLogger()
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{}).WithLogger(l)
	if err := r.Run(context.Background(), Request{Model: "coder", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "find"}}}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"agent tool call", `tool=grep`, "agent tool result", `tool=grep`} {
		if !strings.Contains(out, want) {
			t.Errorf("loop trace missing %q:\n%s", want, out)
		}
	}
	// The args summary is bounded and logfmt-encoded (the value is quoted);
	// the payload shape still shows without the wire's raw JSON quotes.
	if !strings.Contains(out, `args=`) || !strings.Contains(out, "needle") {
		t.Errorf("args summary missing:\n%s", out)
	}
}

func TestRunnerLogsContextBudget(t *testing.T) {
	l, buf := newTraceLogger()
	r := NewRunner(ollama.New("http://localhost:1", ""), "", "", 1).WithLogger(l)
	// One exchange whose content cannot fit 3/4 of a tiny num_ctx: the
	// budget shortens it, and the drawer must see the decision.
	big := strings.Repeat("x", 4096)
	out := r.budgeted([]ollama.ChatMessage{
		{Role: ollama.RoleUser, Content: big},
	}, 64)
	if len(out) == 0 || len(out[0].Content) >= len(big) {
		t.Fatalf("budgeted = %d messages (%d bytes), want shortened content", len(out), len(out[0].Content))
	}
	if !strings.Contains(buf.String(), "agent context budget") {
		t.Errorf("budget trace missing:\n%s", buf.String())
	}
	// An untruncated pass stays silent (the drawer only shows decisions).
	buf.Reset()
	r.budgeted([]ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}}, 0)
	if strings.Contains(buf.String(), "agent context budget") {
		t.Errorf("untruncated pass logged:\n%s", buf.String())
	}
}

func TestRunnerLogsFallback(t *testing.T) {
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

	l, buf := newTraceLogger()
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), t.TempDir(), "", 2, &ToolPolicy{}).WithLogger(l)
	if err := r.Run(context.Background(), Request{Model: "m", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}}}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "agent fallback") || !strings.Contains(buf.String(), "plain chat") {
		t.Errorf("fallback warn missing:\n%s", buf.String())
	}
}

func TestRunnerLogsIterationBudgetExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, toolEvent(nativeCall("list_dir", `{"path":"."}`)))
	}))
	t.Cleanup(srv.Close)

	l, buf := newTraceLogger()
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), t.TempDir(), "", 1, &ToolPolicy{}).WithLogger(l)
	err := r.Run(context.Background(), Request{Model: "m", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "x"}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "maximum tool iterations") {
		t.Fatalf("error = %v, want bounded-iteration error", err)
	}
	if !strings.Contains(buf.String(), "agent iteration budget exhausted") {
		t.Errorf("iteration-budget warn missing:\n%s", buf.String())
	}
}

func TestSummarizeToolArgsBounds(t *testing.T) {
	short := `{"path":"x.txt"}`
	if got := summarizeToolArgs(short); got != short {
		t.Errorf("short args rewritten: %q", got)
	}
	long := strings.Repeat("a", 500)
	got := summarizeToolArgs(long)
	if len(got) != 163 || !strings.HasSuffix(got, "…") { // 160 bytes + 3-byte ellipsis
		t.Errorf("long args summary = %d bytes, want 160 + ellipsis", len(got))
	}
	multibyte := strings.Repeat("é", 200) // invalid UTF-8 when byte-cut
	got = summarizeToolArgs(multibyte)
	if !strings.HasSuffix(got, "…") || !utf8Valid(got[:len(got)-3]) {
		t.Errorf("multibyte summary is not valid UTF-8: %q", got)
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == 0xfffd {
			return false
		}
	}
	return true
}

// The existing runner tests run with no logger attached; this is a targeted
// pin that a nil-logger WithLogger call is a harmless no-op and the nil
// logger guards never panic.
func TestRunnerWithLoggerNilSafe(t *testing.T) {
	r := NewRunner(ollama.New("http://localhost:1", ""), "", "", 1)
	if got := r.WithLogger(nil); got != r {
		t.Error("WithLogger(nil) should return the same runner")
	}
	r.logDebug("silent")
	r.logWarn("silent")
	if r.budgeted([]ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}}, 0) == nil {
		t.Error("budgeted returned nil")
	}
}
