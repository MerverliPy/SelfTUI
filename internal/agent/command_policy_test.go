package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"selftui/internal/ollama"
)

func TestAgentToolsExposeSandboxedRunCommand(t *testing.T) {
	for _, def := range AgentTools() {
		if def.Function.Name == "run_command" {
			return
		}
	}
	t.Fatal("AgentTools() does not expose sandboxed run_command")
}

func TestRunnerRejectsUnknownCommandInsteadOfDispatching(t *testing.T) {
	r := NewRunner(nil, t.TempDir(), "", 1)
	call := ollama.ToolCall{Function: ollama.ToolCallFunction{
		Name: "shell", Arguments: json.RawMessage(`{"argv":["echo","nope"]}`),
	}}
	_, err := r.executeTool(context.Background(), call, func(msg Msg) {
		t.Fatalf("unknown tool emitted %T", msg)
	})
	if err == nil || err.Error() != `tool "shell" is not allowed` {
		t.Fatalf("executeTool(shell) error = %v", err)
	}
}

func TestEditFileStillRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(nil, root, "", 1)
	call := ollama.ToolCall{Function: ollama.ToolCallFunction{
		Name: "edit_file", Arguments: json.RawMessage(`{"path":"notes.txt","old":"original","new":"changed"}`),
	}}
	_, err := r.executeTool(context.Background(), call, func(msg Msg) {
		confirmation, ok := msg.(ToolConfirmMsg)
		if !ok {
			t.Fatalf("event = %T, want ToolConfirmMsg", msg)
		}
		confirmation.Respond(false)
	})
	if err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("declined edit error = %v, want not approved", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original\n" {
		t.Fatalf("file after declined edit = %q, %v (must be untouched)", got, err)
	}
}
