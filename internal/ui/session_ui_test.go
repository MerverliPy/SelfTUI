package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestAgentViewPersistsChatSession drives one turn with a session dir
// configured and asserts the per-process transcript file records both turns
// in readable markdown.
func TestAgentViewPersistsChatSession(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "http://fake-host:11434")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("session dir entries = %v (%v), want exactly one transcript", entries, err)
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"# SelfTUI chat session",
		"# host: http://fake-host:11434",
		"## user (qwen3:8b)",
		"hello",
		"## assistant (qwen3:8b)",
		"func main()",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("transcript missing %q:\n%s", want, text)
		}
	}
	// The in-memory transcript is untouched by persistence.
	if len(v.history) != 2 {
		t.Errorf("history = %d, want 2", len(v.history))
	}
}

func TestAgentViewExportCommandShowsPath(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)

	typeText(t, &v, "/export")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(v.notice, "transcript: ") || !strings.Contains(v.notice, filepath.Base(dir)) {
		t.Errorf("notice = %q, want the transcript path", v.notice)
	}
	if _, err := os.Stat(strings.TrimPrefix(v.notice, "transcript: ")); err != nil {
		t.Errorf("notice path is not a real file: %v", err)
	}
	// The export reports the transcript file; it must never claim the
	// conversation itself can be resumed from it (chat stays in-memory).
	if strings.Contains(strings.ToLower(v.notice), "resume") {
		t.Errorf("notice = %q, must not claim the conversation can be resumed", v.notice)
	}
}

func TestAgentViewExportWithoutRecording(t *testing.T) {
	// No session dir configured: /export explains recording is off.
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/export")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(v.notice, "recording is off") {
		t.Errorf("notice = %q, want the off-state hint", v.notice)
	}

	// Dir configured but no messages yet.
	v2 := testAgent(t, nil)
	v2 = v2.WithSessionDir(t.TempDir(), "")
	v2, _ = v2.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v2, "/export")
	v2, _ = v2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(v2.notice, "nothing recorded yet") {
		t.Errorf("notice = %q, want the empty hint", v2.notice)
	}
}

func TestAgentViewWithoutSessionDirWritesNothing(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	v := testAgent(t, client) // default: no dir
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	if v.session != nil {
		t.Fatal("no session should be opened without a dir")
	}
	if v.sessionErr || v.notice != "" {
		t.Errorf("unexpected session state: err=%v notice=%q", v.sessionErr, v.notice)
	}
}
