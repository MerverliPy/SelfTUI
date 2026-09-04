package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

func newTestApp(t *testing.T) App {
	t.Helper()
	cfg := config.Default()
	return New(&cfg, NewStyles(cfg.Theme), ollama.New(cfg.Host, cfg.AuthToken))
}

func updateTab(t *testing.T, m tea.Model, msg tea.Msg) App {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(App)
}

// view extracts the rendered text without ANSI noise.
func view(t *testing.T, m tea.Model) string {
	t.Helper()
	v := m.View()
	return stripANSI(v.Content)
}

func TestBootsAndRendersTabs(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 44})
	v := view(t, m)
	for _, label := range tabLabels {
		if !strings.Contains(v, label) {
			t.Errorf("rendered view missing tab %q:\n%s", label, v)
		}
	}
	if !strings.Contains(v, "no models installed") {
		t.Errorf("expected Models empty-state hint, got:\n%s", v)
	}
}

func TestTabCycling(t *testing.T) {
	// The Settings form is a modal while editing (see the ownership test), so
	// shell wrap is exercised between Models and Agent here: tab forward once,
	// shift+tab back.
	m := newTestApp(t)
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.tab; got != 1 {
		t.Fatalf("tab after forward = %d, want 1", got)
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if got := m.tab; got != 0 {
		t.Errorf("tab after shift+tab = %d, want 0 (wrap)", got)
	}
}

// TestSettingsFormOwnsTabKeysWhileEditing: an open settings form is a modal —
// tab advances the form's fields, so the shell tab bar and 1/2/3 jumps are
// inert until the form is saved or discarded (esc).
func TestSettingsFormOwnsTabKeysWhileEditing(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "3"})
	if !m.settings.Editing() {
		t.Fatal("entering Settings should open the editing form")
	}

	// Tab and digit jumps must not steal keys from the form.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "1"})
	if m.tab != 2 || !m.settings.Editing() {
		t.Fatalf("form lost focus while editing: tab=%d editing=%v", m.tab, m.settings.Editing())
	}

	// esc discards; only then does the shell regain the keys.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.settings.Editing() {
		t.Fatal("esc should discard the editing form")
	}
	if got := view(t, m); !strings.Contains(got, "Changes discarded") {
		t.Errorf("expected discarded panel, got:\n%s", got)
	}
	m = updateTab(t, m, tea.KeyPressMsg{Text: "1"})
	if m.tab != 0 {
		t.Errorf("tab after leaving settings = %d, want 0", m.tab)
	}
}

func TestNumberKeysJump(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	if m.tab != 1 {
		t.Errorf("tab after '2' = %d, want 1 (Agent)", m.tab)
	}
}

// TestAgentTabDigitsTypeNotJump: the Agent chat input is a text field, so
// bare digits are prompt characters — typing "1 to 300" must never switch
// tabs mid-prompt (M6 bug found by the reconnect smoke). Digit jumps only
// act from the Models tab (and the Settings result panel).
func TestAgentTabDigitsTypeNotJump(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultModel = "qwen3:8b"
	m := New(&cfg, NewStyles(cfg.Theme), ollama.New(cfg.Host, cfg.AuthToken))
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"}) // to Agent
	if m.tab != 1 {
		t.Fatalf("start tab = %d, want 1", m.tab)
	}
	// Resolve the model fetch deterministically (no live host in tests), then
	// start composing so the digits below are prompt characters, not jumps.
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "Count "})

	for _, d := range []string{"1", "2", "3", "9", "0"} {
		m = updateTab(t, m, tea.KeyPressMsg{Text: d})
		if m.tab != 1 {
			t.Fatalf("digit %q switched tab to %d while composing in Agent", d, m.tab)
		}
		if m.settings.Editing() {
			t.Fatalf("digit %q opened the settings form while composing in Agent", d)
		}
	}

	// The composing contract keeps digit jumps on an EMPTY Agent input (a
	// fresh tab with no text still jumps, per the README's 1/2/3 nav).
	m2 := newTestApp(t)
	m2 = updateTab(t, m2, tea.WindowSizeMsg{Width: 100, Height: 40})
	m2 = updateTab(t, m2, tea.KeyPressMsg{Text: "2"})
	m2 = updateTab(t, m2, agentModelsLoadedMsg{models: sampleModels()})
	m2 = updateTab(t, m2, tea.KeyPressMsg{Text: "3"})
	if m2.tab != 2 || !m2.settings.Editing() {
		t.Errorf("empty Agent input: '3' should jump to Settings, tab=%d editing=%v",
			m2.tab, m2.settings.Editing())
	}

	// Digits must actually land in the conversation input: while composing,
	// the hint legend hides the letter commands ("… · m model · …") and shows
	// only "enter send · shift+enter newline" — visible proof the prompt text
	// is buffered.
	prompt := "from 1 to 300, then stop."
	for _, ch := range prompt {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(ch)})
	}
	if m.tab != 1 {
		t.Fatalf("typing the prompt switched tab to %d", m.tab)
	}
	got := view(t, m)
	if !strings.Contains(got, "enter send · shift+enter newline") ||
		strings.Contains(got, "m model · r refresh") {
		t.Errorf("input did not buffer the prompt (hint should be the composing "+
			"legend without letter commands):\n%s", got)
	}

	// Enter sends (or is consumed by) the Agent view — never a tab jump.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.tab != 1 || m.settings.Editing() {
		t.Errorf("enter while typing in Agent switched away: tab=%d editing=%v",
			m.tab, m.settings.Editing())
	}
}

func TestCtrlCQuits(t *testing.T) {
	m := newTestApp(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected quit command, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("cmd() = %T, want tea.QuitMsg", cmd())
	}
}

func TestRendersAtBothTargetWidths(t *testing.T) {
	// PC-ish wide and iPhone-portrait-ish narrow: no panics, tabs always on screen.
	for _, w := range []int{120, 88} {
		m := newTestApp(t)
		m = updateTab(t, m, tea.WindowSizeMsg{Width: w, Height: 40})
		v := view(t, m)
		if !strings.Contains(v, "Models") || !strings.Contains(v, "Settings") {
			t.Errorf("width %d: tabs missing:\n%s", w, v)
		}
		if strings.Contains(v, "\n\n\n\n\n") {
			t.Errorf("width %d: excessive blank lines in view", w)
		}
	}
}

func TestModelsGeometryAdapts(t *testing.T) {
	cases := []struct {
		width int
		side  bool
	}{
		{60, false}, // compact: stacked
		{88, false}, // medium-narrow: still stacked (text readability)
		{100, true}, // medium split
		{140, true}, // wide split
	}
	for _, c := range cases {
		if got := ForModels(c.width).SideBySide; got != c.side {
			t.Errorf("ForModels(%d).SideBySide = %v, want %v", c.width, got, c.side)
		}
	}
}

func TestBreakpointBoundaries(t *testing.T) {
	cases := []struct {
		width int
		want  Breakpoint
	}{
		{1, Compact}, {79, Compact}, {80, Medium}, {119, Medium}, {120, Wide}, {200, Wide},
	}
	for _, c := range cases {
		if got := BreakpointFor(c.width); got != c.want {
			t.Errorf("BreakpointFor(%d) = %v, want %v", c.width, got, c.want)
		}
	}
}

func TestMeasuredDeviceWidthIsCompact(t *testing.T) {
	// M0a evidence: the real Moshi session on an iPhone 16 Pro measured
	// 72x30 (docs/m0a-gate-evidence.md). The measured portrait width must
	// classify compact and stack the Models panes.
	if got := BreakpointFor(devicePortraitCols); got != Compact {
		t.Errorf("BreakpointFor(%d) = %v, want Compact (measured device width)", devicePortraitCols, got)
	}
	if got := ForModels(devicePortraitCols).SideBySide; got {
		t.Error("ForModels(devicePortraitCols).SideBySide = true, want false (stacked on the phone)")
	}
}

// stripANSI removes SGR/CSI sequences so tests assert on text, not styling.
func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '\x1b' {
			in = true
			continue
		}
		if in {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
