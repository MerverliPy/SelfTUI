package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
)

func newTestApp(t *testing.T) App {
	t.Helper()
	cfg := config.Default()
	return New(&cfg, NewStyles(cfg.Theme))
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
	if !strings.Contains(v, "Models list") {
		t.Errorf("expected Models placeholder body, got:\n%s", v)
	}
}

func TestTabCycling(t *testing.T) {
	m := newTestApp(t)
	for i := 0; i < 3; i++ {
		m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	if got := m.tab; got != 0 {
		t.Errorf("tab after 3 tabs = %d, want 0 (wrap)", got)
	}
}

func TestNumberKeysJump(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	if m.tab != 1 {
		t.Errorf("tab after '2' = %d, want 1 (Agent)", m.tab)
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
