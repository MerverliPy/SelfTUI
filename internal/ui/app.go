package ui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/agent"
	"selftui/internal/config"
	"selftui/internal/ollama"
)

// App is the root Bubble Tea model: owns tab state, geometry, and the
// responsive shell. Each tab's view lives in its own file (models_view.go
// since M1a, agent_view.go since M2; settings lands with M4).
type App struct {
	cfg    *config.Config
	styles Styles
	tab    int
	w, h   int
	models ModelsView
	agent  AgentView
}

// New builds the root model. client is the Ollama connection shared by the
// Models and Agent tabs.
func New(cfg *config.Config, styles Styles, client *ollama.Client) App {
	return App{
		cfg:    cfg,
		styles: styles,
		models: NewModelsView(client, styles, cfg.Theme),
		agent:  NewAgentViewWithWorkspace(client, styles, cfg.Theme, cfg.DefaultModel, cfg.WorkspaceRoot, cfg.Agent),
	}
}

// Init starts the Models list and Agent model-list fetches.
func (a App) Init() tea.Cmd { return tea.Batch(a.models.Init(), a.agent.Init()) }

// Update handles window geometry, navigation keys, tab keys, and async
// model/chat results.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		a.models, _ = a.models.Update(msg)
		a.agent, _ = a.agent.Update(msg)

	case tea.KeyMsg:
		switch k := msg.Key(); {
		case k.Code == tea.KeyTab:
			a.tab = (a.tab + 1) % numTabs
		case k.Mod.Contains(tea.ModShift) && k.Code == tea.KeyTab:
			a.tab = (a.tab + numTabs - 1) % numTabs
		case k.Text == "1" && !a.models.ModalOpen() && !a.agent.ModalOpen():
			a.tab = 0
		case k.Text == "2" && !a.models.ModalOpen() && !a.agent.ModalOpen():
			a.tab = 1
		case k.Text == "3" && !a.models.ModalOpen() && !a.agent.ModalOpen():
			a.tab = 2
		case k.Code == 'c' && k.Mod.Contains(tea.ModCtrl):
			return a, func() tea.Msg { return tea.Quit() }
		default:
			// Keys not claimed by the shell go to the active tab.
			switch a.tab {
			case 0:
				models, cmd := a.models.Update(msg)
				a.models = models
				return a, cmd
			case 1:
				agent, cmd := a.agent.Update(msg)
				a.agent = agent
				return a, cmd
			}
		}

	case modelsLoadedMsg, modelsLoadErrMsg, modelsShowMsg, modelsShowErrMsg,
		modelsDeleteDoneMsg, modelsPullMsg, modelsPullDoneMsg, spinner.TickMsg:
		models, cmd := a.models.Update(msg)
		a.models = models
		return a, cmd

	case agentModelsLoadedMsg, agentModelsErrMsg, agentTokenMsg, agentDoneMsg,
		agent.TokenMsg, agent.ToolStartMsg, agent.ToolResultMsg, agent.FallbackMsg, agent.AgentDoneMsg:
		agent, cmd := a.agent.Update(msg)
		a.agent = agent
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

func (a App) agentView() string { return a.agent.View() }

func (a App) settingsView() string {
	return a.styles.Body.Render("Settings forms (M4)")
}
