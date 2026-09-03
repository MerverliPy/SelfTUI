package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"selftui/internal/ollama"
)

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

func TestRunCommandScrubsEnvironmentAndUsesJailHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-reach-child")
	result, err := RunCommand(context.Background(), root, []string{"go", "env", "GOCACHE", "GOPATH"}, 5, nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if !strings.Contains(result, root+string(os.PathSeparator)+".selftui-gocache") || !strings.Contains(result, root+string(os.PathSeparator)+".selftui-gopath") {
		t.Fatalf("environment output = %q", result)
	}
	if env := strings.Join(commandEnv(root), "\n"); strings.Contains(env, "AWS_SECRET_ACCESS_KEY") || !strings.Contains(env, "HOME="+root+string(os.PathSeparator)+".selftui-home") {
		t.Fatalf("scrubbed environment = %q", env)
	}
}

func TestCommandOutputCapsEachStream(t *testing.T) {
	out := &commandOutput{stream: "stdout"}
	if _, err := out.Write([]byte(strings.Repeat("x", maxCommandOutput+1))); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); len(got) != maxCommandOutput+len("\n[output truncated]") || !strings.HasSuffix(got, "[output truncated]") {
		t.Fatalf("capped output length=%d suffix=%q", len(got), got[len(got)-20:])
	}
}

func TestRunCommandCancelsProcess(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/go.mod", []byte("module commandtest\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/slow_test.go", []byte("package commandtest\nimport \"testing\"\nimport \"time\"\nfunc TestSlow(t *testing.T) { time.Sleep(10*time.Second) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := RunCommand(ctx, root, []string{"go", "test", "./..."}, 30, nil)
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
		t.Fatal("command did not return after cancellation")
	}
}

func TestBudgetMessagesDropsOldTurns(t *testing.T) {
	messages := []chatMessageForTest{
		{role: "system", content: "system"},
		{role: "user", content: strings.Repeat("a", 160)},
		{role: "assistant", content: strings.Repeat("b", 160)},
		{role: "user", content: "latest"},
	}
	got := BudgetMessages(toOllamaMessages(messages), 64)
	if got[len(got)-1].Content != "latest" || strings.Contains(strings.Join(messageContents(got), ""), strings.Repeat("a", 160)) {
		t.Fatalf("BudgetMessages retained=%d last=%q", len(got), got[len(got)-1].Content)
	}
	if !strings.Contains(got[1].Content, "Earlier conversation omitted") {
		t.Fatalf("missing truncation marker: %+v", got)
	}
}

type chatMessageForTest struct{ role, content string }

func toOllamaMessages(in []chatMessageForTest) []ollama.ChatMessage {
	out := make([]ollama.ChatMessage, len(in))
	for i, msg := range in {
		out[i] = ollama.ChatMessage{Role: ollama.Role(msg.role), Content: msg.content}
	}
	return out
}

func messageContents(in []ollama.ChatMessage) []string {
	out := make([]string, len(in))
	for i, msg := range in {
		out[i] = msg.Content
	}
	return out
}
