package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// fitContent caps a wrapped dialog body to budget rows while keeping its
// head and its tail (the action/decision legend is always the last line, so
// approve/cancel keys stay on screen). Dropped middle lines become one
// marker row. Used by every overlay so a long payload can never push a
// dialog past the terminal height — on the measured iPhone geometry
// (72x30) that would hide the decision row and the status bar.
func fitContent(lines []string, budget int) []string {
	if len(lines) <= budget {
		return lines
	}
	tailKeep := 2
	if budget < 4 {
		tailKeep = 1
	}
	headKeep := budget - 1 - tailKeep // marker row + tail
	if headKeep < 1 {
		headKeep = 1
	}
	out := make([]string, 0, budget)
	out = append(out, lines[:headKeep]...)
	out = append(out, fmt.Sprintf("… (%d more lines)", len(lines)-headKeep-tailKeep))
	out = append(out, lines[len(lines)-tailKeep:]...)
	return out
}

// renderCenteredOverlay centers a bordered dialog over a body band. Shared by
// every full-screen overlay (Agent modals, the App command palette) so they
// render identically: the body is capped at bodyH-4 rows, then padded and
// centered — a long payload (a multi-KB tool input, a long list) can never
// push the box past the terminal height. On a 30-row phone the decision
// legend and status bar must stay on screen (see fitContent).
func renderCenteredOverlay(w, bodyH int, styles Styles, title string, lines []string) string {
	innerW := maxInt(w-6, 16)
	wrapped := wrapLines(lines, innerW)
	wrapped = fitContent(wrapped, maxInt(bodyH-4, 4))
	padded := make([]string, len(wrapped))
	for i, l := range wrapped {
		padded[i] = l + strings.Repeat(" ", maxInt(0, innerW-lipgloss.Width(l)))
	}
	content := strings.Join(padded, "\n")
	box := styles.Pane.Render(
		lipgloss.NewStyle().Bold(true).Foreground(styles.accent).Render(title) + "\n\n" + content,
	)
	return lipgloss.Place(w, maxInt(1, bodyH), lipgloss.Center, lipgloss.Center, box)
}

// Tab labels in navigation order. <1>/<2>/<3> and tab/shift-tab cycle here.
var tabLabels = []string{"Models", "Agent", "Settings"}

const (
	numTabs = 3
	// agentTab is the Agent (chat) tab index. The shell's digit-key tab
	// jumps are disabled while it is active: its input is a text field and
	// bare digits are legitimate prompt characters (M6 fix, found by the
	// reconnect smoke typing "1 to 300").
	agentTab = 1
)

// TabBar renders the navigation tab row. Highlighting is purely presentational:
// the root model owns the active index (single source of truth).
type TabBar struct {
	Active int
	Styles Styles
	Width  int
}

// Render draws the tab row, wrapping the last tab to fit Width when needed.
func (t TabBar) Render() string {
	var cells []string
	for i, label := range tabLabels {
		style := t.Styles.Tab
		if i == t.Active {
			style = t.Styles.TabActive
		}
		cells = append(cells, style.Render(" "+label+" "))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

// Label returns the active tab label.
func (t TabBar) Label() string { return tabLabels[t.Active] }

// StatusBar is the bottom info strip (left context, right geometry/state).
type StatusBar struct {
	Left, Right string
	Styles      Styles
	Width       int
}

// Render lays out Left … Right with a gap, truncating Right to fit Width.
func (s StatusBar) Render() string {
	left := s.Styles.Status.Render(s.Left)
	right := s.Styles.Status.Render(s.Right)
	gap := s.Width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Not enough room: drop Right.
		return lipgloss.NewStyle().MaxWidth(s.Width).Render(left)
	}
	return left + strings.Repeat(" ", gap) + right
}
