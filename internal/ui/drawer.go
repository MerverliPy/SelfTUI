package ui

// Logs drawer (PLAN.md §12 N5): a k9s-style, keybind-toggled read-only
// overlay over the shell's bottom rows, backed by the shared logsink ring
// buffer. The drawer never mutates anything — it renders the same redacted
// stream the --log-file sink writes — so it is safe to open over any tab,
// any modal, and a live stream.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	// drawerMaxRows is the body band height the drawer may take over. It
	// leaves the tab header and status bar untouched and keeps most of the
	// active view visible on the 30-row phone.
	drawerMaxRows = 10

	// drawerPageSize is how far one pgup/pgdn scrolls inside the ring.
	drawerPageSize = 10

	// drawerScrollMax caps the scroll-back offset so a huge ring cannot make
	// the arithmetic overflow; the render clamps to the real top anyway.
	drawerScrollMax = 1 << 20
)

// toggleDrawer opens or closes the logs drawer (ctrl+o, or the palette's
// Logs command). A shell without a sink (compat constructors, tests) cannot
// open one — the key is inert rather than an empty drawer.
func (a *App) toggleDrawer() {
	if a.drawerOpen {
		a.drawerOpen = false
		a.drawerUp = 0
		return
	}
	if a.logSink == nil {
		return
	}
	a.drawerOpen = true
	a.drawerUp = 0
}

// drawerKey handles keys while the drawer is open: pgup/pgdn and up/down
// scroll (offset 0 = follow the tail), Home/End jump, esc closes, and
// everything else is swallowed — the drawer is a read-only modal.
func (a App) drawerKey(k tea.Key) (App, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		a.drawerOpen = false
		a.drawerUp = 0
	case k.Code == tea.KeyPgUp:
		a.drawerUp = minInt(a.drawerUp+drawerPageSize, drawerScrollMax)
	case k.Code == tea.KeyPgDown:
		a.drawerUp = maxInt(a.drawerUp-drawerPageSize, 0)
	case k.Code == tea.KeyUp || k.Text == "k":
		a.drawerUp = minInt(a.drawerUp+1, drawerScrollMax)
	case k.Code == tea.KeyDown || k.Text == "j":
		a.drawerUp = maxInt(a.drawerUp-1, 0)
	case k.Code == tea.KeyHome:
		a.drawerUp = drawerScrollMax
	case k.Code == tea.KeyEnd:
		a.drawerUp = 0
	}
	return a, nil
}

// overlayDrawer replaces the bottom rows of the full-height body band with
// the logs drawer, k9s-style: the header stays above, the status bar below,
// and the drawer floats over the active view's tail. The drawer takes
// drawerMaxRows rows (never more than the band minus one body row).
func (a App) overlayDrawer(body string) string {
	rows := strings.Split(body, "\n")
	if len(rows) < 2 {
		return body
	}
	h := minInt(drawerMaxRows, len(rows)-1)
	head := rows[:len(rows)-h]
	return strings.Join(append(head, a.renderDrawer(h, a.w)...), "\n")
}

// renderDrawer draws `rows` drawer rows at width w: an accent title bar,
// the ring window (bottom-aligned to the legend so the newest entry sits
// just above it while following), and the key legend. Scrolled state is
// announced in the legend; every line is truncated to the terminal width.
func (a App) renderDrawer(rows, w int) []string {
	out := make([]string, 0, rows)
	title := truncateToWidth(" logs — debug (read-only)", w)
	out = append(out, lipgloss.NewStyle().Bold(true).Foreground(a.styles.accent).Render(title))

	var lines []string
	if a.logSink != nil {
		lines = a.logSink.Lines()
	}
	bodyRows := maxInt(rows-2, 1)

	up := minInt(a.drawerUp, len(lines)) // cannot scroll past the top
	end := len(lines) - up
	start := maxInt(0, end-bodyRows)
	window := lines[start:end]

	if len(window) == 0 && up == 0 && len(lines) == 0 {
		window = append(window, a.styles.Placeholder.Render(
			"no log entries — ollama requests and agent decisions appear here"))
	}
	// Bottom-align: pad above so the newest entry hugs the legend.
	for pad := bodyRows - len(window); pad > 0; pad-- {
		window = append([]string{""}, window...)
	}
	for _, l := range window {
		out = append(out, truncateToWidth(l, w))
	}

	legend := "pgup/pgdn scroll · esc close · ctrl+o toggle"
	if up > 0 {
		legend = fmt.Sprintf("↑ %d back · %s (end to follow tail)", up, legend)
	}
	out = append(out, truncateToWidth(a.styles.Placeholder.Render(legend), w))
	if len(out) > rows {
		out = out[:rows]
	}
	return out
}
