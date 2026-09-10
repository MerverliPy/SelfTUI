package ui

import (
	"fmt"
	"testing"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

func TestLastAssistantMarkdown(t *testing.T) {
	turns := []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "question 1"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: "first *reply*"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "question 2"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: "second **reply**"}},
	}
	got, ok := lastAssistantMarkdown(turns)
	if !ok || got != "second **reply**" {
		t.Errorf("lastAssistantMarkdown = %q, %v; want %q, true", got, ok, "second **reply**")
	}

	if _, ok := lastAssistantMarkdown([]turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "only a question"}},
	}); ok {
		t.Error("lastAssistantMarkdown reported a reply with no assistant turn")
	}
	if _, ok := lastAssistantMarkdown(nil); ok {
		t.Error("lastAssistantMarkdown reported a reply on an empty transcript")
	}
}

// TestCopyLastDisabledRefuses pins the spike-gated opt-in: with the config
// flag off (the default), the palette action emits nothing — only a pointer
// to the Settings toggle.
func TestCopyLastDisabledRefuses(t *testing.T) {
	app := newTestApp(t) // Default() has OSC52Copy = false
	app.agent.turns = []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: "a reply"}},
	}
	next, cmd := app.runPaletteItem(paletteItem{id: "copy-last"})
	if cmd != nil {
		t.Error("copy-last with the flag off produced a command, want none")
	}
	if next.agent.notice == "" {
		t.Error("copy-last with the flag off set no notice")
	}
}

func TestCopyLastWithoutReplies(t *testing.T) {
	app := newTestApp(t)
	app.cfg.OSC52Copy = true
	next, cmd := app.runPaletteItem(paletteItem{id: "copy-last"})
	if cmd != nil {
		t.Error("copy-last with an empty transcript produced a command, want none")
	}
	if next.agent.notice != "no reply to copy yet" {
		t.Errorf("notice = %q, want the no-reply hint", next.agent.notice)
	}
}

// TestCopyLastEmitsOSC52 proves the ON path: the most recent assistant turn's
// full markdown goes out as a bubbletea setClipboardMsg (OSC 52) — grill
// decisions #3/#4.
func TestCopyLastEmitsOSC52(t *testing.T) {
	app := newTestApp(t)
	app.cfg.OSC52Copy = true
	app.agent.turns = []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "q"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: "old"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "q2"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: "# latest\nfull *markdown*"}},
	}
	next, cmd := app.runPaletteItem(paletteItem{id: "copy-last"})
	if cmd == nil {
		t.Fatal("copy-last with the flag on produced no command")
	}
	want := "copied 24 chars via OSC 52"
	if next.agent.notice != want {
		t.Errorf("notice = %q, want %q", next.agent.notice, want)
	}
	msg := cmd()
	if got := fmt.Sprintf("%T", msg); got != "tea.setClipboardMsg" {
		t.Errorf("cmd() = %T, want tea.setClipboardMsg (OSC 52 emission)", msg)
	}
}
