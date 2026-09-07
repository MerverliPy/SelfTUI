package agent

import (
	"context"
	"encoding/json"
	"errors"
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
