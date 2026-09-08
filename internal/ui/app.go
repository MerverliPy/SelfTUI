package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/log"

	"github.com/MerverliPy/SelfTUI/internal/config"
	"github.com/MerverliPy/SelfTUI/internal/logsink"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
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

	// client is the App-owned Ollama connection shared by the Models and
	// Agent tabs. It is created once at construction and replaced only when a
	// settings save changes host or token; scalar/theme-only saves reuse it,
	// so the two tabs can never drift onto different client instances (P1-6).
	client *ollama.Client

	// settings is the Settings tab. While its form is being edited it is a
	// modal: the shell's tab keys and 1/2/3 jumps yield to the form so typed
	// characters reach the fields (see switchTab and the KeyMsg handling).
	settings SettingsView

	// Command palette (M7-A): ctrl+p from any tab. It owns its keys while
	// open and routes actions back into the views; see palette.go.
	paletteOpen bool
	palFilter   string
	palIdx      int

	// curTheme is the theme actually applied this session (dark/light). It
	// can differ from cfg.Theme between a Settings save and a session-only
	// toggle (/theme, palette), so theme toggles never stick on the old value.
	curTheme string

	// note is a transient app-level toast shown in the status bar's left
	// cell (theme toggles etc.), cleared on the next keypress.
	note string

	// N5 debug drawer: logger + shared sink, wired by main through WithLog
	// (nil in compat constructors/tests → ctrl+o inert). drawerOpen flags
	// the read-only logs overlay; drawerUp is the scroll-back offset from
	// the ring tail (0 follows).
	logger     *log.Logger
	logSink    *logsink.Sink
	drawerOpen bool
	drawerUp   int
}

// New builds the root model with a background parent context. It is the
// compatibility entry point (tests, legacy callers): main wires the real
// signal-derived context through NewWithContext so every Models/Agent
// operation derives from the process root. client is the Ollama connection
// shared by the Models and Agent tabs; cfg is the live session config
// (settings edits and env/flag overrides all resolve onto it).
func New(cfg *config.Config, styles Styles, client *ollama.Client) App {
	return NewWithContext(context.Background(), cfg, styles, client)
}

// NewWithContext builds the root model exactly like New but binds the parent
// context: every operation the Models and Agent tabs start (list/show/delete
// fetches, chat and pull streams) derives from ctx, so canceling the process
// root aborts in-flight background work. A nil ctx is treated as a background
// root (see normalizeCtx).
func NewWithContext(ctx context.Context, cfg *config.Config, styles Styles, client *ollama.Client) App {
	ctx = normalizeCtx(ctx)
	a := App{
		cfg:      cfg,
		styles:   styles,
		curTheme: cfg.Theme,
		client:   client,
		models:   newModelsView(ctx, client, styles, cfg.Theme),
		agent:    newAgentView(ctx, client, styles, cfg.Theme, cfg.DefaultModel, cfg.WorkspaceRoot, cfg.Agent.SystemPrompt, cfg.Agent, cfg.ToolsEnabled, cfg.Host),
		settings: NewSettingsView(cfg, styles),
	}
	return a
}

// normalizeCtx returns ctx, or a background root when ctx is nil, so a
// caller that omits a parent (the compatibility constructors, tests) still
// gets a live root. Shared by NewWithContext and the view constructors; the
// package's background-root literals live only here and in New's call.
func normalizeCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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
		if a.tooSmall() {
			// Below the minimum the shell chrome cannot lay out, so the
			// children keep their last usable geometry and the App renders
			// the bounded small-terminal message instead (phase 7). The size
			// is re-forwarded once the window grows back over the minimum.
			return a, nil
		}
		a.models, _ = a.models.Update(msg)
		a.agent, _ = a.agent.Update(msg)
		a.settings, _ = a.settings.Update(msg)

	case settingsThemeMsg:
		// Live theme preview (or rollback after a discard). The config file
		// is not involved; cfg.Theme only changes when a save commits.
		a.applyTheme(msg.theme)
		return a, nil

	case agentThemeMsg:
		// The Agent slash command /theme: session-only shell-wide toggle.
		// Persisting happens through Settings save (M7 decision).
		a.applyTheme(msg.theme)
		a.note = "theme " + msg.theme + " · save in Settings to keep it"
		return a, nil

	case settingsSaveDoneMsg:
		if msg.err != nil {
			a.settings = a.settings.noteSaveError(msg.err)
			// A failed write must not leave the shell on a theme the form
			// only previewed: roll back to the theme this edit started from
			// (the same rollback the discard path performs).
			if msg.rollbackTheme != "" && a.curTheme != msg.rollbackTheme {
				a.applyTheme(msg.rollbackTheme)
			}
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
		// A sub-minimum window shows only the small-terminal notice; no key
		// (tab jumps, palette, composer, form) reaches the hidden shell.
		if a.tooSmall() {
			return a, nil
		}
		// Any keypress dismisses the transient status toast before the key
		// reaches the view it is aimed at.
		if a.note != "" {
			a.note = ""
		}
		// ctrl+p opens the command palette from any tab (phone keyboards map
		// ctrl in Blink; on a soft keyboard the Agent's "/" menu is the path).
		if k.Code == 'p' && k.Mod.Contains(tea.ModCtrl) {
			if a.canOpenPalette() {
				a.paletteOpen = true
				a.palFilter = ""
				a.palIdx = 0
			}
			return a, nil
		}
		// An open palette is its own modal: it consumes every key.
		if a.paletteOpen {
			return a.paletteKey(k)
		}
		// N5 logs drawer: ctrl+o toggles a read-only overlay from any tab,
		// over any modal — unlike the palette it never races a stream (it
		// only reads the shared log ring). While open it is its own modal
		// and consumes every key (drawerKey); ctrl+o closes it again.
		if k.Code == 'o' && k.Mod.Contains(tea.ModCtrl) && !a.paletteOpen {
			a.toggleDrawer()
			return a, nil
		}
		if a.drawerOpen {
			return a.drawerKey(k)
		}
		// An open settings form consumes every key (it is its own modal);
		// esc discards, enter/tab advance. Tab-bar and digit keys only act
		// once the form is finished.
		if a.tab == 2 && a.settings.Editing() {
			updated, cmd := a.settings.Update(msg)
			a.settings = updated
			return a, cmd
		}
		// M-01: a child modal on the ACTIVE tab owns every key — including
		// Tab/Shift-Tab, which the shell used to route to the tab bar before
		// the child was consulted (only the 1/2/3 digit jumps checked
		// ModalOpen). A tab press could therefore hide a pending mutation
		// approval, a delete/pull dialog, or the picker/help/clear overlays.
		// The child consumes or ignores the key while its modal is open;
		// ctrl+c above keeps its documented quit behavior.
		if a.tab == 0 && a.models.ModalOpen() {
			models, cmd := a.models.Update(msg)
			a.models = models
			return a, cmd
		}
		if a.tab == 1 && a.agent.ModalOpen() {
			agent, cmd := a.agent.Update(msg)
			a.agent = agent
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

	case modelsEventMsg:
		// One envelope per child: every asynchronous Models result (list/show/
		// delete fetches, streamed pull events, dialog spinner ticks) crosses
		// the shell wrapped in modelsEventMsg, so a new async result can never
		// be dropped here again — unwrap and let ModelsView route it.
		models, cmd := a.models.Update(msg.msg)
		a.models = models
		return a, cmd

	case agentEventMsg:
		// One envelope per child: every asynchronous Agent result (model-list
		// fetches, every chat activity-channel event) crosses the shell wrapped
		// in agentEventMsg — unwrap and let AgentView route it.
		agent, cmd := a.agent.Update(msg.msg)
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

// statusLeft composes the status bar's left cell (N4 observability row).
// Head: the observability segments — the model chip, a background-job pill
// (pull progress; queued-turn pills await a real queue feature, see
// ModelsView.pullPill), a compact context meter, and the last completed
// turn's measured tok/s. Tail (today's identity row): the Ollama host, the
// workspace-tools state, the canonical workspace, and — when tools are armed
// against a non-loopback host — the remote-host warning.
//
// Under width pressure the row sheds in a fixed order: workspace first
// (today's discipline), then the observability segments from their tail
// (tok/s → meter → pill, and finally the model chip), then the remote-host
// warning — host + tools always survive, on a 72-col phone too, so the N4
// anatomy `model · ctx bar · tok/s · host` is the roomy-shape anatomy while
// the privacy warning outlives the meter and the identity floor is never
// truncated mid-word. truncateToWidth is the last resort.
func (a App) statusLeft(right string) string {
	obs := make([]string, 0, 4)
	if a.agent.model != "" {
		obs = append(obs, a.agent.model)
	}
	if pill := a.models.pullPill(); pill != "" {
		obs = append(obs, pill)
	}
	if meter := a.agent.ctxMeterSegment(); meter != "" {
		obs = append(obs, meter)
	}
	if tps := a.agent.lastTokPerSec; tps > 0 {
		obs = append(obs, fmt.Sprintf("%d tok/s", tps))
	}
	tail := []string{"⏻ " + a.cfg.Host, toolsChip(a.cfg.ToolsEnabled)}
	if a.cfg.ToolsEnabled && a.cfg.Host != "" && !config.LoopbackHost(a.cfg.Host) {
		tail = append(tail, "⚠ workspace content may be sent to the remote host")
	}
	ws := canonicalWorkspaceLabel(a.cfg.WorkspaceRoot)

	budget := a.w - lipgloss.Width(right) - 3
	join := func(wsShown bool) string {
		parts := append(append([]string{}, obs...), tail...)
		if wsShown && ws != "" {
			parts = append(parts, ws)
		}
		return strings.Join(parts, " · ")
	}
	joined := join(true)
	// Today's tail discipline: the workspace yields first (it keeps its
	// last-shown position whether or not the warning is present).
	if ws != "" && lipgloss.Width(joined) > budget {
		joined = join(false)
	}
	// Then observability yields from its tail, model chip last, so a tight
	// phone keeps host + tools fully legible rather than a truncated floor.
	for lipgloss.Width(joined) > budget && len(obs) > 0 {
		obs = obs[:len(obs)-1]
		joined = join(false)
	}
	// The remote-host warning outranks the meter but still yields before the
	// host + tools floor.
	for lipgloss.Width(joined) > budget && len(tail) > 2 {
		tail = tail[:len(tail)-1]
		joined = join(false)
	}
	if budget > 0 && lipgloss.Width(joined) > budget {
		joined = truncateToWidth(joined, budget)
	}
	return joined
}

// canOpenPalette reports whether the command palette may open right now:
// never over another modal (settings form, approval, picker, pull, delete,
// the logs drawer) and never mid-generation (its actions would race the
// stream).
func (a App) canOpenPalette() bool {
	return !a.paletteOpen && !a.drawerOpen && !a.settings.Editing() && !a.models.ModalOpen() && !a.agent.ModalOpen() && !a.agent.streaming
}

// WithSessionDir enables Agent chat transcript persistence under dir (the
// resolved state dir; empty disables). Called by main after New; the host is
// recorded in each transcript's header.
func (a App) WithSessionDir(dir, host string) App {
	a.agent = a.agent.WithSessionDir(dir, host)
	return a
}

// WithUndoDir attaches the V2e undo journal's crash-artifact dir (the
// resolved state dir; empty keeps it memory-only). Called by main after New;
// a prior journal is closed and replaced so the crash artifacts land under
// the configured root.
func (a App) WithUndoDir(dir string) App {
	a.agent = a.agent.WithUndoDir(dir)
	return a
}

// WithLog attaches the shared debug logger and sink (PLAN.md §12 N5): the
// logger reaches the Ollama client and the agent runner for transport and
// loop traces, the sink feeds the logs drawer. Nil (compat constructors,
// tests) leaves the shell drawerless and ctrl+o inert.
func (a App) WithLog(l *log.Logger, sink *logsink.Sink) App {
	a.logger = l
	a.logSink = sink
	if l != nil {
		a.agent = a.agent.WithLogger(l)
		if a.client != nil {
			a.client.SetLogger(l)
		}
	}
	return a
}

// CloseSession is the normal-shutdown lifecycle boundary for chat-transcript
// persistence and the V2e undo journal: it flushes every committed turn to
// the transcript, stops the recorder worker, and GCs the undo journal's
// crash artifacts (session stacks die with the process) so nothing is
// stranded when the process returns (main calls it on the final model
// returned by Program.Run). A disabled or silent session is a no-op; the
// underlying closes are idempotent.
func (a App) CloseSession() error {
	if err := a.agent.CloseRecorder(); err != nil {
		return err
	}
	return a.agent.CloseUndo()
}

// applyTheme re-themes the whole shell (styles, tab chrome, and every child
// view). Used for live Theme previews and after a saved theme change, plus
// the session-only /theme toggles.
func (a *App) applyTheme(theme string) {
	dark := theme != "light"
	a.curTheme = theme
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

	if clientChanged {
		// Host/token changed: swap the App-owned client once and point both
		// tabs at that same new instance (P1-6). The debug logger re-attaches
		// so the new client keeps tracing, and the sink learns the new token
		// so later entries redact it too (N5).
		a.client = ollama.New(cfg.Host, cfg.AuthToken)
		if a.logger != nil {
			a.client.SetLogger(a.logger)
		}
		if a.logSink != nil {
			a.logSink.SetSecret(cfg.AuthToken)
		}
		models, mCmd := a.models.ApplyClient(a.client)
		agent, aCmd := a.agent.ApplyConfig(cfg, a.client, true)
		a.models, a.agent = models, agent
		return tea.Batch(mCmd, aCmd)
	}

	// Scalar/theme-only save: keep the existing App-owned client (both tabs
	// already hold it), re-point the Agent view's config at it, and rebuild
	// the runner for the new agent settings. No new client instance is
	// created, so the shared-client seam never drifts (P1-6).
	agent, aCmd := a.agent.ApplyConfig(cfg, a.client, false)
	a.agent = agent
	if aCmd != nil {
		return aCmd
	}
	return nil
}

func (a App) tooSmall() bool {
	// A zero-size frame (no pty size negotiated yet, M0a edge note) is stored
	// harmlessly and is not "too small": it is not a real geometry.
	if a.w == 0 || a.h == 0 {
		return false
	}
	return a.w < minTermW || a.h < minTermH
}

// renderTooSmall is the bounded small-terminal placeholder. Every row is
// truncated to the window width and the view is capped at the window height,
// so the message can never overflow — even on a 1x1 frame.
func (a App) renderTooSmall() string {
	lines := []string{
		"terminal too small",
		"",
		fmt.Sprintf("current: %dx%d", a.w, a.h),
		fmt.Sprintf("minimum: %dx%d", minTermW, minTermH),
		"",
		"enlarge the window to continue",
	}
	if maxRows := maxInt(a.h, 1); len(lines) > maxRows {
		lines = lines[:maxRows]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = truncateToWidth(l, maxInt(a.w, 1))
	}
	return strings.Join(out, "\n")
}

// View renders header + active body + status bar. The command palette
// replaces the active tab's body; the transient note is shown in the status
// bar's left cell (see Update: every keypress clears it). Below the minimum
// window geometry the whole shell is replaced by the bounded small-terminal
// message.
func (a App) View() tea.View {
	if a.tooSmall() {
		return tea.NewView(a.renderTooSmall())
	}
	header := TabBar{Active: a.tab, Styles: a.styles, Width: a.w}.Render()

	var body string
	if a.paletteOpen {
		body = a.renderPaletteOverlay()
	} else {
		switch a.tab {
		case 0:
			body = a.models.View()
		case 1:
			body = a.agent.View()
		default:
			body = a.settings.View()
		}
		if a.drawerOpen {
			// N5: the logs drawer floats over the active view's bottom rows
			// (k9s-style); it is read-only and never changes the view state.
			body = a.overlayDrawer(body)
		}
	}

	right := fmt.Sprintf("%s · %dx%d · %s", tabLabels[a.tab], a.w, a.h, BreakpointFor(a.w))
	// Transient toasts win the left cell; otherwise show the Phase 4
	// identity: host · tools state · canonical workspace, plus a remote-host
	// warning when armed tools could send workspace content off the machine
	// (space permitting).
	left := a.note
	if left == "" {
		left = a.statusLeft(right)
	}
	status := StatusBar{
		Left:   left,
		Right:  right,
		Styles: a.styles,
		Width:  a.w,
	}.Render()

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body, status))
}
