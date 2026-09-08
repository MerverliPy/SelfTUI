package ui

// N5 debug/logs drawer tests (PLAN.md §12 N5): ctrl+o toggles a read-only
// k9s-style drawer over the shell's bottom rows, backed by the shared
// logsink ring buffer; the palette exposes the same toggle for phone users.
// Drawer-closed frames stay byte-identical (the goldens pin it); drawer-open
// frames are new fixtures (agent-logs-drawer-*).

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/logsink"
)

// newDrawerApp builds an App whose shell carries a sink pre-loaded with the
// given log lines (written through the sink so they arrive redacted, the
// same path live entries take). The logger itself is not needed here — the
// drawer reads the ring, not the logger.
func newDrawerApp(t *testing.T, w, h int, lines ...string) App {
	t.Helper()
	app := bootApp(t, w, h)
	sink := logsink.New(io.Discard, logsink.NewRing(1000), "")
	for _, l := range lines {
		sink.Write([]byte(l + "\n"))
	}
	return app.WithLog(nil, sink)
}

func ctrlO(t *testing.T, m App) App {
	return updateTab(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
}

func TestCtrlOOpensAndClosesDrawer(t *testing.T) {
	m := newDrawerApp(t, 72, 30, "INF one", "INF two")
	m = ctrlO(t, m)
	if !m.drawerOpen {
		t.Fatal("ctrl+o should open the drawer")
	}
	out := view(t, m)
	for _, want := range []string{"logs", "INF two", "pgup/pgdn scroll"} {
		if !strings.Contains(out, want) {
			t.Errorf("drawer view missing %q:\n%s", want, out)
		}
	}
	// Newest entry hugs the legend (tail-follow), the ring cap holds.
	m = ctrlO(t, m)
	if m.drawerOpen {
		t.Fatal("ctrl+o should close the drawer again")
	}
}

func TestDrawerInertWithoutSink(t *testing.T) {
	m := bootApp(t, 72, 30)
	m = ctrlO(t, m)
	if m.drawerOpen {
		t.Fatal("ctrl+o must be inert without a sink (compat constructors/tests)")
	}
}

func TestDrawerClosedViewIsByteIdentical(t *testing.T) {
	plain := bootApp(t, 72, 30)
	wired := newDrawerApp(t, 72, 30, "INF one", "INF two")
	if plain.View().Content != wired.View().Content {
		t.Error("wiring a sink (drawer closed) must not change the rendered frame")
	}
	widePlain := bootApp(t, 120, 40)
	wideWired := newDrawerApp(t, 120, 40, "INF one")
	if widePlain.View().Content != wideWired.View().Content {
		t.Error("wiring a sink (wide, drawer closed) must not change the frame")
	}
}

func TestDrawerEscCloses(t *testing.T) {
	m := newDrawerApp(t, 72, 30, "INF one")
	m = ctrlO(t, m)
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.drawerOpen || m.drawerUp != 0 {
		t.Fatal("esc should close the drawer and reset the scroll")
	}
}

func TestDrawerOwnsKeysWhileOpen(t *testing.T) {
	m := newDrawerApp(t, 72, 30, "INF one")
	m = ctrlO(t, m)
	// Digit jumps must not escape the drawer (modal discipline, M-01).
	m = updateTab(t, m, tea.KeyPressMsg{Text: "1"})
	if m.drawerOpen && m.tab != 0 {
		t.Fatalf("digit leaked through the drawer: tab=%d", m.tab)
	}
	// Tab navigation must not escape either.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.tab != 0 {
		t.Fatalf("tab leaked through the drawer: tab=%d", m.tab)
	}
}

func TestDrawerScrollAndFollow(t *testing.T) {
	lines := make([]string, 0, 25)
	for i := 0; i < 25; i++ {
		lines = append(lines, strings.Repeat("x", 20)+fmtInt(i))
	}
	m := newDrawerApp(t, 72, 30, lines...)
	m = ctrlO(t, m)

	// Following shows the newest entry.
	if got := view(t, m); !strings.Contains(got, fmtInt(24)) {
		t.Errorf("tail-follow should show the newest entry:\n%s", got)
	}
	// pgup scrolls back a page; the legend announces the offset.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.drawerUp != 10 {
		t.Fatalf("pgup offset = %d, want 10", m.drawerUp)
	}
	if got := view(t, m); !strings.Contains(got, "↑ 10 back") || strings.Contains(got, fmtInt(24)) {
		t.Errorf("scrolled view should hide the tail and announce the offset:\n%s", got)
	}
	// End returns to tail-follow; Home goes to the top.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.drawerUp != 0 {
		t.Fatal("end should return to tail-follow")
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	if m.drawerUp == 0 {
		t.Fatal("home should scroll to the top of the ring")
	}
	if got := view(t, m); !strings.Contains(got, fmtInt(0)) {
		t.Errorf("home view should show the oldest entry:\n%s", got)
	}
	// Scrolling past the top clamps: older entries exist, not negative rows.
	for i := 0; i < 5; i++ {
		m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if got := view(t, m); !strings.Contains(got, fmtInt(0)) {
		t.Errorf("clamped top view should still show the oldest entry:\n%s", got)
	}
}

func TestDrawerEmptyRingPlaceholder(t *testing.T) {
	m := newDrawerApp(t, 72, 30)
	m = ctrlO(t, m)
	if got := view(t, m); !strings.Contains(got, "no log entries") {
		t.Errorf("empty ring should show a placeholder:\n%s", got)
	}
}

func TestDrawerStaysInsideFrame(t *testing.T) {
	for _, g := range []struct{ w, h int }{{72, 30}, {120, 40}} {
		m := newDrawerApp(t, g.w, g.h, strings.Repeat("y", 200))
		m = ctrlO(t, m)
		rows := frameRows(m.View().Content)
		if len(rows) > g.h {
			t.Errorf("%dx%d: drawer view is %d rows (terminal %d)", g.w, g.h, len(rows), g.h)
		}
		for i, l := range rows {
			if wd := lipgloss.Width(l); wd > g.w {
				t.Errorf("%dx%d: row %d is %d wide:\n%q", g.w, g.h, i+1, wd, stripANSI(l))
			}
		}
	}
}

func TestPaletteListsAndOpensLogsDrawer(t *testing.T) {
	found := false
	for _, it := range paletteItemList() {
		if it.id == "logs" {
			found = true
			if !strings.Contains(it.desc, "ctrl+o") {
				t.Errorf("logs command desc should teach the keybind: %q", it.desc)
			}
		}
	}
	if !found {
		t.Fatal("palette is missing the logs command")
	}
	m := newDrawerApp(t, 72, 30, "INF one")
	m = updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "logs"})
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.drawerOpen {
		t.Fatal("palette logs command should open the drawer")
	}
	// A sink-less shell's palette entry is inert, not a blank drawer.
	bare := bootApp(t, 72, 30)
	bare = updateTab(t, bare, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	bare = updateTab(t, bare, tea.KeyPressMsg{Text: "logs"})
	bare = updateTab(t, bare, tea.KeyPressMsg{Code: tea.KeyEnter})
	if bare.drawerOpen {
		t.Fatal("logs command must stay inert without a sink")
	}
}

func TestCtrlPBlockedWhileDrawerOpen(t *testing.T) {
	m := newDrawerApp(t, 72, 30, "INF one")
	m = ctrlO(t, m)
	m = updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if m.paletteOpen {
		t.Fatal("the palette must not open over the drawer modal")
	}
}

// fmtInt is a tiny deterministic formatter for scroll test fixtures.
func fmtInt(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for n := i; n > 0; n /= 10 {
		digits = string(rune('0'+n%10)) + digits
	}
	return digits
}
