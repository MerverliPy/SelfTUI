package agent

// Security scope for v0.1 (2026-09-03 hardening decision): the public tool
// surface must not expose or retain command execution. run_command is not
// shipped — the schema must exclude it and the execution boundary must reject
// a model-supplied run_command call outright. write_file and edit_file keep
// their per-call confirmation gates.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"selftui/internal/ollama"
)

// TestAgentToolsExcludeRunCommand: the v0.1 schema must not expose run_command
// to any model. This is a closed list — a leftover entry would let a model
// request an executor that must not exist in the public release.
func TestAgentToolsExcludeRunCommand(t *testing.T) {
	for _, def := range AgentTools() {
		if def.Function.Name == "run_command" {
			t.Fatalf("AgentTools() exposes %q; v0.1 must not ship command execution", def.Function.Name)
		}
	}
}

// TestRunnerRejectsRunCommand: a model-supplied run_command call must be
// refused at the execution boundary with the exact not-allowed error — never
// dispatched to an executor, never offered for confirmation.
func TestRunnerRejectsRunCommand(t *testing.T) {
	r := NewRunner(nil, t.TempDir(), "", 1)
	call := ollama.ToolCall{Function: ollama.ToolCallFunction{
		Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`),
	}}
	_, err := r.executeTool(context.Background(), call, func(msg Msg) {
		// A live executor would emit a confirmation here; decline it so a
		// regression fails fast instead of hanging on the approval channel.
		if confirmation, ok := msg.(ToolConfirmMsg); ok {
			confirmation.Respond(false)
		}
	})
	if err == nil || err.Error() != `tool "run_command" is not allowed` {
		t.Fatalf("executeTool(run_command) error = %v, want exact error %q", err, `tool "run_command" is not allowed`)
	}
}

// TestEditFileStillRequiresExplicitConfirmation: after the run_command
// removal, edit_file must keep the same per-call approval gate as write_file
// — a decline aborts with "not approved" and leaves the file untouched.
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
