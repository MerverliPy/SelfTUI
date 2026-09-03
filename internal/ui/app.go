package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/config"
)

// App is the root Bubble Tea model: owns tab state, geometry, and the
// responsive shell. Per-tab views land with their milestones (M1, M2, M4);
// M0 renders placeholders that still exercise the breakpoint system.
type App struct {
	cfg    *config.Config
	styles Styles
	tab    int
	w, h   int
}

// New builds the root model.
func New(cfg *config.Config, styles Styles) App {
	return App{cfg: cfg, styles: styles}
}

// Init satisfies tea.Model. No startup command in M0.
func (a App) Init() tea.Cmd { return nil }

// Update handles window geometry, navigation keys, and quit.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height

	case tea.KeyMsg:
		switch k := msg.Key(); {
		case k.Code == tea.KeyTab:
			a.tab = (a.tab + 1) % numTabs
		case k.Mod.Contains(tea.ModShift) && k.Code == tea.KeyTab:
			a.tab = (a.tab + numTabs - 1) % numTabs
		case k.Text == "1":
			a.tab = 0
		case k.Text == "2":
			a.tab = 1
		case k.Text == "3":
			a.tab = 2
		case k.Code == 'c' && k.Mod.Contains(tea.ModCtrl):
			return a, func() tea.Msg { return tea.Quit() }
		}
	}
	return a, nil
}

// View renders header + active body + status bar.
func (a App) View() tea.View {
	header := TabBar{Active: a.tab, Styles: a.styles, Width: a.w}.Render()

	var body string
	switch a.tab {
	case 0:
		body = a.modelsView()
	case 1:
		body = a.agentView()
	default:
		body = a.settingsView()
	}

	status := StatusBar{
		Left:   "⏻ " + a.cfg.Host,
		Right:  fmt.Sprintf("%s · %dx%d · %s", tabLabels[a.tab], a.w, a.h, BreakpointFor(a.w)),
		Styles: a.styles,
		Width:  a.w,
	}.Render()

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body, status))
}

// modelsView is the M0 placeholder that proves the breakpoint system: list +
// detail side-by-side on wide/medium screens, stacked on compact ones.
func (a App) modelsView() string {
	layout := ForModels(a.w)
	detail := a.styles.Body.Render("detail pane — model info lands in M1a")
	if layout.SideBySide {
		list := lipgloss.NewStyle().
			Width(layout.ListWidth).
			Border(lipgloss.RoundedBorder()).
			Render("Models list (M1a)")
		return lipgloss.JoinHorizontal(lipgloss.Top, list, detail)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		a.styles.Body.Render("Models list (M1a)"),
		detail,
	)
}

func (a App) agentView() string {
	return a.styles.Body.Render("Agent chat + tools (M2/M3)")
}

func (a App) settingsView() string {
	return a.styles.Body.Render("Settings forms (M4)")
}
