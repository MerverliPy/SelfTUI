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

func TestWriteAndEditStayInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := WriteFile(root, "notes.txt", "first\n", false); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := WriteFile(root, "notes.txt", "second", false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("non-overwrite error = %v", err)
	}
	if err := EditFile(root, "notes.txt", "first", "edited"); err != nil {
		t.Fatalf("EditFile: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil || string(got) != "edited\n" {
		t.Fatalf("contents = %q, %v", got, err)
	}
	if err := EditFile(root, "notes.txt", "missing", "replacement"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("exact-match error = %v", err)
	}
	if err := WriteFile(root, "../outside.txt", "no", false); err == nil {
		t.Fatal("WriteFile allowed workspace escape")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "outside-link")); err == nil {
		if err := WriteFile(root, "outside-link/no.txt", "no", false); err == nil || !strings.Contains(err.Error(), "escapes workspace") {
			t.Fatalf("symlink write error = %v", err)
		}
	}
}

func TestMutationToolsNeedExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/x-ndjson")
		if calls == 1 {
			io.WriteString(w, toolEvent(nativeCall("write_file", `{"path":"note.txt","content":"approved"}`)))
			return
		}
		io.WriteString(w, finalEvent("written"))
	}))
	t.Cleanup(srv.Close)

	confirmed := false
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 2, &ToolPolicy{})
	err := r.Run(context.Background(), Request{Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "write"}}}, func(msg Msg) {
		if confirmation, ok := msg.(ToolConfirmMsg); ok {
			confirmed = confirmation.Name == "write_file" && confirmation.Workspace == root
			confirmation.Respond(true)
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !confirmed || calls != 2 {
		t.Fatalf("confirmed=%v calls=%d", confirmed, calls)
	}
	got, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil || string(got) != "approved" {
		t.Fatalf("written contents = %q, %v", got, err)
	}
}

func TestDeclinedMutationDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	r := NewRunner(nil, root, "", 1)
	call := ollama.ToolCall{Function: ollama.ToolCallFunction{Name: "write_file", Arguments: json.RawMessage(`{"path":"note.txt","content":"no"}`)}}
	result, err := r.executeTool(context.Background(), call, func(msg Msg) {
		confirmation, ok := msg.(ToolConfirmMsg)
		if !ok {
			t.Fatalf("event = %T, want ToolConfirmMsg", msg)
		}
		confirmation.Respond(false)
	})
	if err == nil || !strings.Contains(err.Error(), "not approved") || result != "" {
		t.Fatalf("result=%q err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "note.txt")); !os.IsNotExist(err) {
		t.Fatalf("file exists after refusal: %v", err)
	}
}
