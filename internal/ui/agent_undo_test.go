package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// V2e UI tests: the write_files review overlay (approve-all / decline), the
// /undo and /redo slash commands with their y/esc confirms, and the palette
// entries routing to the same flow.

// pendingBatchReviewView boots an Agent tab with a stub host that, on the
// first chat request, asks for a deterministic write_files batch. Returns the
// view once the review overlay is pending (ModalOpen with batchReview set).
func pendingBatchReviewView(t *testing.T) AgentView {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("x old y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			calls++
			w.Header().Set("Content-Type", "application/x-ndjson")
			if calls == 1 {
				io.WriteString(w, `{"message":{"role":"assistant","tool_calls":[{"function":{"name":"write_files","arguments":{"ops":[{"path":"a.txt","kind":"create","content":"hello\n"},{"path":"b.txt","kind":"edit","old":"old","new":"new"}]}}}]},"done":true}`+"\n")
				return
			}
			io.WriteString(w, chatEvent("batch complete", true)+"\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	v := newAgentTools(t, ollama.New(srv.URL, ""), srv.URL, root)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "write the batch")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	deadline := time.After(3 * time.Second)
	for v.batchReview == nil {
		select {
		case msg := <-v.chatCh:
			v, _ = v.Update(msg)
		case <-deadline:
			t.Fatal("batch review never arrived")
		}
	}
	return v
}

func TestBatchReviewOverlayRendersAndApproves(t *testing.T) {
	v := pendingBatchReviewView(t)
	out := stripANSI(v.View())
	for _, want := range []string{"Review write_files batch", "a.txt", "b.txt", "hello", "y / enter apply all · n / esc decline"} {
		if !strings.Contains(out, want) {
			t.Errorf("batch overlay missing %q:\n%s", want, out)
		}
	}
	if !v.ModalOpen() {
		t.Fatal("a pending batch review must block modal input")
	}

	// y approves the whole batch; the runner applies and finishes the turn.
	v, _ = v.Update(tea.KeyPressMsg{Text: "y"})
	if v.batchReview != nil {
		t.Fatal("approve did not clear the pending review")
	}
	drainChat(t, &v)
	root := v.runner.Root()
	if b, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(b) != "hello\n" {
		t.Errorf("approved batch did not create a.txt (%q, %v)", b, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "b.txt")); string(b) != "x new y\n" {
		t.Errorf("approved batch did not edit b.txt: %q", b)
	}
}

func TestBatchReviewDeclineAppliesNothing(t *testing.T) {
	v := pendingBatchReviewView(t)
	root := v.runner.Root()
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.batchReview != nil {
		t.Fatal("esc should clear the pending review")
	}
	if v.notice != "declined batch" {
		t.Errorf("notice = %q", v.notice)
	}
	drainChat(t, &v)
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Error("declined batch created a.txt")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "b.txt")); string(b) != "x old y\n" {
		t.Errorf("declined batch modified b.txt: %q", b)
	}
}

// journaledView builds an Agent tab with a journal backed by a temp dir and a
// recorded change-set on disk, so /undo and /redo have something to act on.
func journaledView(t *testing.T) (AgentView, string) {
	t.Helper()
	root := t.TempDir()
	v := testAgent(t, nil)
	undoDir := t.TempDir()
	v = v.WithUndoDir(undoDir)
	// Record a mutation directly into the journal (the runner records the
	// same shape through applyMutationSet).
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	j := v.undo
	entry, err := j.Prepare([]agent.FileRecord{{
		Requested: "file.txt",
		Path:      filepath.Join(root, "file.txt"),
		Mode:      0o600,
		Existed:   true,
		Pre:       []byte("v1\n"),
		Post:      []byte("v2\n"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	j.Commit(entry)
	v.undo = j
	return v, root
}

// runSlashUndo approves a pending /undo or /redo confirm and drives the
// returned journal command to completion on the view.
func runSlashUndo(t *testing.T, v *AgentView, redo bool) {
	t.Helper()
	cmd := "undo"
	if redo {
		cmd = "redo"
	}
	typeText(t, v, "/"+cmd)
	av, c := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	*v = av
	if (!redo && !av.undoConfirm) || (redo && !av.redoConfirm) {
		t.Fatalf("/%s did not ask for confirmation", cmd)
	}
	av, c = av.Update(tea.KeyPressMsg{Text: "y"})
	*v = av
	if c == nil {
		t.Fatal("undo confirm returned no command")
	}
	msg := c()
	if msg == nil {
		t.Fatal("undo command returned no message")
	}
	av, _ = av.Update(msg)
	*v = av
}

func TestSlashUndoAsksThenRuns(t *testing.T) {
	v, root := journaledView(t)
	typeText(t, &v, "/undo")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.undoConfirm || !v.ModalOpen() {
		t.Fatal("'/undo' should ask for confirmation first")
	}
	if !strings.Contains(stripANSI(v.View()), "undo the agent's last file change") {
		t.Fatalf("undo confirm dialog missing:\n%s", stripANSI(v.View()))
	}

	// n/esc cancels: file untouched.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.undoConfirm || v.notice != "undo cancelled" {
		t.Fatalf("esc should cancel undo (confirm=%v notice=%q)", v.undoConfirm, v.notice)
	}
	b, _ := os.ReadFile(filepath.Join(root, "file.txt"))
	if string(b) != "v2\n" {
		t.Errorf("cancelled undo modified the file: %q", b)
	}

	// y/enter runs the undo as a background command and the notice lands.
	runSlashUndo(t, &v, false)
	b, _ = os.ReadFile(filepath.Join(root, "file.txt"))
	if string(b) != "v1\n" {
		t.Errorf("undo did not restore pre-image: %q", b)
	}
	if !strings.Contains(v.notice, "undo") {
		t.Errorf("notice = %q, want an undo notice", v.notice)
	}
}

func TestSlashRedoRoundTrip(t *testing.T) {
	v, root := journaledView(t)
	runSlashUndo(t, &v, false)
	b, _ := os.ReadFile(filepath.Join(root, "file.txt"))
	if string(b) != "v1\n" {
		t.Fatalf("precondition: undo failed, file = %q", b)
	}
	runSlashUndo(t, &v, true)
	b, _ = os.ReadFile(filepath.Join(root, "file.txt"))
	if string(b) != "v2\n" {
		t.Errorf("redo did not restore post-image: %q", b)
	}
}

func TestUndoEmptyJournalIsNoticeNotDialog(t *testing.T) {
	v := testAgent(t, nil)
	v = v.WithUndoDir("")
	typeText(t, &v, "/undo")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.undoConfirm {
		t.Fatal("empty journal must not open the confirm dialog")
	}
	if v.notice != "nothing to undo" {
		t.Errorf("notice = %q, want nothing to undo", v.notice)
	}
}

func TestUndoRefuseGuardSurfacesNotice(t *testing.T) {
	v, root := journaledView(t)
	// An external edit makes the recorded post-image stale: /undo must refuse
	// visibly and leave both the file and the entry alone.
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("user edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	typeText(t, &v, "/undo")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	v, c := v.Update(tea.KeyPressMsg{Text: "y"})
	if c == nil {
		t.Fatal("undo confirm returned no command")
	}
	msg := c()
	v, _ = v.Update(msg)
	if !strings.Contains(v.notice, "undo refused") || !strings.Contains(v.notice, "file.txt") {
		t.Fatalf("notice = %q, want refusal naming file.txt", v.notice)
	}
	b, _ := os.ReadFile(filepath.Join(root, "file.txt"))
	if string(b) != "user edit\n" {
		t.Errorf("refused undo modified the file: %q", b)
	}
}

func TestPaletteUndoRoutesToAgentConfirm(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	// Seed a journaled change so undo has something to offer.
	v, root := journaledView(t)
	_ = root
	m.agent = v
	m = updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	for _, r := range "undo" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.paletteOpen {
		t.Fatal("enter should close the palette")
	}
	if m.tab != agentTab {
		t.Fatalf("tab = %d, want Agent", m.tab)
	}
	if !m.agent.undoConfirm {
		t.Fatal("palette undo should open the Agent undo confirm")
	}
}

// TestJournalLifecycleViaAppClose wires the undo dir through the App and
// verifies clean-exit GC removes crash artifacts.
func TestJournalLifecycleViaAppClose(t *testing.T) {
	undoDir := t.TempDir()
	// Simulate a crash artifact left by a previous run BEFORE the journal is
	// opened this session.
	crash := filepath.Join(undoDir, "entry-1-stale")
	if err := os.MkdirAll(crash, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crash, "meta.json"), []byte(`{"seq":1,"files":["a.txt"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Opening detects the stale entry (crash recovery report).
	v := testAgent(t, nil)
	v = v.WithUndoDir(undoDir)
	if len(v.undo.Stale()) != 1 {
		t.Fatalf("stale entries = %d, want 1", len(v.undo.Stale()))
	}
	if !strings.Contains(v.notice, "interrupted undo entries") {
		t.Errorf("notice = %q, want the crash report", v.notice)
	}
	// Close GCs the artifacts.
	if err := v.CloseUndo(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(undoDir); len(entries) != 0 {
		t.Errorf("clean close left %d artifacts", len(entries))
	}
}
