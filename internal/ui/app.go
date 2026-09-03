package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

// App is the root Bubble Tea model: owns tab state, geometry, and the
// responsive shell. Each tab's view lives in its own file (models_view.go
// since M1a; agent/settings land with M2/M4).
type App struct {
	cfg    *config.Config
	styles Styles
	tab    int
	w, h   int
	models ModelsView
}

// New builds the root model. client is the Ollama connection used by the
// Models tab (M1a); later tabs share it.
func New(cfg *config.Config, styles Styles, client *ollama.Client) App {
	return App{
		cfg:    cfg,
		styles: styles,
		models: NewModelsView(client, styles, cfg.Theme),
	}
}

// Init starts the Models list fetch.
func (a App) Init() tea.Cmd { return a.models.Init() }

// Update handles window geometry, navigation keys, Models-tab keys and async
// model/detail results.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		a.models, _ = a.models.Update(msg)

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
		default:
			// Keys not claimed by the shell go to the active tab.
			if a.tab == 0 {
				models, cmd := a.models.Update(msg)
				a.models = models
				return a, cmd
			}
		}

	case modelsLoadedMsg, modelsLoadErrMsg, modelsShowMsg, modelsShowErrMsg:
		models, cmd := a.models.Update(msg)
		a.models = models
		return a, cmd
	}
	return a, nil
}

// View renders header + active body + status bar.
func (a App) View() tea.View {
	header := TabBar{Active: a.tab, Styles: a.styles, Width: a.w}.Render()

	var body string
	switch a.tab {
	case 0:
		body = a.models.View()
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

func (a App) agentView() string {
	return a.styles.Body.Render("Agent chat + tools (M2/M3)")
}

func (a App) settingsView() string {
	return a.styles.Body.Render("Settings forms (M4)")
}
