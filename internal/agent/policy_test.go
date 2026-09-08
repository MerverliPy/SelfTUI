package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// Phase 4 (workspace tool trust): ToolPolicy is the opt-in policy. A nil
// policy (NewRunner, the compatibility constructor) means tools disabled —
// the runner behaves as plain chat and never puts a tools field on the wire.
// NewRunnerWithPolicy arms the seven jailed V2e tools (V2c's six plus the
// write_files batch tool) and AuthorizePath gates every path-based tool
// before it executes, on top of the canonical containment.

func TestToolPolicyToolsAreTheSevenV2eTools(t *testing.T) {
	var names []string
	for _, d := range (ToolPolicy{}).Tools() {
		names = append(names, d.Function.Name)
	}
	want := []string{"read_file", "list_dir", "grep", "write_file", "edit_file", "write_files", "run_command"}
	if len(names) != len(want) {
		t.Fatalf("Tools() = %v, want exactly %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("Tools() = %v, want exactly %v (order matters)", names, want)
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
		// V2e: .git is never a writable (or readable-through-tools) target —
		// git internals are not workspace content (design §2 gap row).
		".git/config", ".git/HEAD", "a/.git/hooks/pre-commit", "sub/.git/refs/heads/main",
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

// TestPolicyRunnerSendsToolDefinitions: arming the policy puts the seven V2e
// tools on the wire, restoring the tool loop.
func TestPolicyRunnerSendsToolDefinitions(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Tools) != 7 {
			t.Errorf("tools = %d, want the seven V2e tools", len(req.Tools))
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

// grepDeniedMarkers are the unique content markers seeded into every
// policy-denied fixture below. grepDeniedHeaders are the rel-path line
// prefixes a leak would print. Both must never appear in grep output over a
// workspace that also holds the GOOD_* allowed controls.
var grepDeniedMarkers = []string{
	"LEAK_DOTENV", "LEAK_DOTENV_LOCAL", "LEAK_DOTENV_PROD",
	"LEAK_CREDENTIALS", "LEAK_CREDENTIALS_JSON",
	"LEAK_SSH", "LEAK_GNUPG", "LEAK_AWS", "LEAK_AZURE", "LEAK_KUBE", "LEAK_GCLOUD",
	"LEAK_NESTED_SSH",
}

// seedSensitiveGrepWorkspace builds a workspace where ordinary files and the
// .env.example template carry GOOD_* markers while every sensitive class the
// denylist covers (.env, .env.local, .env.production, credentials,
// credentials.json, .ssh, .gnupg, .aws, .azure, .kube, .config/gcloud, plus a
// nested denied directory and a nested denied dotenv) carries a distinct
// LEAK_* marker. grep pattern "GOOD_|LEAK_" matches every seeded line.
func seedSensitiveGrepWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Ordinary matching files.
	write("README.md", "GOOD_README: hello world\n")
	write("src/app.go", "GOOD_CODE\n")
	write("docs/notes.txt", "GOOD_NOTES\n")
	// The template carve-out: .env.example stays readable.
	write(".env.example", "GOOD_EXAMPLE=dummy\n")
	// Denied credential files sitting in otherwise-allowed directories.
	write(".env", "LEAK_DOTENV=topsecret\n")
	write(".env.local", "LEAK_DOTENV_LOCAL=secret\n")
	write("credentials", "LEAK_CREDENTIALS\n")
	write("credentials.json", "LEAK_CREDENTIALS_JSON\n")
	write("proj/sub/deep/.env.production", "LEAK_DOTENV_PROD\n")
	// Denied directories (pruned before descent).
	write(".ssh/id_rsa", "LEAK_SSH\n")
	write(".gnupg/private.key", "LEAK_GNUPG\n")
	write(".aws/credentials", "LEAK_AWS\n")
	write(".azure/azureProfile.json", "LEAK_AZURE\n")
	write(".kube/config", "LEAK_KUBE\n")
	write(".config/gcloud/application_default_credentials.json", "LEAK_GCLOUD\n")
	// A nested denied directory under otherwise-allowed directories.
	write("proj/sub/.ssh/id_ed25519", "LEAK_NESTED_SSH\n")
	return root
}

// assertGrepLeakFree is the shared policy-aware grep oracle: no denied
// content marker and no denied rel-path header may appear, while every
// allowed control must.
func assertGrepLeakFree(t *testing.T, out string) {
	t.Helper()
	for _, marker := range grepDeniedMarkers {
		if strings.Contains(out, marker) {
			t.Errorf("grep output leaked denied content marker %q\noutput:\n%s", marker, out)
		}
	}
	for _, header := range []string{".env:", ".env.local:", "credentials:", "credentials.json:", ".ssh/", ".gnupg/", ".aws/", ".azure/", ".kube/", "gcloud/", ".env.production:"} {
		if strings.Contains(out, header) {
			t.Errorf("grep output names a policy-denied path (%q)\noutput:\n%s", header, out)
		}
	}
	for _, want := range []string{"GOOD_README", "GOOD_CODE", "GOOD_NOTES"} {
		if !strings.Contains(out, want) {
			t.Errorf("grep output is missing ordinary match %q\noutput:\n%s", want, out)
		}
	}
	// The documented .env.example carve-out must be readable and visible.
	if !strings.Contains(out, ".env.example:1:GOOD_EXAMPLE") {
		t.Errorf("grep output lost the .env.example carve-out control\noutput:\n%s", out)
	}
}

// TestGrepAppliesPolicyToEveryWorkspaceDescendant is the direct-boundary
// proof of C-01: recursive grep over "." with the policy callback never
// opens a denied descendant — denied directories are pruned, denied files
// are skipped, ordinary files and .env.example still match.
func TestGrepAppliesPolicyToEveryWorkspaceDescendant(t *testing.T) {
	root := seedSensitiveGrepWorkspace(t)
	p := ToolPolicy{}
	out, err := Grep(context.Background(), root, "GOOD_|LEAK_", ".", func(rel string) error { return p.AuthorizePath(rel) })
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	assertGrepLeakFree(t, out)
}

// TestGrepSkipsNestedDeniedDirectoriesButPreservesIOErrors pins the two
// sides of the skip contract: policy denials are skipped deterministically
// wherever they sit (deep .aws, .config/gcloud, nested dotenv files), while
// a real traversal error (unreadable directory) still fails the operation
// instead of being swallowed.
func TestGrepSkipsNestedDeniedDirectoriesButPreservesIOErrors(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a/b/.aws/credentials", "LEAK_NESTED_AWS\n")
	write("c/.config/gcloud/creds.json", "LEAK_DEEP_GCLOUD\n")
	write("d/e/.env.production", "LEAK_DEEP_DOTENV\n")
	write("ok.txt", "GOOD_OK\n")
	p := ToolPolicy{}
	out, err := Grep(context.Background(), root, "GOOD_|LEAK_", ".", func(rel string) error { return p.AuthorizePath(rel) })
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	for _, marker := range []string{"LEAK_NESTED_AWS", "LEAK_DEEP_GCLOUD", "LEAK_DEEP_DOTENV"} {
		if strings.Contains(out, marker) {
			t.Errorf("deep policy denial %q leaked:\n%s", marker, out)
		}
	}
	if !strings.Contains(out, "ok.txt:1:GOOD_OK") {
		t.Errorf("allowed file missing from output:\n%s", out)
	}

	if os.Geteuid() == 0 {
		t.Skip("permission-denied traversal check needs a non-root user")
	}
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "secret.txt"), []byte("GOOD_LOCKED\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	if _, err := Grep(context.Background(), root, "GOOD", ".", func(rel string) error { return p.AuthorizePath(rel) }); err == nil {
		t.Errorf("Grep over an unreadable directory returned nil, want a real traversal error")
	}
}

// TestGrepChecksCancellationWhileWalkingAndScanning pins the ctx plumbing
// added for Task 13 (M-06): a canceled context surfaces as context.Canceled
// promptly — before any work (pre-canceled), from a walk-time hook (the
// dir-prune authorize callback), and between scanned files (the scan loop
// checks before each file).
func TestGrepChecksCancellationWhileWalkingAndScanning(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		dir := filepath.Join(root, fmt.Sprintf("d%d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("GOOD\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("pre-canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := Grep(ctx, root, "GOOD", ".", func(string) error { return nil }); !errors.Is(err, context.Canceled) {
			t.Errorf("pre-canceled Grep error = %v, want context.Canceled", err)
		}
	})

	t.Run("cancel-during-walk", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		// authorize is consulted for directory pruning during the walk with
		// bare dir rels ("d2", no separator); canceling there must surface.
		_, err := Grep(ctx, root, "GOOD", ".", func(rel string) error {
			if rel == "d2" {
				cancel()
			}
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancel during walk: Grep error = %v, want context.Canceled", err)
		}
	})

	t.Run("cancel-between-files", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		// File rels ("d0/f.txt") are only authorized from the scan loop;
		// canceling there must stop the next file from being scanned.
		_, err := Grep(ctx, root, "GOOD", ".", func(rel string) error {
			if rel == "d0/f.txt" {
				cancel()
			}
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancel between files: Grep error = %v, want context.Canceled", err)
		}
	})
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
