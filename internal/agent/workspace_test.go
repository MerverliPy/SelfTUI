package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// gitInitRepo turns dir into a real git repository with one commit, using the
// same host-side git the production context builder uses. Tests skip the git
// assertions cleanly if git is unavailable (mirrors WorkspaceContext itself,
// which degrades to the tree-only block).
func gitInitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "trunk")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("hello git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "initial commit")
}

func TestWorkspaceContextNonGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := WorkspaceContext(ctx, root)
	if got == "" {
		t.Fatal("WorkspaceContext = empty")
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("index missing main.go: %q", got)
	}
	if strings.Contains(got, "Git:") || strings.Contains(got, "porcelain") {
		t.Errorf("non-git dir must omit the git section: %q", got)
	}
}

func TestWorkspaceContextGitRepo(t *testing.T) {
	root := t.TempDir()
	gitInitRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "dirty.go"), []byte("package dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := WorkspaceContext(ctx, root)
	if !strings.Contains(got, "## trunk") {
		t.Errorf("git section missing porcelain branch line: %q", got)
	}
	if !strings.Contains(got, "dirty.go") {
		t.Errorf("porcelain status missing untracked file: %q", got)
	}
	if !strings.Contains(got, "initial commit") {
		t.Errorf("recent commits missing seeded commit: %q", got)
	}
	if !strings.Contains(got, "tracked.txt") {
		t.Errorf("index missing tracked.txt: %q", got)
	}
}

func TestWorkspaceContextBounded(t *testing.T) {
	root := t.TempDir()
	// More files than maxTreeEntries: the index must truncate, not grow.
	for i := 0; i < maxTreeEntries+50; i++ {
		name := filepath.Join(root, "filler"+strings.Repeat("x", 3)+"_"+string(rune('a'+i%26))+string(rune('0'+i/26))+string(rune('0'+i%10))+".txt")
		if err := os.WriteFile(name, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := WorkspaceContext(ctx, root)
	if len(got) > maxWorkspaceContextBytes+len("[workspace context truncated]\n") {
		t.Errorf("context = %d bytes, exceeds bound", len(got))
	}
	if !strings.Contains(got, "[index truncated]") {
		t.Errorf("expected truncation marker in %d-byte context", len(got))
	}
}

func TestWorkspaceContextDepthCapped(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d", "e", "f")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "deep.txt"), []byte("deep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := WorkspaceContext(ctx, root)
	if strings.Contains(got, "deep.txt") {
		t.Errorf("depth cap leaked beyond %d levels: %q", maxTreeDepth, got)
	}
}

func TestWorkspaceContextCanceled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := WorkspaceContext(ctx, root); got != "" {
		t.Errorf("canceled context = %q, want empty", got)
	}
}

func TestArmedRunnerInjectsWorkspaceContext(t *testing.T) {
	root := t.TempDir()
	gitInitRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var firstMessages []ollama.ChatMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		firstMessages = req.Messages
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, finalEvent("ok"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "you are an agent", 4, &ToolPolicy{})
	if err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hello"}},
	}, func(Msg) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(firstMessages) < 3 ||
		firstMessages[0].Role != ollama.RoleSystem || firstMessages[0].Content != "you are an agent" ||
		firstMessages[1].Role != ollama.RoleSystem || !strings.Contains(firstMessages[1].Content, "Workspace context") ||
		firstMessages[2].Role != ollama.RoleUser {
		t.Fatalf("wire messages = %+v, want system prompt + workspace context + user turn", firstMessages)
	}
	if !strings.Contains(firstMessages[1].Content, "initial commit") {
		t.Errorf("workspace context missing git section: %q", firstMessages[1].Content)
	}
}

func TestPlainChatRunnerDoesNotInjectWorkspaceContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var firstMessages []ollama.ChatMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Tools) != 0 {
			t.Errorf("plain chat must not send tools, got %d", len(req.Tools))
		}
		firstMessages = req.Messages
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, finalEvent("ok"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunner(ollama.New(srv.URL, ""), root, "you are chat", 4)
	if err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hello"}},
	}, func(Msg) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(firstMessages) != 2 || firstMessages[0].Role != ollama.RoleSystem ||
		firstMessages[0].Content != "you are chat" || firstMessages[1].Role != ollama.RoleUser {
		t.Fatalf("wire messages = %+v, want exactly system + user (no workspace context)", firstMessages)
	}
}
