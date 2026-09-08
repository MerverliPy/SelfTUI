package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
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
	srv, stub := newChatStub(t, func(phase int, _ ollama.ChatRequest, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		if phase == 0 {
			io.WriteString(w, toolEvent(nativeCall("write_file", `{"path":"note.txt","content":"approved"}`)))
			return
		}
		io.WriteString(w, finalEvent("written"))
	})

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
	if !confirmed || stub.requests() != 2 {
		t.Fatalf("confirmed=%v requests=%d", confirmed, stub.requests())
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

// TestWriteConfirmExpiresWithoutResponse is the M-01 regression: a mutation
// approval that receives no answer within its window must not stall the turn
// (or, worse, stall forever). The runner's approval window is injected through
// the per-Runner confirmTimeout seam (never a package-global mutable hook);
// nobody responds, so the turn must end with the stable approval-timeout
// error — not with the surrounding context deadline — having emitted exactly
// one terminal done result and written nothing.
func TestWriteConfirmExpiresWithoutResponse(t *testing.T) {
	root := t.TempDir()
	srv, stub := newChatStub(t, func(_ int, _ ollama.ChatRequest, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		// Every chat request asks for the same write. With the expiry fix the
		// runner gives up on its own after the injected window, so the server
		// must never see a second logical request.
		io.WriteString(w, toolEvent(nativeCall("write_file", `{"path":"note.txt","content":"late"}`)))
	})

	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 4, &ToolPolicy{})
	r.confirmTimeout = 50 * time.Millisecond // injected short approval window

	// The harness itself stays time-bounded: before the M-01 timer existed the
	// approval select only woke on this deadline with the wrong error.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dones := 0
	err := r.Run(ctx, Request{Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "write the note"}}}, func(msg Msg) {
		if _, ok := msg.(AgentDoneMsg); ok {
			dones++
		}
	})
	if err == nil || !strings.Contains(err.Error(), "approval timed out") {
		t.Fatalf("Run error = %v, want the stable approval-timeout error", err)
	}
	if dones != 1 {
		t.Fatalf("terminal done results = %d, want exactly one", dones)
	}
	if stub.requests() != 1 {
		t.Fatalf("chat calls = %d, want 1 (the runner must not retry after expiry)", stub.requests())
	}
	if _, err := os.Stat(filepath.Join(root, "note.txt")); !os.IsNotExist(err) {
		t.Fatalf("expired approval wrote the file: %v", err)
	}
}
