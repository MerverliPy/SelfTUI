package agent

import (
	"context"
	"encoding/json"
	"errors"
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

func requireBubblewrap(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/usr/bin/bwrap"); err != nil {
		t.Skipf("V2c integration test requires /usr/bin/bwrap: %v", err)
	}
}

func TestRunCommandRejectsUnsafeArgv(t *testing.T) {
	root := t.TempDir()
	for _, argv := range [][]string{
		nil,
		{"sh", "-c", "echo nope"},
		{"python3", "-c", "print(1)"},
		{"go", "test;whoami"},
		{"go", "test", "/tmp"},
		{"git", "-C", "../outside", "status"},
		{"make", "test"},
	} {
		if _, err := RunCommand(context.Background(), root, argv, 1, nil); err == nil {
			t.Errorf("RunCommand(%q) unexpectedly succeeded", argv)
		}
	}
}

func TestRunCommandWaitsForSingleFlightSlotWithCancellation(t *testing.T) {
	commandSlot <- struct{}{}
	defer releaseCommand()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := RunCommand(ctx, t.TempDir(), []string{"go", "version"}, 1, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunCommand while slot is held = %v, want context deadline", err)
	}
}

func TestRunCommandUsesBubblewrapAndScrubsEnvironment(t *testing.T) {
	requireBubblewrap(t)
	root := t.TempDir()
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-reach-child")
	result, err := RunCommand(context.Background(), root, []string{"go", "env", "-json", "GOCACHE", "GOPATH", "GOMODCACHE", "GOPROXY"}, 5, nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if strings.Contains(result, "AWS_SECRET_ACCESS_KEY") || strings.Contains(result, "must-not-reach-child") {
		t.Fatalf("secret reached child output: %q", result)
	}
	for _, want := range []string{"/tmp/selftui-gocache", "/tmp/selftui-gopath", "/modcache", `"GOPROXY": "off"`} {
		if !strings.Contains(result, want) {
			t.Errorf("environment output %q does not contain %q", result, want)
		}
	}
}

func TestCommandOutputCapsEachStream(t *testing.T) {
	out := &commandOutput{stream: "stdout"}
	if _, err := out.Write([]byte(strings.Repeat("x", maxCommandOutput+1))); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if len(got) != maxCommandOutput+len("\n[output truncated]") || !strings.HasSuffix(got, "[output truncated]") {
		t.Fatalf("capped output length=%d suffix=%q", len(got), got[len(got)-20:])
	}
}

func TestRunCommandExecutesOfflineGoTestInsideSandbox(t *testing.T) {
	requireBubblewrap(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module commandtest\n\ngo 1.25.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello_test.go"), []byte("package commandtest\nimport \"testing\"\nfunc TestHello(t *testing.T) {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RunCommand(context.Background(), root, []string{"go", "test", "-count=1", "./..."}, 30, nil)
	if err != nil {
		t.Fatalf("sandboxed go test: %v\n%s", err, result)
	}
}

func TestRunCommandAllowsReadOnlyGit(t *testing.T) {
	requireBubblewrap(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunCommand(context.Background(), root, []string{"git", "status", "--short"}, 5, nil)
	if err != nil {
		t.Fatalf("sandboxed git status: %v\n%s", err, result)
	}
}

func TestRunnerConfirmsAndExecutesSandboxedCommand(t *testing.T) {
	requireBubblewrap(t)
	root := t.TempDir()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req ollama.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		if calls == 1 {
			io.WriteString(w, toolEvent(nativeCall("run_command", `{"argv":["go","version"],"timeout":1}`)))
			return
		}
		io.WriteString(w, finalEvent("command completed"))
	}))
	t.Cleanup(srv.Close)

	confirmed := false
	var output string
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{})
	err := r.Run(context.Background(), Request{Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "check the Go version"}}}, func(msg Msg) {
		switch m := msg.(type) {
		case ToolConfirmMsg:
			confirmed = m.Name == "run_command" && m.Workspace == root && m.Timeout == time.Second
			m.Respond(true)
		case ToolOutputMsg:
			output += m.Text
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !confirmed || !strings.Contains(output, "go version") || calls != 2 {
		t.Fatalf("confirmed=%v output=%q chat calls=%d", confirmed, output, calls)
	}
}

func TestRunCommandEnforcesTimeout(t *testing.T) {
	requireBubblewrap(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module commandtest\n\ngo 1.25.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "slow_test.go"), []byte("package commandtest\nimport \"testing\"\nimport \"time\"\nfunc TestSlow(t *testing.T) { time.Sleep(10*time.Second) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err := RunCommand(context.Background(), root, []string{"go", "test", "-count=1", "./..."}, 1, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("timeout took %s, want prompt termination", elapsed)
	}
}

func TestRunCommandCancellationKillsSandboxProcessGroup(t *testing.T) {
	requireBubblewrap(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module commandtest\n\ngo 1.25.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "slow_test.go"), []byte("package commandtest\nimport \"testing\"\nimport \"time\"\nfunc TestSlow(t *testing.T) { time.Sleep(10*time.Second) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := RunCommand(ctx, root, []string{"go", "test", "-count=1", "./..."}, 30, nil)
		done <- err
	}()
	time.Sleep(250 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("error = %v, want context canceled", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("sandboxed command did not return after cancellation")
	}
}

// TestValidateCommandRejectsGitWriteAndExecOptions pins the read-only git
// promise: git options that write files (--output, honored by the diff
// machinery shared by diff/log/show) or that make git execute externally
// configured helpers (--ext-diff, --textconv) must be rejected by the
// validator itself, along with the config/directory override spellings
// (-c, --config*, --git-dir*, --work-tree*) wherever they appear.
func TestValidateCommandRejectsGitWriteAndExecOptions(t *testing.T) {
	tests := [][]string{
		// cB-002: the --output write sink, in every spelling git accepts.
		{"git", "diff", "--output=generated.patch"},
		{"git", "diff", "--output", "generated.patch"},
		{"git", "log", "-1", "-p", "--output=log.patch"},
		{"git", "show", "--stat", "--output=show.patch"},
		{"git", "diff", "--out=generated.patch"},       // abbreviated spelling
		{"git", "log", "--output-indicator-new=+"},     // sibling of --output, still not allowlisted
		{"git", "diff", "--output=../generated.patch"}, // also fails path-escape rules
		// cB-003: external-helper execution switches.
		{"git", "diff", "--ext-diff"},
		{"git", "log", "--ext-diff"},
		{"git", "show", "--textconv"},
		{"git", "diff", "--textconv"},
		{"git", "grep", "--textconv", "data"},
		{"git", "show", "--textc"}, // abbreviated spelling
		// Existing config/work-tree/directory override families. In the
		// subcommand slot (argv[1]) they are not an allowlisted subcommand;
		// after the subcommand they are refused by the deny-by-default policy.
		{"git", "-c", "core.autocrlf=false", "status"},
		{"git", "-cfoo.bar=baz", "status"},
		{"git", "--git-dir=/tmp", "status"},
		{"git", "--work-tree", "/tmp", "status"},
		{"git", "status", "-c", "user.name=x"},
		{"git", "status", "-cfoo.bar=baz"},
		{"git", "status", "--git-dir=/tmp"},
		{"git", "diff", "--work-tree=/tmp"},
		{"git", "log", "--config-env=core.pager", "--oneline"},
		// Unlisted options of any kind are rejected rather than passed through.
		{"git", "log", "--pretty=email", "--output=x"},
		{"git", "status", "-p"},
	}
	for _, argv := range tests {
		if err := validateCommand(argv); err == nil {
			t.Errorf("validateCommand(%q) unexpectedly accepted", argv)
		}
	}
}

// TestValidateCommandAcceptsReadOnlyGitDiagnostics is the compatibility table:
// the ordinary read-only git diagnostics the model relies on must keep passing
// the validator, so the tightened option policy does not over-reject (cB-004).
func TestValidateCommandAcceptsReadOnlyGitDiagnostics(t *testing.T) {
	tests := [][]string{
		// git status.
		{"git", "status"},
		{"git", "status", "--short"},
		{"git", "status", "-s"},
		{"git", "status", "--short", "--branch"},
		{"git", "status", "--short", "internal/agent/command.go"},
		// git diff (no output flag), stat/word-diff variants, revisions and
		// pathspec operands.
		{"git", "diff"},
		{"git", "diff", "--stat"},
		{"git", "diff", "--stat", "HEAD~1", "HEAD"},
		{"git", "diff", "--word-diff"},
		{"git", "diff", "--name-only"},
		{"git", "diff", "HEAD"},
		{"git", "diff", "--name-only", "HEAD~1", "HEAD"},
		{"git", "diff", "HEAD~1", "HEAD", "--", "internal/agent/command.go"},
		{"git", "diff", "--stat", "--", "internal/agent/command.go"},
		// git log: oneline, -n/-<count>/-n<count> with and without other
		// options, --format (attached value), --grep/--since/--author with
		// attached or separate values, pathspec after --.
		{"git", "log", "--oneline"},
		{"git", "log", "-n", "5"},
		{"git", "log", "--oneline", "-n", "5"},
		{"git", "log", "-5"},
		{"git", "log", "-n5"},
		{"git", "log", "-1", "--oneline"},
		{"git", "log", "--format=%h %s"},
		{"git", "log", "--grep", "c1", "--oneline"},
		{"git", "log", "--since=2.weeks", "--oneline"},
		{"git", "log", "--author", "Ada", "--oneline"},
		{"git", "log", "--all", "--oneline"},
		{"git", "log", "--graph", "--oneline"},
		{"git", "log", "-p", "-1"},
		{"git", "log", "--name-only", "-1"},
		{"git", "log", "--oneline", "--", "internal/agent/command.go"},
		// git show <rev> and <rev>:<file>.
		{"git", "show", "HEAD"},
		{"git", "show", "HEAD:README.md"},
		{"git", "show", "HEAD:internal/agent/command.go"},
		{"git", "show", "--stat", "HEAD"},
		{"git", "show", "--format=%h", "HEAD"},
		// git branch.
		{"git", "branch"},
		{"git", "branch", "-a"},
		{"git", "branch", "-vv"},
		// git rev-parse.
		{"git", "rev-parse", "HEAD"},
		{"git", "rev-parse", "--abbrev-ref", "HEAD"},
		{"git", "rev-parse", "--short", "HEAD"},
		{"git", "rev-parse", "--show-toplevel"},
		{"git", "rev-parse", "--verify", "HEAD"},
		// git ls-files (including the -c that must not be confused with the
		// git global config option, which cannot appear here anyway).
		{"git", "ls-files"},
		{"git", "ls-files", "-c"},
		{"git", "ls-files", "--cached", "internal/agent/command.go"},
		{"git", "ls-files", "--others", "--exclude-standard"},
		// git grep.
		{"git", "grep", "-n", "TODO"},
		{"git", "grep", "-n", "func", "--", "internal"},
		{"git", "grep", "-n", "-i", "func", "internal/agent/command.go"},
		{"git", "grep", "-n", "-e", "TODO", "--", "internal/agent"},
		{"git", "grep", "-c", "func"},
		// The two helper-execution negations are accepted on the diff family
		// (they are also forced at execution time by applyReadOnlyGitGuards),
		// while the enabling forms --textconv/--ext-diff stay rejected above.
		{"git", "diff", "--no-textconv"},
		{"git", "diff", "--no-ext-diff", "--no-textconv"},
		{"git", "log", "--no-textconv", "-1", "-p"},
		{"git", "log", "--no-ext-diff", "-p", "-1"},
		{"git", "show", "--no-textconv", "HEAD"},
		{"git", "show", "--no-ext-diff", "--stat", "HEAD"},
	}
	for _, argv := range tests {
		if err := validateCommand(argv); err != nil {
			t.Errorf("validateCommand(%q) rejected a read-only diagnostic: %v", argv, err)
		}
	}
}

// TestApplyReadOnlyGitGuards pins the execution-level forcing that closes
// git's default-on helper-execution channels (textconv filters and external
// diff drivers): every diff-family invocation gets both negations inserted
// right after the subcommand, other subcommands and executables pass through
// untouched, and the guarded argv always re-validates cleanly.
func TestApplyReadOnlyGitGuards(t *testing.T) {
	tests := []struct{ in, want []string }{
		{[]string{"git", "diff", "--stat"}, []string{"git", "diff", "--no-textconv", "--no-ext-diff", "--stat"}},
		{[]string{"git", "diff", "--no-textconv"}, []string{"git", "diff", "--no-textconv", "--no-ext-diff", "--no-textconv"}},
		{[]string{"git", "diff", "--", "internal/agent/command.go"}, []string{"git", "diff", "--no-textconv", "--no-ext-diff", "--", "internal/agent/command.go"}},
		{[]string{"git", "log", "--oneline", "-n", "5"}, []string{"git", "log", "--no-textconv", "--no-ext-diff", "--oneline", "-n", "5"}},
		{[]string{"git", "show", "HEAD:README.md"}, []string{"git", "show", "--no-textconv", "--no-ext-diff", "HEAD:README.md"}},
		{[]string{"git", "status", "--short"}, []string{"git", "status", "--short"}},
		{[]string{"git", "branch", "-a"}, []string{"git", "branch", "-a"}},
		{[]string{"git", "rev-parse", "HEAD"}, []string{"git", "rev-parse", "HEAD"}},
		{[]string{"git", "grep", "-n", "TODO"}, []string{"git", "grep", "-n", "TODO"}},
		{[]string{"go", "test", "./..."}, []string{"go", "test", "./..."}},
	}
	for _, tt := range tests {
		got := applyReadOnlyGitGuards(tt.in)
		if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
			t.Errorf("applyReadOnlyGitGuards(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if err := validateCommand(got); err != nil {
			t.Errorf("guarded argv %q fails validation: %v", got, err)
		}
	}
}

// TestRunCommandDoesNotExecuteRepoConfiguredGitHelpers is the execution-level
// regression for the cB follow-up (Codex review P1, round 1): git enables
// textconv filters (by default for git-diff and git-log) and external diff
// drivers (git diff) WITHOUT any enabling flag when a repository's
// .gitattributes and .git/config define a driver for a path. A workspace can
// therefore carry a driver whose command mutates the writable /workspace
// mount, and an approved "read-only" git argv would silently execute it.
// applyReadOnlyGitGuards forces --no-textconv and --no-ext-diff onto every
// diff/log/show invocation inside the sandbox; this test arms BOTH a textconv
// driver and an external diff driver against a tracked binary file and asserts
// neither runs through the real sandboxed RunCommand path (git diff over an
// uncommitted change, then git log -p over a committed change pair).
//
// The trap is armed with sandbox-resident paths (/workspace/...) because the
// driver commands execute inside the bubblewrap mount namespace. If a future
// git stops enabling these helpers by default, the trap goes inert and the
// test asserts the still-correct guard without firing — which is why the
// assertion is marker absence (and a live internal diff), not output shape.
func TestRunCommandDoesNotExecuteRepoConfiguredGitHelpers(t *testing.T) {
	requireBubblewrap(t)
	root := t.TempDir()
	mustGit := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name string, body []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustGit("init", "-q")
	mustGit("config", "user.email", "t@selftui.test")
	mustGit("config", "user.name", "t")
	// Binary-ish content (NUL bytes) so the diff machinery would render it
	// through the configured textconv driver.
	write("payload.bin", []byte("payload v1\x00\x01\x02 binary-ish\n"))
	mustGit("add", "payload.bin")
	mustGit("commit", "-qm", "v1")
	// The version-controlled attribute selects the evil driver; the driver's
	// command itself lives in .git/config (not version controlled).
	write(".gitattributes", []byte("payload.bin diff=evil\n"))
	mustGit("add", ".gitattributes")
	mustGit("commit", "-qm", "attrs")
	// Both helper channels armed, pointing at a script inside the sandbox
	// mount. The script writes a marker into the writable /workspace and exits
	// cleanly (empty driver output is valid for both channels).
	helper := "#!/usr/bin/sh\necho executed >> /workspace/PWNED\nexit 0\n"
	// #!/usr/bin/sh (not #!/bin/sh): inside the sandbox /usr is ro-bound but
	// /bin is not mounted, so the shebang must resolve under /usr for the
	// helper to actually run when the guard is absent — which is what makes
	// this test a non-vacuous tripwire.
	if err := os.WriteFile(filepath.Join(root, "evil-helper.sh"), []byte(helper), 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit("config", "diff.evil.textconv", "/workspace/evil-helper.sh")
	mustGit("config", "diff.evil.command", "/workspace/evil-helper.sh")
	marker := filepath.Join(root, "PWNED")
	assertNoMarker := func(step string) {
		t.Helper()
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("%s: repository-configured git helper executed (marker %s exists)", step, marker)
		}
	}

	ctx := context.Background()
	// Leg A: git diff over an uncommitted change — textconv and the external
	// diff driver are both default-on for git diff.
	write("payload.bin", []byte("payload v2\x00\x01\x02 changed\n"))
	out, err := RunCommand(ctx, root, []string{"git", "diff", "--", "payload.bin"}, 30, nil)
	if err != nil {
		t.Fatalf("sandboxed git diff: %v\n%s", err, out)
	}
	assertNoMarker("git diff (uncommitted change)")
	if out == "" {
		t.Error("git diff produced no output: guards broke the read-only diagnostic")
	}

	// Leg B: git log -p over a committed change pair — textconv is default-on
	// for git log.
	mustGit("add", "payload.bin")
	mustGit("commit", "-qm", "v2")
	out, err = RunCommand(ctx, root, []string{"git", "log", "-p", "-1"}, 30, nil)
	if err != nil {
		t.Fatalf("sandboxed git log -p: %v\n%s", err, out)
	}
	assertNoMarker("git log -p (committed pair)")
	if out == "" {
		t.Error("git log -p produced no output: guards broke the read-only diagnostic")
	}
}
