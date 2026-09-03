package ui

// Command palette (M7-A): ctrl+p from any tab, live-filtered, modeled on the
// opencode.ai TUI reference. Actions run synchronously against the App value
// in Update (value semantics), so no async plumbing is needed; the modal's
// own keys are handled by paletteKey and rendering by renderPaletteOverlay.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// paletteItem is one command in the palette.
type paletteItem struct {
	id   string
	name string
	desc string
}

// paletteItemList is the static command set. Names double as filter keywords.
func paletteItemList() []paletteItem {
	return []paletteItem{
		{"tab-models", "Models", "go to the Models tab"},
		{"tab-agent", "Agent", "go to the Agent tab"},
		{"tab-settings", "Settings", "go to the Settings tab"},
		{"model", "Change model", "open the Agent model picker"},
		{"clear", "Clear conversation", "wipe the Agent transcript (asks first)"},
		{"theme", "Toggle theme", "switch dark ↔ light for this session"},
		{"refresh", "Refresh models", "reload the model list"},
		{"help", "Command list", "open the slash-command reference"},
	}
}

// paletteMatches filters the command set by the live filter (substring over
// name + description, case-insensitive).
func paletteMatches(filter string) []paletteItem {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return paletteItemList()
	}
	var out []paletteItem
	for _, it := range paletteItemList() {
		hay := strings.ToLower(it.name + " " + it.desc + " " + it.id)
		if strings.Contains(hay, f) {
			out = append(out, it)
		}
	}
	return out
}

// paletteKey handles keys while the palette is open. It mirrors the Agent
// picker: j/k + arrows move, any printable rune extends the filter, backspace
// edits it, enter runs the highlighted command, esc closes.
func (a App) paletteKey(k tea.Key) (App, tea.Cmd) {
	items := paletteMatches(a.palFilter)
	switch {
	case k.Code == tea.KeyEsc:
		a.paletteOpen = false
	case k.Code == tea.KeyEnter:
		if len(items) > 0 && a.palIdx >= 0 && a.palIdx < len(items) {
			a.paletteOpen = false
			return a.runPaletteItem(items[a.palIdx])
		}
		a.paletteOpen = false
	case k.Text == "j" || k.Code == tea.KeyDown:
		if a.palIdx < len(items)-1 {
			a.palIdx++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if a.palIdx > 0 {
			a.palIdx--
		}
	case k.Code == tea.KeyBackspace:
		if a.palFilter != "" {
			a.palFilter = trimLastRune(a.palFilter)
			a.palIdx = 0
		}
	default:
		if t := k.Text; t != "" {
			a.palFilter += t
			a.palIdx = 0
		}
	}
	return a, nil
}

// runPaletteItem executes a palette command synchronously and returns any
// follow-up command (model reloads). Actions that act on the Agent tab switch
// there first so their result is visible; /help-style overlays open on it.
func (a *App) runPaletteItem(it paletteItem) (App, tea.Cmd) {
	switch it.id {
	case "tab-models":
		a.switchTab(0)
	case "tab-agent":
		a.switchTab(1)
	case "tab-settings":
		a.switchTab(2)
	case "model":
		a.switchTab(1)
		a.agent, _ = a.agent.openSelector()
	case "clear":
		a.switchTab(1)
		if len(a.agent.history) == 0 && a.agent.streamText == "" {
			a.agent.notice = "nothing to clear"
		} else {
			a.agent.clearConfirm = true
		}
	case "theme":
		next := "light"
		if a.curTheme == "light" {
			next = "dark"
		}
		a.applyTheme(next)
		a.note = "theme " + next + " · save in Settings to keep it"
	case "refresh":
		a.models.loading = true
		a.models.listErr = ""
		a.agent.loading = true
		a.agent.modelsErr = ""
		return *a, tea.Batch(a.models.loadCmd(), a.agent.loadModelsCmd())
	case "help":
		a.switchTab(1)
		a.agent.helpOpen = true
	}
	return *a, nil
}

// renderPaletteOverlay draws the centered command list over the body band.
// Rows are filtered live, windowed around the highlight, and height-capped by
// the shared overlay machinery (fitContent) so the palette can never overflow
// a 30-row phone.
func (a App) renderPaletteOverlay() string {
	items := paletteMatches(a.palFilter)
	bodyH := maxInt(a.h-2, 1)
	maxRows := maxInt(bodyH-12, 3)

	lines := []string{"filter: " + a.palFilter}
	if len(items) == 0 {
		lines = append(lines, a.styles.Placeholder.Render("no command matches “"+a.palFilter+"”"))
	} else {
		start := clampInt(a.palIdx-maxRows/2, 0, maxInt(0, len(items)-maxRows))
		end := start + maxRows
		if end > len(items) {
			end = len(items)
		}
		if start > 0 {
			lines = append(lines, "… more above")
		}
		for i := start; i < end; i++ {
			it := items[i]
			marker := "  "
			name := it.name
			if i == a.palIdx {
				marker = "❯ "
				name = lipgloss.NewStyle().Bold(true).Foreground(a.styles.accent).Render(it.name)
			}
			row := marker + name
			base := marker + it.name
			if lipgloss.Width(base)+2+lipgloss.Width(it.desc) <= maxInt(a.w-8, 20) {
				row += "  " + a.styles.Placeholder.Render(it.desc)
			}
			lines = append(lines, row)
		}
		if end < len(items) {
			lines = append(lines, "… more below")
		}
	}
	lines = append(lines, "type to filter · ↑/↓ or j/k move · enter run · esc close")
	return renderCenteredOverlay(a.w, bodyH, a.styles, "Command palette", lines)
}
