package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"selftui/internal/ollama"
)

// Phase 4 (workspace tool trust): ToolPolicy is the opt-in policy. A nil
// policy (NewRunner, the compatibility constructor) means tools disabled —
// the runner behaves as plain chat and never puts a tools field on the wire.
// NewRunnerWithPolicy arms the five v0.1 tools and AuthorizePath gates every
// one of them before it executes, on top of the canonical containment.

func TestToolPolicyToolsAreTheFiveV01Tools(t *testing.T) {
	var names []string
	for _, d := range (ToolPolicy{}).Tools() {
		names = append(names, d.Function.Name)
	}
	want := []string{"read_file", "list_dir", "grep", "write_file", "edit_file"}
	if len(names) != len(want) {
		t.Fatalf("Tools() = %v, want exactly %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("Tools() = %v, want exactly %v (order matters)", names, want)
		}
	}
	for _, name := range names {
		if name == "run_command" {
			t.Fatal("Tools() must not expose run_command in v0.1")
		}
	}
}

func TestAuthorizePathSensitiveRules(t *testing.T) {
	p := ToolPolicy{}
	rejected := []string{
		// Sensitive dot-directories anywhere in the requested path.
		".ssh/id_rsa", "a/.ssh/b", "/home/u/.ssh/config", "~/.ssh/known_hosts",
		".gnupg/pubring.kbx", "x/.gnupg/y", "/etc/.gnupg",
		".aws/credentials", "a/.aws/b", "/home/u/.aws/config",
		".azure/azureProfile.json", "a/.azure/b",
		".kube/config", "a/.kube/b",
		".config/gcloud/creds.json",
		"/home/u/.config/gcloud/application_default_credentials.json",
		".config/gcloud", "a/.config/gcloud/b",
		// Forbidden credential basenames (last path element).
		".env", "creds/.env", "/abs/.env", "x/.env",
		".env.local", ".env.production",
		"credentials", "deep/credentials", "a/credentials.json",
	}
	for _, path := range rejected {
		if err := p.AuthorizePath(path); err == nil {
			t.Errorf("AuthorizePath(%q) = nil, want rejection", path)
		}
	}
	allowed := []string{
		"notes.txt", "README.md", "a/b/c.txt", ".gitignore",
		".", "./", "./notes.txt", "a/./b.txt",
		// The template carve-out: .env.example stays readable/writable.
		".env.example", "config/.env.example", "a/.env.example",
		// Only the .config/gcloud composite is sensitive.
		".config/other", ".config/gcloud-util/x",
		// Traversal is containment's job, not this lexical policy's.
		"../x", "sub/../../x",
		"credentials.json.backup", "my-credentials.txt", "creds.env", "env.local",
	}
	for _, path := range allowed {
		if err := p.AuthorizePath(path); err != nil {
			t.Errorf("AuthorizePath(%q) = %v, want allowed", path, err)
		}
	}
}

// TestDisabledRunnerSendsNoToolsField is the wire proof of the disabled
// default: the Ollama chat request body must not contain a tools key at all
// (omitempty on the field is not enough — this inspects the raw JSON).
func TestDisabledRunnerSendsNoToolsField(t *testing.T) {
	var rawBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, finalEvent("plain answer"))
	}))
	t.Cleanup(srv.Close)

	var got string
	r := NewRunner(ollama.New(srv.URL, ""), t.TempDir(), "", 2)
	if err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}},
	}, func(msg Msg) {
		if token, ok := msg.(TokenMsg); ok {
			got += token.Text
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, has := rawBody["tools"]; has {
		t.Errorf("disabled runner sent a tools field: %s", rawBody["tools"])
	}
	if got != "plain answer" {
		t.Errorf("final text = %q, want plain chat answer", got)
	}
}

// TestPolicyRunnerSendsToolDefinitions: arming the policy puts the five v0.1
// tools on the wire, restoring the tool loop.
func TestPolicyRunnerSendsToolDefinitions(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Tools) != 5 {
			t.Errorf("tools = %d, want the five v0.1 tools", len(req.Tools))
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, finalEvent("done"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), t.TempDir(), "", 2, &ToolPolicy{})
	if err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}},
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 1 {
		t.Errorf("chat calls = %d, want 1", calls)
	}
}

// TestDisabledRunnerNeverExecutesTools: even an endpoint that replies with a
// tool_calls event cannot make a disabled runner execute anything — it
// behaves as plain chat and no confirmation or file write may occur.
func TestDisabledRunnerNeverExecutesTools(t *testing.T) {
	root := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, toolEvent(nativeCall("write_file", `{"path":"sneaky.txt","content":"x"}`)))
	}))
	t.Cleanup(srv.Close)

	var toolEvents int
	r := NewRunner(ollama.New(srv.URL, ""), root, "", 2)
	if err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "write"}},
	}, func(msg Msg) {
		switch msg.(type) {
		case ToolStartMsg, ToolConfirmMsg, ToolResultMsg:
			toolEvents++
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if toolEvents != 0 {
		t.Errorf("disabled runner emitted %d tool events, want 0", toolEvents)
	}
	if _, err := os.Stat(filepath.Join(root, "sneaky.txt")); !os.IsNotExist(err) {
		t.Fatalf("disabled runner wrote a file: %v", err)
	}
}

// TestPolicyAuthorizesBeforeExecution: with tools armed, every one of the
// five tools is gated by AuthorizePath on the requested path before it can
// execute — a sensitive request fails with the policy error, not the file's
// contents, and never reaches the user confirmation for mutations.
func TestPolicyAuthorizesBeforeExecution(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "credentials.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("fine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRunnerWithPolicy(nil, root, "", 1, &ToolPolicy{})

	tools := []struct{ name, args string }{
		{"read_file", `{"path":".env"}`},
		{"list_dir", `{"path":".ssh"}`},
		{"grep", `{"pattern":"x","path":".env"}`},
		{"write_file", `{"path":"credentials.json","content":"x"}`},
		{"edit_file", `{"path":".env","old":"a","new":"b"}`},
	}
	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			confirmed := false
			_, err := r.executeTool(context.Background(), ollama.ToolCall{
				Function: ollama.ToolCallFunction{Name: tc.name, Arguments: json.RawMessage(tc.args)},
			}, func(msg Msg) {
				if _, ok := msg.(ToolConfirmMsg); ok {
					confirmed = true
				}
			})
			if err == nil || !strings.Contains(err.Error(), "not allowed by the workspace tool policy") {
				t.Fatalf("%s error = %v, want policy rejection", tc.name, err)
			}
			if confirmed {
				t.Errorf("%s surfaced a confirmation for a policy-blocked path", tc.name)
			}
		})
	}

	// A benign path still executes (read-only works without a client).
	got, err := r.executeTool(context.Background(), ollama.ToolCall{
		Function: ollama.ToolCallFunction{Name: "read_file", Arguments: json.RawMessage(`{"path":"ok.txt"}`)},
	}, nil)
	if err != nil || got != "fine\n" {
		t.Errorf("read_file ok.txt = %q, %v, want fine", got, err)
	}

	// write_file may create .env.example (the one dotenv that is allowed).
	confirmed := false
	result, err := r.executeTool(context.Background(), ollama.ToolCall{
		Function: ollama.ToolCallFunction{Name: "write_file", Arguments: json.RawMessage(`{"path":".env.example","content":"TOKEN=\n"}`)},
	}, func(msg Msg) {
		if confirmation, ok := msg.(ToolConfirmMsg); ok {
			confirmed = true
			confirmation.Respond(true)
		}
	})
	if err != nil || result == "" || !confirmed {
		t.Errorf("write_file .env.example result=%q confirmed=%v err=%v, want approval path", result, confirmed, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".env.example")); err != nil {
		t.Errorf(".env.example was not written: %v", err)
	}
}

// TestPolicyStillEnforcesContainment: AuthorizePath is additive — the
// canonical workspace containment (traversal and symlink escapes) still
// rejects paths the lexical policy allows.
func TestPolicyStillEnforcesContainment(t *testing.T) {
	root := t.TempDir()
	// A real file outside the workspace (sibling of root) so the escape
	// resolves and containment must reject it.
	outsidePath := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "link")); err == nil {
		t.Cleanup(func() { os.Remove(filepath.Join(root, "link")) })
	}
	r := NewRunnerWithPolicy(nil, root, "", 1, &ToolPolicy{})
	emit := func(msg Msg) {
		if confirmation, ok := msg.(ToolConfirmMsg); ok {
			confirmation.Respond(true) // approve; containment must still refuse
		}
	}
	for _, tc := range []struct{ name, args string }{
		{"read_file", `{"path":"../outside.txt"}`},
		{"read_file", `{"path":"/etc/passwd"}`},
		{"write_file", `{"path":"../out.txt","content":"x"}`},
	} {
		_, err := r.executeTool(context.Background(), ollama.ToolCall{
			Function: ollama.ToolCallFunction{Name: tc.name, Arguments: json.RawMessage(tc.args)},
		}, emit)
		if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
			t.Errorf("%s %s error = %v, want containment rejection", tc.name, tc.args, err)
		}
	}
	if _, err := r.executeTool(context.Background(), ollama.ToolCall{
		Function: ollama.ToolCallFunction{Name: "read_file", Arguments: json.RawMessage(`{"path":"link"}`)},
	}, nil); err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("symlink read error = %v, want containment rejection", err)
	}
}
