package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// N6 (PLAN.md §12): the runner relays qwen3 thinking as opaque ThinkingMsg
// deltas (never discarded, never interpreted), the @-picker's workspace
// listing shares the jailed walk discipline, and @-reference expansion goes
// through the same jailed read_file the model itself could request.

func TestRunnerRelaysThinkingDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"message":{"role":"assistant","thinking":"why "},"done":false}`+"\n")
		io.WriteString(w, `{"message":{"role":"assistant","thinking":"now what"},"done":false}`+"\n")
		io.WriteString(w, finalEvent("the answer"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunner(ollama.New(srv.URL, ""), t.TempDir(), "", 4)
	var thinking []string
	var tokens []string
	err := r.Run(context.Background(), Request{
		Model:    "qwen3:8b",
		Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}},
	}, func(msg Msg) {
		switch m := msg.(type) {
		case ThinkingMsg:
			thinking = append(thinking, m.Text)
		case TokenMsg:
			tokens = append(tokens, m.Text)
		}
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Join(thinking, "") != "why now what" {
		t.Errorf("thinking deltas = %q, want relayed in order", thinking)
	}
	if strings.Join(tokens, "") != "the answer" {
		t.Errorf("token deltas = %q, want content untouched by thinking", tokens)
	}
}

// TestRunnerRelaysTopLevelThinking covers the older wire shape where
// reasoning arrives as a top-level thinking field instead of
// message.thinking; the relay treats the two as alternatives, never
// concatenating both (a delta present in both places must not double).
func TestRunnerRelaysTopLevelThinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"thinking":"top level","message":{"role":"assistant","thinking":"message level"},"done":false}`+"\n")
		io.WriteString(w, finalEvent("answer"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunner(ollama.New(srv.URL, ""), t.TempDir(), "", 4)
	var thinking string
	r.Run(context.Background(), Request{
		Model:    "qwen3:8b",
		Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}},
	}, func(msg Msg) {
		if m, ok := msg.(ThinkingMsg); ok {
			thinking += m.Text
		}
	})
	if thinking != "message level" {
		t.Errorf("thinking = %q, want the message-level delta once, no doubling", thinking)
	}
}

// TestRunnerThinkingNeverEntersToolLoopContent pins the N6 invariant that
// reasoning is neither user-visible output by default nor a tool-call
// transport: a thinking-bearing iteration must still end in the plain
// answer path with the same content the pre-N6 runner produced.
func TestRunnerThinkingNeverEntersToolLoopContent(t *testing.T) {
	root := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"message":{"role":"assistant","thinking":"reasoning about tools"},"done":false}`+"\n")
		io.WriteString(w, finalEvent("plain answer"))
	}))
	t.Cleanup(srv.Close)

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 4, &ToolPolicy{})
	var content strings.Builder
	err := r.Run(context.Background(), Request{
		Model:    "qwen3:8b",
		Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "hi"}},
	}, func(msg Msg) {
		if m, ok := msg.(TokenMsg); ok {
			content.WriteString(m.Text)
		}
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if content.String() != "plain answer" {
		t.Errorf("content = %q, want thinking excluded from the answer stream", content.String())
	}
}

func TestWorkspaceFilesJailAndCaps(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt")
	write("src/b.go")
	write("src/deep/c.go")
	write("src/too/deep/for/limit.txt")  // file at depth 4: the max descent keeps it
	write("src/too/deep/for/more/x.txt") // dir at depth 4: pruned
	write(".git/HEAD")
	write(".git/config")

	files := WorkspaceFiles(context.Background(), root)
	set := map[string]bool{}
	for _, f := range files {
		if filepath.IsAbs(f) || strings.HasPrefix(f, "..") {
			t.Errorf("listing escaped the workspace: %q", f)
		}
		set[f] = true
		if strings.Contains(f, ".git") {
			t.Errorf(".git must be pruned, got %q", f)
		}
	}
	for _, want := range []string{"a.txt", "src/b.go", "src/deep/c.go"} {
		if !set[want] {
			t.Errorf("missing %q in %v", want, files)
		}
	}
	if set["src/too/deep/for/more/x.txt"] {
		t.Error("depth cap must prune directories at maxTreeDepth")
	}
	// Entry cap: 600 files in one flat dir degrade to at most the cap.
	root2 := t.TempDir()
	for i := 0; i < workspaceFilesMaxResults+100; i++ {
		if err := os.WriteFile(filepath.Join(root2, "f"+strings.Repeat("0", 3)+itoa(i)+".txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files2 := WorkspaceFiles(context.Background(), root2)
	if len(files2) > workspaceFilesMaxResults {
		t.Errorf("listing = %d files, want ≤ %d (entry cap)", len(files2), workspaceFilesMaxResults)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestFileRefTokens(t *testing.T) {
	toks := FileRefTokens("look at @README.md and @src/app.go, plus @ x and a@b")
	got := map[string]bool{}
	for _, tok := range toks {
		got[tok] = true
	}
	for _, want := range []string{"README.md", "src/app.go", "b"} {
		if !got[want] {
			t.Errorf("missing token %q in %v", want, toks)
		}
	}
	if got["a"] {
		t.Errorf("'a@b' must tokenize from the '@' as %q only", "b")
	}
	if len(toks) != 3 {
		t.Errorf("tokens = %v, want exactly 3 (trailing comma trimmed, '@ x' skipped)", toks)
	}
}

func TestExpandFileRefs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	// A real sibling directory outside the workspace, so escape attempts hit
	// the containment check (the path exists; only the jail refuses it).
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Repeat("x", maxReadBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "link")); err == nil {
		t.Cleanup(func() { _ = os.Remove(filepath.Join(root, "link")) })
	}

	t.Run("single-ref-expands-inline", func(t *testing.T) {
		got := ExpandFileRefs(ctx, root, "see @a.txt please")
		want := "see @a.txt please\n\n[file: a.txt]\nalpha"
		if got != want {
			t.Errorf("expanded = %q, want %q", got, want)
		}
	})
	t.Run("duplicate-refs-attach-once", func(t *testing.T) {
		got := ExpandFileRefs(ctx, root, "@a.txt then @a.txt again")
		if strings.Count(got, "[file: a.txt]") != 1 {
			t.Errorf("duplicate reference attached more than once:\n%s", got)
		}
	})
	t.Run("missing-file-is-prose", func(t *testing.T) {
		got := ExpandFileRefs(ctx, root, "email me @ nobody.txt")
		if got != "email me @ nobody.txt" {
			t.Errorf("missing file changed the text: %q", got)
		}
	})
	t.Run("escape-attempt-carries-visible-note", func(t *testing.T) {
		got := ExpandFileRefs(ctx, root, "read @../outside/secret now")
		if !strings.Contains(got, "unavailable") || !strings.Contains(got, "escapes workspace") {
			t.Errorf("escape attempt must carry a visible jail note, got %q", got)
		}
		if strings.Contains(got, "secret content") {
			t.Error("jail content must never be attached")
		}
	})
	t.Run("symlink-escape-rejected", func(t *testing.T) {
		got := ExpandFileRefs(ctx, root, "read @link")
		if !strings.Contains(got, "unavailable") {
			t.Errorf("symlink escape must be noted, got %q", got)
		}
		if strings.Contains(got, "secret") {
			t.Error("symlink target content must never be attached")
		}
	})
	t.Run("oversized-file-noted-not-attached", func(t *testing.T) {
		got := ExpandFileRefs(ctx, root, "read @big.txt")
		if !strings.Contains(got, "unavailable") || strings.Contains(got, strings.Repeat("x", 300)) {
			t.Errorf("oversized file must be noted, not attached, got %q", got)
		}
	})
	t.Run("directory-noted-not-attached", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(root, "subdir"), 0o700); err != nil {
			t.Fatal(err)
		}
		got := ExpandFileRefs(ctx, root, "read @subdir")
		if !strings.Contains(got, "unavailable") {
			t.Errorf("directory reference must be noted, got %q", got)
		}
	})
	t.Run("no-refs-returns-text-unchanged", func(t *testing.T) {
		if got := ExpandFileRefs(ctx, root, "plain text"); got != "plain text" {
			t.Errorf("plain text changed: %q", got)
		}
	})
	t.Run("escaped-space-path-expands", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, "design notes.md"), []byte("design"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := ExpandFileRefs(ctx, root, "see @design\\ notes.md now")
		want := "see @design\\ notes.md now\n\n[file: design notes.md]\ndesign"
		if got != want {
			t.Errorf("expanded = %q, want %q", got, want)
		}
	})
	t.Run("sensitive-path-refused-by-policy", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := ExpandFileRefs(ctx, root, "load @.env")
		if !strings.Contains(got, "unavailable") || !strings.Contains(got, "tool policy") {
			t.Errorf("sensitive reference must carry a policy note, got %q", got)
		}
		if strings.Contains(got, "SECRET=1") {
			t.Error("credential content must never be attached")
		}
	})
}

// TestFileRefTokensEscapedSpaces pins the "\\ " escape: a backslashed space
// stays inside one token (paths with spaces survive tokenization), while an
// unescaped space still ends the reference.
func TestFileRefTokensEscapedSpaces(t *testing.T) {
	toks := FileRefTokens("read @docs/design\\ notes.md now and @a.txt")
	if len(toks) != 2 || toks[0] != "docs/design notes.md" || toks[1] != "a.txt" {
		t.Fatalf("tokens = %v, want [docs/design notes.md a.txt]", toks)
	}
}

// TestWorkspaceFilesSensitivePathsFiltered pins the picker-side policy gate:
// the listing never offers a path the workspace tool policy refuses, and
// never descends into sensitive directories.
func TestWorkspaceFilesSensitivePathsFiltered(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".env")
	write(".env.local")
	write("credentials.json")
	write(".ssh/id_ed25519")
	write(".aws/config")
	write(".config/gcloud/credentials.db")
	write(".config/allowed.txt") // .config itself is not sensitive
	write("keep.txt")

	files := WorkspaceFiles(context.Background(), root)
	set := map[string]bool{}
	for _, f := range files {
		set[f] = true
	}
	for _, banned := range []string{".env", ".env.local", "credentials.json", ".ssh/id_ed25519", ".aws/config", ".config/gcloud/credentials.db"} {
		if set[banned] {
			t.Errorf("listing offered the sensitive path %q: %v", banned, files)
		}
	}
	for _, want := range []string{"keep.txt", ".config/allowed.txt"} {
		if !set[want] {
			t.Errorf("missing %q in %v", want, files)
		}
	}
}
