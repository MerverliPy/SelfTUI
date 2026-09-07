package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/ollama"
	"selftui/internal/session"
)

// seedSavedTranscript records two turns through the real writer (round-trip via
// the production append path) and returns the session dir + file path.
func seedSavedTranscript(t *testing.T, dir string) string {
	t.Helper()
	l, err := session.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 7, 21, 31, 2, 0, time.UTC)
	if err := l.Append("user", "qwen3:8b", "explain this repo", "", at); err != nil {
		t.Fatal(err)
	}
	if err := l.Append("assistant", "qwen3:8b", "It is a terminal UI.", "0.4s · stop", at.Add(18*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return l.Path()
}

// runCmd executes one returned tea.Cmd (tests run commands by hand) and
// re-delivers the (envelope-wrapped) message through Update.
func runCmd(t *testing.T, v *AgentView, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("command returned nil")
	}
	*v, _ = v.Update(msg)
}

func TestAgentViewResumeImport(t *testing.T) {
	client, sent, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	path := seedSavedTranscript(t, dir)

	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "http://fake-host:11434")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.numCtx = 64 // a small budget so the recomputed meter is a visible percent

	typeText(t, &v, "/resume")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.resumeOpen {
		t.Fatalf("picker did not open: notice=%q", v.notice)
	}
	runCmd(t, &v, cmd)
	if len(v.resumeList) != 1 || v.resumeList[0].Path != path {
		t.Fatalf("resumeList = %+v, want the seeded transcript", v.resumeList)
	}
	v, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // pick row 0
	if v.resumeConfirm {
		t.Fatal("empty conversation must not ask for overwrite confirmation")
	}
	if v.resumeOpen {
		t.Fatal("picker should close on pick")
	}
	runCmd(t, &v, cmd) // the load command

	// Imported state: messages, model chips, footer meta, and the render
	// cache land together, in order.
	if len(v.turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(v.turns))
	}
	if v.turns[0].msg.Role != ollama.RoleUser || v.turns[0].msg.Content != "explain this repo" {
		t.Errorf("turn 0 = %+v", v.turns[0])
	}
	if v.turns[1].msg.Role != ollama.RoleAssistant || v.turns[1].msg.Content != "It is a terminal UI." {
		t.Errorf("turn 1 = %+v", v.turns[1])
	}
	if v.turns[0].model != "qwen3:8b" || v.turns[1].model != "qwen3:8b" {
		t.Errorf("turn models = %q/%q, want qwen3:8b", v.turns[0].model, v.turns[1].model)
	}
	if v.turns[1].meta != "0.4s · stop" {
		t.Errorf("turn meta = %q", v.turns[1].meta)
	}
	if v.turns[0].render == "" || v.turns[1].render == "" {
		t.Errorf("render cache not rebuilt: %+v", v.turns)
	}
	if !v.follow || v.scroll != 0 || v.truncated {
		t.Errorf("follow/scroll/truncated = %v/%d/%v, want true/0/false", v.follow, v.scroll, v.truncated)
	}
	if !strings.Contains(v.notice, "resumed 2 turns") {
		t.Errorf("notice = %q", v.notice)
	}
	// The picker's historical model names never switch the active model.
	if v.model != "qwen3:8b" {
		t.Errorf("model = %q, want the selected model unchanged", v.model)
	}
	// The context meter is derived from history, so it recomputed.
	if v.ctxPct() <= 0 {
		t.Errorf("ctx meter did not recompute after import (pct=%d)", v.ctxPct())
	}

	// The next send carries the imported conversation before the new text
	// (safe import: plain user/assistant history for the runner request).
	typeText(t, &v, "and the tools tab?")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	if len(*sent) != 1 {
		t.Fatalf("sent %d requests, want 1", len(*sent))
	}
	req := (*sent)[0]
	if len(req) != 3 ||
		req[0].Role != ollama.RoleUser || req[0].Content != "explain this repo" ||
		req[1].Role != ollama.RoleAssistant || req[1].Content != "It is a terminal UI." ||
		req[2].Role != ollama.RoleUser || req[2].Content != "and the tools tab?" {
		t.Errorf("next request did not include the imported history:\n%+v", req)
	}
}

func TestAgentViewResumeOverwriteConfirm(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	seedSavedTranscript(t, dir)

	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	// A live conversation exists: picking must ask first.
	v.turns = append(v.turns, turn{
		msg:    ollama.ChatMessage{Role: ollama.RoleUser, Content: "live turn"},
		render: "x",
	})

	typeText(t, &v, "/resume")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	runCmd(t, &v, cmd)
	v, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.resumeConfirm {
		t.Fatal("non-empty conversation must ask for overwrite confirmation")
	}
	if !v.ModalOpen() {
		t.Fatal("confirm dialog must register as a modal")
	}
	// n declines; the live conversation survives.
	v, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.resumeConfirm {
		t.Fatal("esc should dismiss the confirm")
	}
	if v.notice != "resume cancelled" {
		t.Errorf("notice = %q", v.notice)
	}
	if len(v.turns) != 1 || v.turns[0].msg.Content != "live turn" {
		t.Fatalf("decline must keep the live conversation: %+v", v.turns)
	}
	_ = cmd

	// y proceeds with the import.
	typeText(t, &v, "/resume")
	v, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	runCmd(t, &v, cmd)
	v, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.resumeConfirm {
		t.Fatal("expected the overwrite confirm again")
	}
	v, cmd = v.Update(tea.KeyPressMsg{Text: "y"})
	runCmd(t, &v, cmd)
	if len(v.turns) != 2 || v.turns[0].msg.Content != "explain this repo" {
		t.Errorf("confirm did not import the transcript: %+v", v.turns)
	}
}

func TestAgentViewResumeNoSessionsAndOff(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)

	// Recording off (SELFTUI_NO_SESSION=1 leaves the dir empty).
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/resume")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.resumeOpen {
		t.Fatal("picker must not open when recording is off")
	}
	if v.notice != "session recording is off — nothing to resume" {
		t.Errorf("notice = %q", v.notice)
	}

	// Empty dir: the picker opens, the listing empties it with one notice.
	v = testAgent(t, client)
	v = v.WithSessionDir(t.TempDir(), "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/resume")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.resumeOpen {
		t.Fatalf("picker did not open: %q", v.notice)
	}
	runCmd(t, &v, cmd)
	if v.resumeOpen {
		t.Error("picker should close on an empty listing")
	}
	if v.notice != "no saved sessions" {
		t.Errorf("notice = %q", v.notice)
	}
}

func TestAgentViewResumePendingBlocksSend(t *testing.T) {
	// Blocker regression: picking a transcript arms an async load; a
	// message sent before the load lands used to be wiped by
	// applySessionLoaded replacing the conversation. The load window
	// refuses sends (draft preserved) and unblocks once the import lands.
	client, sent, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	path := seedSavedTranscript(t, dir)

	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// Pick the transcript but do not run the load command yet.
	typeText(t, &v, "/resume")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	runCmd(t, &v, cmd)
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // pick row 0
	if !v.resumePending {
		t.Fatal("pick with an empty conversation must arm the load window")
	}

	// A send while the load is in flight is refused, not queued.
	typeText(t, &v, "new conversation")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(v.turns) != 0 {
		t.Fatalf("send during a pending load must not append: %+v", v.turns)
	}
	if len(*sent) != 0 {
		t.Fatalf("send during a pending load must not stream (sent=%d)", len(*sent))
	}
	if v.input.Value() != "new conversation" {
		t.Errorf("draft must survive the refused send: %q", v.input.Value())
	}
	if !strings.Contains(v.notice, "resume in progress") {
		t.Errorf("notice = %q, want the pending-load notice", v.notice)
	}

	// The load lands: import proceeds, sends unblock, and the next send
	// carries the imported history.
	runCmd(t, &v, func() tea.Msg {
		turns, err := session.Load(path)
		return agentEventMsg{msg: sessionLoadedMsg{path: path, turns: turns, err: err}}
	})
	if v.resumePending {
		t.Fatal("landing the load must clear the pending window")
	}
	if len(v.turns) != 2 {
		t.Fatalf("import landed %d turns, want 2", len(v.turns))
	}
	// The preserved draft is still there; sending it now goes through.
	if v.input.Value() != "new conversation" {
		t.Fatalf("draft must survive the load landing: %q", v.input.Value())
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	if len(*sent) != 1 || len((*sent)[0]) != 3 || (*sent)[0][2].Content != "new conversation" {
		t.Errorf("send after landing did not carry the import:\nsent=%+v", *sent)
	}
}

func TestAgentViewResumeOverwritePendingBlocksSend(t *testing.T) {
	// The confirm-dialog path arms the same window: y → import must also
	// refuse sends until the load lands.
	client, sent, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	path := seedSavedTranscript(t, dir)

	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.turns = append(v.turns, turn{
		msg:    ollama.ChatMessage{Role: ollama.RoleUser, Content: "live turn"},
		render: "x",
	})

	typeText(t, &v, "/resume")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	runCmd(t, &v, cmd)
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // pick → confirm dialog
	v, _ = v.Update(tea.KeyPressMsg{Text: "y"})          // arm the import
	if !v.resumePending {
		t.Fatal("confirming the overwrite must arm the load window")
	}
	typeText(t, &v, "raced")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(*sent) != 0 || len(v.turns) != 1 || v.turns[0].msg.Content != "live turn" {
		t.Errorf("send during a confirmed import must be refused: sent=%d turns=%+v", len(*sent), v.turns)
	}
	runCmd(t, &v, func() tea.Msg {
		turns, err := session.Load(path)
		return agentEventMsg{msg: sessionLoadedMsg{path: path, turns: turns, err: err}}
	})
	if len(v.turns) != 2 || v.turns[0].msg.Content != "explain this repo" {
		t.Fatalf("confirmed import must land: %+v", v.turns)
	}
}

func TestAgentViewResumeImportRecomputesBudget(t *testing.T) {
	// Blocker regression: the truncation flag must recompute at import
	// (mirroring a normal commit), so an over-budget transcript shows the
	// truncation marker immediately instead of only after the next send.
	client, _, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	path := seedSavedTranscript(t, dir)

	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.numCtx = 8 // tiny budget: limit = 6 tokens; the seeded history alone (~10) overflows it

	v = v.applySessionLoaded(sessionLoadedMsg{path: path, turns: func() []session.Turn {
		turns, err := session.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return turns
	}()})
	if !v.truncated {
		t.Errorf("over-budget import must flag truncation (truncated=%v, tokens=%d, limit=%d)",
			v.truncated, v.ctxTokens(), v.ctxLimit())
	}
}

func TestAgentViewResumeImportSanitizesMetadata(t *testing.T) {
	// Imported model/meta strings and the notice's file name are file-
	// derived display text: control bytes must not survive into state or
	// the notice (same boundary as streamed tokens).
	client, _, _ := fakeOllamaUI(t)
	v := testAgent(t, client)
	turns := []session.Turn{
		{Role: "user"},
		{Role: "assistant", Model: "q\x1b]52;c;x\x07wen", Meta: "0.4s\x1b[2J· stop"},
	}
	hostile := "/tmp/selftui/chat-\x1b]52;c;p\x07evil.md"
	v = v.applySessionLoaded(sessionLoadedMsg{path: hostile, turns: turns})
	if strings.ContainsAny(v.turns[1].model, "\x1b\x07") {
		t.Errorf("imported model kept control bytes: %q", v.turns[1].model)
	}
	if strings.ContainsAny(v.turns[1].meta, "\x1b\x07") {
		t.Errorf("imported meta kept control bytes: %q", v.turns[1].meta)
	}
	if strings.ContainsAny(v.notice, "\x1b\x07") {
		t.Errorf("resume notice kept control bytes: %q", v.notice)
	}
	if len(v.turns) != 2 || v.turns[1].msg.Content != "" {
		t.Errorf("turns = %+v", v.turns)
	}
}

func TestAgentViewResumeErrorsSurfaceOnce(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)

	// Listing failure: a file path is not a directory.
	dir := t.TempDir()
	blocked := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := testAgent(t, client)
	v = v.WithSessionDir(blocked, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/resume")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.resumeOpen {
		t.Fatal("picker should open before the listing fails")
	}
	runCmd(t, &v, cmd)
	if v.resumeOpen {
		t.Error("picker should close on a listing failure")
	}
	if !strings.Contains(v.notice, "resume: ") {
		t.Errorf("notice = %q", v.notice)
	}

	// Load failure (file deleted between listing and picking): one notice,
	// conversation intact.
	v = testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.turns = append(v.turns, turn{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "live turn"}})
	v = v.applySessionLoaded(sessionLoadedMsg{path: "/gone/chat-x.md", err: os.ErrNotExist})
	if !strings.Contains(v.notice, "resume failed") {
		t.Errorf("notice = %q", v.notice)
	}
	if len(v.turns) != 1 || v.turns[0].msg.Content != "live turn" {
		t.Errorf("failed import must keep the live conversation: %+v", v.turns)
	}
}

func TestResumeSlashCommandListed(t *testing.T) {
	cmds := slashCommandList()
	if len(cmds) != 7 {
		t.Fatalf("%d slash commands, want 7 (menu cap tracks the set)", len(cmds))
	}
	found := false
	for _, c := range cmds {
		if c.name == "resume" {
			found = strings.Contains(c.desc, "resume")
		}
	}
	if !found {
		t.Error("no /resume command in the list")
	}
	// "/r" filters to resume + refresh; "/res" narrows to resume only.
	v := testAgent(t, nil)
	typeText(t, &v, "/res")
	if matches := v.slashMatches(); len(matches) != 1 || matches[0].name != "resume" {
		t.Errorf("/res matches = %+v", matches)
	}
}
