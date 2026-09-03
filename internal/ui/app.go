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
// since M1a, agent_view.go since M2, settings_view.go since M4).
type App struct {
	cfg    *config.Config
	styles Styles
	tab    int
	w, h   int
	models ModelsView
	agent  AgentView
	// settings is the Settings tab. While its form is being edited it is a
	// modal: the shell's tab keys and 1/2/3 jumps yield to the form so typed
	// characters reach the fields (see switchTab and the KeyMsg handling).
	settings SettingsView
}

// New builds the root model. client is the Ollama connection shared by the
// Models and Agent tabs. cfg is the live session config (settings edits and
// env/flag overrides all resolve onto it).
func New(cfg *config.Config, styles Styles, client *ollama.Client) App {
	return App{
		cfg:      cfg,
		styles:   styles,
		models:   NewModelsView(client, styles, cfg.Theme),
		agent:    NewAgentViewWithWorkspace(client, styles, cfg.Theme, cfg.DefaultModel, cfg.WorkspaceRoot, cfg.Agent),
		settings: NewSettingsView(cfg, styles),
	}
}

// Init starts the Models list and Agent model-list fetches.
func (a App) Init() tea.Cmd { return tea.Batch(a.models.Init(), a.agent.Init()) }

// switchTab moves to tab i. Entering the Settings tab always starts a fresh
// editing form over the current config, so saved/discarded result panels are
// never carried into the next edit and no form survives a tab change.
func (a *App) switchTab(i int) {
	if i < 0 || i >= numTabs || i == a.tab {
		return
	}
	if i == 2 {
		a.settings = a.settings.Begin()
	}
	a.tab = i
}

// Update handles window geometry, navigation keys, tab keys, async results,
// and settings persistence / live-apply messages.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		a.models, _ = a.models.Update(msg)
		a.agent, _ = a.agent.Update(msg)
		a.settings, _ = a.settings.Update(msg)

	case settingsThemeMsg:
		// Live theme preview (or rollback after a discard). The config file
		// is not involved; cfg.Theme only changes when a save commits.
		a.applyTheme(msg.theme)
		return a, nil

	case settingsSaveDoneMsg:
		if msg.err != nil {
			a.settings = a.settings.noteSaveError(msg.err)
			return a, nil
		}
		cmd := a.applySaved(msg.cfg)
		a.settings = a.settings.noteSaved(msg.cfg.ConfigPath())
		return a, cmd

	case tea.KeyMsg:
		k := msg.Key()
		// ctrl+c always quits the app, even while a settings form is open.
		if k.Code == 'c' && k.Mod.Contains(tea.ModCtrl) {
			return a, func() tea.Msg { return tea.Quit() }
		}
		// An open settings form consumes every key (it is its own modal);
		// esc discards, enter/tab advance. Tab-bar and digit keys only act
		// once the form is finished.
		if a.tab == 2 && a.settings.Editing() {
			updated, cmd := a.settings.Update(msg)
			a.settings = updated
			return a, cmd
		}

		switch {
		case k.Mod.Contains(tea.ModShift) && k.Code == tea.KeyTab:
			a.switchTab((a.tab + numTabs - 1) % numTabs)
		case k.Code == tea.KeyTab:
			a.switchTab((a.tab + 1) % numTabs)
		case k.Text == "1" && (a.tab != agentTab || !a.agent.composing()) && !a.models.ModalOpen() && !a.agent.ModalOpen():
			a.switchTab(0)
		case k.Text == "2" && (a.tab != agentTab || !a.agent.composing()) && !a.models.ModalOpen() && !a.agent.ModalOpen():
			a.switchTab(1)
		case k.Text == "3" && (a.tab != agentTab || !a.agent.composing()) && !a.models.ModalOpen() && !a.agent.ModalOpen():
			a.switchTab(2)
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
			case 2:
				// Result panels (saved/discarded/error): enter edits again.
				if k.Code == tea.KeyEnter {
					a.settings = a.settings.Begin()
				}
				return a, nil
			}
		}
		// A shell-level navigation key was handled above; never re-deliver
		// the same keypress to the tab that was just entered.
		return a, nil

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
	default:
		// Leftover messages while a settings form is open belong to the
		// form: huh drives navigation with internal next/prev field/group
		// messages that bubble tea re-feeds from commands, and those must
		// reach the form. Async model/chat results never land here (their
		// cases above); a handled KeyMsg returns before reaching default,
		// so the key that opened the form is never re-typed into it.
		if a.tab == 2 && a.settings.Editing() {
			updated, cmd := a.settings.Update(msg)
			a.settings = updated
			return a, cmd
		}
	}
	return a, nil
}

// applyTheme re-themes the whole shell (styles, tab chrome, and every child
// view). Used for live Theme previews and after a saved theme change.
func (a *App) applyTheme(theme string) {
	dark := theme != "light"
	a.styles = NewStyles(theme)
	a.models = a.models.applyTheme(dark, a.styles)
	a.agent = a.agent.applyTheme(dark, a.styles)
	a.settings = a.settings.applyTheme(dark, a.styles)
}

// applySaved commits a successful settings save in-session: the config value
// replaces the live one, the theme is re-applied when it changed, and a host
// or token change swaps the shared Ollama client and refreshes both model
// lists. Returning cmds (reloads) keeps the UI non-blocking.
func (a *App) applySaved(cfg config.Config) tea.Cmd {
	themeChanged := cfg.Theme != a.cfg.Theme
	clientChanged := cfg.Host != a.cfg.Host || cfg.AuthToken != a.cfg.AuthToken

	*a.cfg = cfg

	if themeChanged {
		a.applyTheme(cfg.Theme)
	} else {
		// Keep the settings chrome in sync with the current styles even when
		// only the theme didn't change (no-op when identical).
		a.settings = a.settings.applyTheme(cfg.Theme != "light", a.styles)
	}

	client := ollama.New(cfg.Host, cfg.AuthToken)

	if clientChanged {
		models, mCmd := a.models.ApplyClient(client)
		agent, aCmd := a.agent.ApplyConfig(cfg, client, true)
		a.models, a.agent = models, agent
		return tea.Batch(mCmd, aCmd)
	}

	agent, aCmd := a.agent.ApplyConfig(cfg, client, false)
	a.agent = agent
	if aCmd != nil {
		return aCmd
	}
	return nil
}

// View renders header + active body + status bar.
func (a App) View() tea.View {
	header := TabBar{Active: a.tab, Styles: a.styles, Width: a.w}.Render()

	var body string
	switch a.tab {
	case 0:
		body = a.models.View()
	case 1:
		body = a.agent.View()
	default:
		body = a.settings.View()
	}

	status := StatusBar{
		Left:   "⏻ " + a.cfg.Host,
		Right:  fmt.Sprintf("%s · %dx%d · %s", tabLabels[a.tab], a.w, a.h, BreakpointFor(a.w)),
		Styles: a.styles,
		Width:  a.w,
	}.Render()

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body, status))
}
