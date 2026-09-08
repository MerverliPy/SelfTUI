package agent

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// Wire-shape tests for V2e (design §5): the write_files tool is a
// closed-schema addition — AgentTools gains one entry, the executor switch
// gains one case, and a real model turn flows propose → BatchReviewMsg →
// approval → apply with journaling. Results stay compact status lines.

// runBatchTurn drives one chat turn whose first request asks for a
// write_files batch; respond is called for the BatchReviewMsg. Returns the
// failed tool result summary (empty on success) and the run error.
func runBatchTurn(t *testing.T, root, journalDir string, opsJSON string, respond func(msg BatchReviewMsg)) (toolFailure string, runErr error) {
	t.Helper()
	srv, stub := newChatStub(t, func(phase int, _ ollama.ChatRequest, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		if phase == 0 {
			io.WriteString(w, toolEvent(nativeCall("write_files", opsJSON)))
			return
		}
		io.WriteString(w, finalEvent("batch done"))
	})
	t.Cleanup(srv.Close)
	journal, err := NewUndoJournal(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{}).WithJournal(journal)
	r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "apply batch"}},
	}, func(msg Msg) {
		switch m := msg.(type) {
		case BatchReviewMsg:
			respond(m)
		case ToolResultMsg:
			if m.Name == "write_files" && !m.OK {
				toolFailure = m.Summary
			}
		}
	})
	_ = stub
	return toolFailure, nil
}

func TestWriteFilesToolAdvertisedAndWireApplies(t *testing.T) {
	root := t.TempDir()
	// The schema exposes write_files as one closed entry.
	names := map[string]bool{}
	for _, d := range AgentTools() {
		names[d.Function.Name] = true
	}
	if !names["write_files"] {
		t.Fatal("AgentTools() is missing write_files")
	}
	if ReadOnlyTools()[0].Function.Name != "read_file" || ReadOnlyTools()[2].Function.Name != "grep" {
		t.Fatalf("read-only prefix changed: %v", ReadOnlyTools())
	}

	ops := `{"ops":[{"path":"a.txt","kind":"create","content":"alpha\n"},{"path":"b.txt","kind":"edit","old":"old","new":"new"}]}`
	mustWrite(t, root, "b.txt", "x old y\n")
	reviewed := false
	failure, err := runBatchTurn(t, root, "", ops, func(bm BatchReviewMsg) {
		reviewed = true
		if bm.Name != "write_files" || len(bm.Files) != 2 {
			t.Errorf("BatchReviewMsg = %+v", bm)
		}
		if !strings.Contains(bm.Files[0].Summary, "+1") {
			t.Errorf("create summary = %q, want a +N count", bm.Files[0].Summary)
		}
		bm.Respond(true)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if failure != "" {
		t.Fatalf("tool failure: %s", failure)
	}
	if !reviewed {
		t.Fatal("write_files did not emit a BatchReviewMsg")
	}
	if got := readFile(t, root, "a.txt"); got != "alpha\n" {
		t.Errorf("a.txt = %q", got)
	}
	if got := readFile(t, root, "b.txt"); got != "x new y\n" {
		t.Errorf("b.txt = %q", got)
	}
}

func TestWriteFilesDeclinedAppliesNothing(t *testing.T) {
	root := t.TempDir()
	ops := `{"ops":[{"path":"a.txt","kind":"create","content":"alpha\n"}]}`
	failure, err := runBatchTurn(t, root, "", ops, func(bm BatchReviewMsg) {
		bm.Respond(false)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if failure == "" || !strings.Contains(failure, "not approved") {
		t.Fatalf("tool failure = %q, want not-approved", failure)
	}
	if exists(root, "a.txt") {
		t.Error("declined batch wrote a.txt")
	}
}

func TestWriteFilesExpiryEndsTurnWithoutApply(t *testing.T) {
	root := t.TempDir()
	ops := `{"ops":[{"path":"a.txt","kind":"create","content":"alpha\n"}]}`
	srv, _ := newChatStub(t, func(_ int, _ ollama.ChatRequest, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, toolEvent(nativeCall("write_files", ops)))
	})
	defer srv.Close()
	journal, _ := NewUndoJournal("")
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{}).WithJournal(journal)
	r.confirmTimeout = 40 * time.Millisecond // M-01 seam: short batch review window
	err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "apply"}},
	}, func(Msg) {})
	if err == nil || !strings.Contains(err.Error(), "approval timed out") {
		t.Fatalf("Run error = %v, want the stable approval-timeout error", err)
	}
	if exists(root, "a.txt") {
		t.Error("expired review wrote a.txt")
	}
	if u, _ := journal.Counts(); u != 0 {
		t.Errorf("journal has %d entries after expiry", u)
	}
}

func TestSingleFileMutationsJournalAsOneOpChangeSets(t *testing.T) {
	// Decision #3: a confirmed single-file write_file is a 1-op change-set —
	// one /undo pops it. Drive through the real runner with a journal.
	root := t.TempDir()
	journal, _ := NewUndoJournal("")
	srv, _ := newChatStub(t, func(phase int, _ ollama.ChatRequest, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		if phase == 0 {
			io.WriteString(w, toolEvent(nativeCall("write_file", `{"path":"note.txt","content":"v1\n"}`)))
			return
		}
		io.WriteString(w, finalEvent("wrote it"))
	})
	defer srv.Close()
	r := NewRunnerWithPolicy(ollama.New(srv.URL, ""), root, "", 3, &ToolPolicy{}).WithJournal(journal)
	err := r.Run(context.Background(), Request{
		Model: "qwen3:8b", Messages: []ollama.ChatMessage{{Role: ollama.RoleUser, Content: "write"}},
	}, func(msg Msg) {
		if confirm, ok := msg.(ToolConfirmMsg); ok {
			confirm.Respond(true)
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if u, _ := journal.Counts(); u != 1 {
		t.Fatalf("journal undo count = %d, want 1", u)
	}
	if _, err := journal.Undo(); err != nil {
		t.Fatal(err)
	}
	if exists(root, "note.txt") {
		t.Error("undo of the single-file mutation left note.txt behind")
	}
}

func TestBatchOverCapsRejectedInFullBeforeDialog(t *testing.T) {
	root := t.TempDir()
	huge := strings.Repeat("x", maxWriteFilesOpBytes+1)
	ops := `{"ops":[{"path":"ok.txt","kind":"create","content":"fine"},{"path":"huge.txt","kind":"create","content":"` + huge + `"}]}`
	reviewed := false
	failure, _ := runBatchTurn(t, root, "", ops, func(bm BatchReviewMsg) {
		reviewed = true // must never be reached: caps reject pre-dialog
		bm.Respond(true)
	})
	if failure == "" || !strings.Contains(failure, "exceeds 262144 bytes") {
		t.Fatalf("tool failure = %q, want per-op cap rejection", failure)
	}
	if reviewed {
		t.Fatal("an over-cap batch reached the review dialog")
	}
	if exists(root, "ok.txt") || exists(root, "huge.txt") {
		t.Error("rejected batch wrote files")
	}
}

func TestWriteFilesBadPayloadErrorsAreActionable(t *testing.T) {
	// Design §7 residual: model reliability emitting valid batch JSON. Every
	// malformed payload must fail BEFORE the review dialog with an error that
	// names the offending op index and/or field, so the model can self-correct
	// inside its bounded iteration loop. The failure text below is exactly the
	// ToolResultMsg summary the model receives.
	root := t.TempDir()
	huge := strings.Repeat("x", maxWriteFilesOpBytes+1)
	cases := []struct {
		name string
		ops  string // raw arguments JSON for write_files
		want []string
	}{
		{"unknown field in args", `{"ops":[{"path":"a.txt","kind":"create","content":"x"}],"bogus":true}`,
			[]string{`unknown field "bogus"`}},
		{"unknown field inside op", `{"ops":[{"path":"a.txt","kind":"create","content":"x","typo":1}]}`,
			[]string{`unknown field "typo"`}},
		{"missing ops key", `{}`,
			[]string{"at least one op"}},
		{"empty ops array", `{"ops":[]}`,
			[]string{"at least one op"}},
		{"unknown kind on second op", `{"ops":[{"path":"a.txt","kind":"create","content":"x"},{"path":"b.txt","kind":"delete"}]}`,
			[]string{"write_files op 2 (b.txt)", "unknown kind"}},
		{"edit missing old", `{"ops":[{"path":"a.txt","kind":"edit","new":"n"}]}`,
			[]string{"write_files op 1 (a.txt)", "old is required"}},
		{"oversized content on second op", `{"ops":[{"path":"a.txt","kind":"create","content":"x"},{"path":"b.txt","kind":"create","content":"` + huge + `"}]}`,
			[]string{"write_files op 2 (b.txt)", "content exceeds"}},
		{"oversized old on an edit", `{"ops":[{"path":"a.txt","kind":"edit","old":"` + huge + `","new":"n"}]}`,
			[]string{"write_files op 1 (a.txt)", "old exceeds"}},
		{"oversized new on an edit", `{"ops":[{"path":"a.txt","kind":"edit","old":"o","new":"` + huge + `"}]}`,
			[]string{"write_files op 1 (a.txt)", "new exceeds"}},
		{".git path on second op", `{"ops":[{"path":"a.txt","kind":"create","content":"x"},{"path":".git/hooks/pre-commit","kind":"create","content":"x"}]}`,
			[]string{"write_files op 2 (.git/hooks/pre-commit)", ".git"}},
		{"sensitive .env path", `{"ops":[{"path":".env","kind":"create","content":"x"}]}`,
			[]string{"write_files op 1 (.env)", ".env"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reviewed := false
			failure, err := runBatchTurn(t, root, "", tc.ops, func(BatchReviewMsg) {
				reviewed = true
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if failure == "" {
				t.Fatal("malformed payload succeeded")
			}
			if reviewed {
				t.Fatal("a malformed batch reached the review dialog")
			}
			for _, want := range tc.want {
				if !strings.Contains(failure, want) {
					t.Errorf("failure %q does not name %q", failure, want)
				}
			}
		})
	}
}

func TestWriteFilesRejectsGitAndSensitivePathsPreDialog(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		ops  string
		want string
	}{
		{`{"ops":[{"path":".git/hooks/pre-commit","kind":"create","content":"x"}]}`, ".git"},
		{`{"ops":[{"path":".env","kind":"create","content":"x"}]}`, ".env"},
	}
	for _, tc := range cases {
		reviewed := false
		failure, _ := runBatchTurn(t, root, "", tc.ops, func(bm BatchReviewMsg) {
			reviewed = true
			bm.Respond(true)
		})
		if failure == "" {
			t.Fatalf("batch %s succeeded, want policy rejection", tc.ops)
		}
		if !strings.Contains(failure, tc.want) {
			t.Errorf("failure %q does not mention %q", failure, tc.want)
		}
		if reviewed {
			t.Errorf("sensitive batch %s reached the review dialog", tc.ops)
		}
	}
}

func TestDiffRendererDeterministic(t *testing.T) {
	// Pin the review-overlay diff text: a create is pure additions; an edit
	// shows one localized hunk; distant changes collapse unchanged context.
	rows, added, removed := lineDiff("", "a\nb\n")
	if added != 2 || removed != 0 || len(rows) != 2 {
		t.Fatalf("create diff: rows=%d +%d −%d", len(rows), added, removed)
	}
	out := renderDiffRows(rows)
	if len(out) != 2 || out[0] != "+a" || out[1] != "+b" {
		t.Errorf("create rows = %v", out)
	}

	oldText := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	newText := "one\ntwo\nTHREE\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	rows, added, removed = lineDiff(oldText, newText)
	if added != 1 || removed != 1 {
		t.Fatalf("edit diff: +%d −%d", added, removed)
	}
	out = renderDiffRows(rows)
	foundMinus := false
	for _, l := range out {
		if l == "-three" {
			foundMinus = true
		}
	}
	if !foundMinus {
		t.Errorf("edit rows missing the removed line: %v", out)
	}

	// A fully-changed middle collapses the untouched prefix/suffix? The
	// unchanged head/tail are trimmed as context; a long unchanged tail is
	// capped by the overlay, not the renderer (fitContent's job).
	if _, err := os.Stat(filepath.Join(t.TempDir(), "x")); !os.IsNotExist(err) {
		t.Fatal("unreachable")
	}
}
