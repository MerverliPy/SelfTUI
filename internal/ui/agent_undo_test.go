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
	// Page 1 of 2 shows the first file (a.txt create) with the page indicator.
	for _, want := range []string{"Review write_files batch", "file 1/2", "a.txt", "hello", "y / enter apply all · n / esc decline · pgup/pgdn · ↑/↓ or j/k"} {
		if !strings.Contains(out, want) {
			t.Errorf("batch overlay missing %q:\n%s", want, out)
		}
	}
	if !v.ModalOpen() {
		t.Fatal("a pending batch review must block modal input")
	}

	// pgdn pages to file 2/2 (the b.txt edit); the second file is reachable
	// even though page 1 rendered only the first (V2e residual: paging).
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if v.batchPage != 1 {
		t.Fatalf("pgdn did not page: batchPage = %d, want 1", v.batchPage)
	}
	out = stripANSI(v.View())
	for _, want := range []string{"file 2/2", "b.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("page 2 missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "a.txt") {
		t.Errorf("page 2 must show only the second file, still showing a.txt:\n%s", out)
	}

	// Mobile-safe aliases (Codex P1): k pages back, Down pages forward —
	// the same ↑/↓ or j/k pattern every other paged view accepts.
	v, _ = v.Update(tea.KeyPressMsg{Text: "k"})
	if v.batchPage != 0 {
		t.Fatalf("k did not page back: batchPage = %d, want 0", v.batchPage)
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if v.batchPage != 1 {
		t.Fatalf("Down did not page forward: batchPage = %d, want 1", v.batchPage)
	}

	// y approves the whole batch (all pages); the runner applies both files.
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

// TestBatchReviewPagesClamp: pgup/pgdn page the review and clamp at both
// ends; the modal stays open on a clamped key.
func TestBatchReviewPagesClamp(t *testing.T) {
	v := pendingBatchReviewView(t) // 2 files
	if v.batchPage != 0 {
		t.Fatalf("fresh review batchPage = %d, want 0", v.batchPage)
	}
	// pgup on page 1 clamps to page 1.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if v.batchPage != 0 || v.batchReview == nil {
		t.Fatalf("pgup on page 1 must clamp: page=%d modal=%v", v.batchPage, v.batchReview != nil)
	}
	// pgdn to the last page, then pgdn clamps there.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if v.batchPage != 1 || v.batchReview == nil {
		t.Fatalf("pgdn past the last page must clamp: page=%d modal=%v", v.batchPage, v.batchReview != nil)
	}
}

// TestBatchReviewNoteOnFirstPageOnly: the batch note renders on page 1 and
// is dropped when the review pages past it.
func TestBatchReviewNoteOnFirstPageOnly(t *testing.T) {
	v := pendingBatchReviewView(t)
	v.batchReview.Note = "rename everything"
	out := stripANSI(v.View())
	if !strings.Contains(out, "note: rename everything") {
		t.Fatalf("page 1 missing the batch note:\n%s", out)
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	out = stripANSI(v.View())
	if strings.Contains(out, "note: rename everything") {
		t.Errorf("page 2 must not repeat the page-1 note:\n%s", out)
	}
}

// TestBatchReviewModalOwnsKeys: while the review is open every other key is
// swallowed — a letter that would open the picker or a digit that would jump
// tabs must not escape the modal.
func TestBatchReviewModalOwnsKeys(t *testing.T) {
	v := pendingBatchReviewView(t)
	for _, k := range []tea.KeyPressMsg{
		{Text: "m"}, // model picker hotkey
		{Text: "3"}, // digit tab jump
		{Text: "f"}, // follow toggle
	} {
		v, _ = v.Update(k)
	}
	if v.batchReview == nil || !v.ModalOpen() {
		t.Fatal("stray keys must leave the batch review open")
	}
	if v.selectorOpen || v.batchPage != 0 {
		t.Fatalf("stray keys escaped the modal (selector=%v page=%d)", v.selectorOpen, v.batchPage)
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

// TestBatchReviewEmptyFilesRendersDeclineShell pins the reviewer P1 fix
// (V2e residual session): a BatchReviewMsg with an empty Files slice must
// render the decline shell, never panic on the page clamp + index. The
// runner refuses empty batches at proposal, so this is purely defensive.
func TestBatchReviewEmptyFilesRendersDeclineShell(t *testing.T) {
	v := testAgent(t, nil)
	v.batchReview = &agent.BatchReviewMsg{
		Name:      "write_files",
		Workspace: "/tmp",
		Timeout:   120 * time.Second,
		Files:     nil,
	}
	out := v.View()
	if !strings.Contains(out, "empty batch — nothing to apply") ||
		!strings.Contains(out, "n / esc decline") {
		t.Errorf("empty batch overlay missing decline shell:\n%s", out)
	}
	if strings.Contains(out, "file 1/0") {
		t.Errorf("empty batch must not show a page indicator:\n%s", out)
	}
	// Keys stay modal-owned and harmless on the empty shell.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if v.batchPage != 0 {
		t.Errorf("pgdn on an empty batch moved the page: %d", v.batchPage)
	}
	for _, key := range []tea.KeyPressMsg{{Text: "j"}, {Text: "k"}, {Code: tea.KeyDown}, {Code: tea.KeyUp}} {
		v, _ = v.Update(key)
		if v.batchPage != 0 {
			t.Errorf("alias %v on an empty batch moved the page: %d", key, v.batchPage)
		}
	}
	if v.batchReview == nil {
		t.Fatal("paging keys must not dismiss the review")
	}
}

// TestBatchReviewAliasPagingWalksAllFiles pins the Codex P1 fix: the j/k and
// ↑/↓ aliases (the repo-wide paged-view pattern) reach every file of the
// batch without PageUp/PageDown, so a phone keyboard can inspect files 2..N
// before y applies the whole set. Aliases clamp at both ends.
func TestBatchReviewAliasPagingWalksAllFiles(t *testing.T) {
	v := testAgent(t, nil)
	mk := func(path string) agent.BatchFileReview {
		return agent.BatchFileReview{Path: path, Kind: "create", Summary: "A " + path, Rows: []string{"+one"}}
	}
	v.batchReview = &agent.BatchReviewMsg{
		Name:      "write_files",
		Workspace: "/tmp",
		Timeout:   120 * time.Second,
		Files:     []agent.BatchFileReview{mk("1.go"), mk("2.go"), mk("3.go"), mk("4.go")},
	}
	// j alone walks page 1 to the last page.
	for want := 1; want <= 3; want++ {
		v, _ = v.Update(tea.KeyPressMsg{Text: "j"})
		if v.batchPage != want {
			t.Fatalf("j walk: batchPage = %d, want %d", v.batchPage, want)
		}
	}
	// j clamps at the last page; the modal stays open.
	v, _ = v.Update(tea.KeyPressMsg{Text: "j"})
	if v.batchPage != 3 || v.batchReview == nil {
		t.Fatalf("j past the last page must clamp: page=%d modal=%v", v.batchPage, v.batchReview != nil)
	}
	// Up arrow and k walk back to page 1.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	v, _ = v.Update(tea.KeyPressMsg{Text: "k"})
	if v.batchPage != 0 {
		t.Fatalf("Up/k walk back: batchPage = %d, want 0", v.batchPage)
	}
	if out := stripANSI(v.View()); !strings.Contains(out, "file 1/4") {
		t.Errorf("back on page 1, missing indicator:\n%s", out)
	}
}

// TestBatchReviewSingleFileAliasesHarmless: a one-file batch has nothing to
// page, so the legend must not advertise paging and every paging key (alias
// or not) is a no-op that keeps the review pending.
func TestBatchReviewSingleFileAliasesHarmless(t *testing.T) {
	v := testAgent(t, nil)
	v.batchReview = &agent.BatchReviewMsg{
		Name:      "write_files",
		Workspace: "/tmp",
		Timeout:   120 * time.Second,
		Files:     []agent.BatchFileReview{{Path: "only.go", Kind: "create", Summary: "A only.go", Rows: []string{"+one"}}},
	}
	for _, key := range []tea.KeyPressMsg{
		{Text: "j"}, {Text: "k"}, {Code: tea.KeyUp}, {Code: tea.KeyDown},
		{Code: tea.KeyPgUp}, {Code: tea.KeyPgDown},
	} {
		v, _ = v.Update(key)
		if v.batchPage != 0 || v.batchReview == nil {
			t.Fatalf("single-file batch must ignore %v: page=%d modal=%v", key, v.batchPage, v.batchReview != nil)
		}
	}
	if out := stripANSI(v.View()); strings.Contains(out, "j/k") || strings.Contains(out, "pgup/pgdn") {
		t.Errorf("single-file legend must not advertise paging:\n%s", out)
	}
}
