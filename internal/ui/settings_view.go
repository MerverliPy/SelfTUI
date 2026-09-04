package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/config"
)

// SettingsView is the Settings tab (PLAN.md §7/§8): a huh form over the
// config surface, grouped into Connection / Model defaults / Theme / Agent.
//
// Editing model (M4):
//   - Arriving on the Settings tab (App.switchTab) builds a fresh form bound
//     to the current session config. While the form is being edited it
//     consumes every key (enter/tab advance, shift+tab back, esc discards) —
//     the tab bar and 1/2/3 jumps are inert until the form is finished, and
//     ctrl+c keeps its root meaning (quit the app).
//   - The Theme select applies live: arrowing Dark/Light re-themes the whole
//     app immediately. Nothing is persisted until the last field is submitted,
//     at which point the config is written back to the file and the app
//     live-applies host/token/model/agent/theme.
//   - esc (discard) or a write failure leaves the config file and in-session
//     state untouched ("revert"); a theme preview is rolled back on discard.
//
// settingsValues holds the ten form-bound values on a shared allocation so
// the form's bindings (Value(&v.host)…) and the view read the same data. The
// SettingsView value is copied on every Update; the pointer survives copies.
type settingsValues struct {
	host, authToken   string
	defaultModel      string
	workspaceRoot     string
	systemPrompt      string
	theme             string
	temperature       string
	topP              string
	numCtx            string
	maxToolIterations string
}

type SettingsView struct {
	cfg    *config.Config // session config: read at Begin, mutated by App on save
	styles Styles
	state  settingsState
	w, h   int // window geometry
	form   *huh.Form
	val    *settingsValues

	// Result-panel state.
	errMsg string
	path   string

	// appliedTheme is the theme the app currently shows. The form's theme
	// value is diffed against it after every key so arrowing through the
	// Theme select previews live; a discard restores the theme the edit
	// started from (cfg.Theme, which previews never touch).
	appliedTheme string
	startTheme   string
}

type settingsState int

const (
	settingsIdle settingsState = iota
	settingsEditing
	settingsSaving
	settingsSaved
	settingsDiscarded
	settingsError
)

// NewSettingsView builds the Settings tab. cfg is the live session config the
// form edits; nothing is written until a form submission.
func NewSettingsView(cfg *config.Config, styles Styles) SettingsView {
	return SettingsView{
		cfg:          cfg,
		styles:       styles,
		state:        settingsIdle,
		val:          &settingsValues{},
		appliedTheme: cfg.Theme,
	}
}

// --- messages -------------------------------------------------------------

// settingsThemeMsg asks the App to re-theme live (a Theme preview, or the
// rollback after a discard). The config file is untouched by this message.
type settingsThemeMsg struct{ theme string }

// settingsSaveDoneMsg reports a completed config-file write. cfg is the
// snapshot that was saved; the App live-applies it on success.
type settingsSaveDoneMsg struct {
	cfg config.Config
	err error
}

// Editing reports whether the Settings tab currently owns an active form.
// The root App uses it to swallow global keys while settings are open.
func (s SettingsView) Editing() bool { return s.state == settingsEditing }

// --- lifecycle ------------------------------------------------------------

// Begin (re)builds the editing form from the current session config. Called
// every time the user enters the Settings tab, so a result panel is never
// carried into a new editing session.
func (s SettingsView) Begin() SettingsView {
	c := *s.cfg
	s.val = &settingsValues{
		host:              c.Host,
		authToken:         c.AuthToken,
		defaultModel:      c.DefaultModel,
		workspaceRoot:     c.WorkspaceRoot,
		systemPrompt:      c.Agent.SystemPrompt,
		theme:             c.Theme,
		temperature:       strconv.FormatFloat(c.Agent.Temperature, 'f', -1, 64),
		topP:              strconv.FormatFloat(c.Agent.TopP, 'f', -1, 64),
		numCtx:            strconv.Itoa(c.Agent.NumCtx),
		maxToolIterations: strconv.Itoa(c.Agent.MaxToolIterations),
	}
	s.state = settingsEditing
	s.errMsg = ""
	s.path = ""
	s.startTheme = c.Theme
	s.appliedTheme = c.Theme
	s.form = s.buildForm()

	// Focus the first field and build every group's content. The returned
	// cmds only drive cursor blink / window-size reporting, which this app
	// does not route; the focus itself is applied synchronously inside Init.
	if s.form != nil {
		s.form.Init()
	}
	return s
}

// noteSaved / noteDiscarded / noteSaveError move to result panels.
func (s SettingsView) noteSaved(path string) SettingsView {
	s.state = settingsSaved
	s.path = path
	return s
}

func (s SettingsView) noteDiscarded() SettingsView {
	s.state = settingsDiscarded
	return s
}

func (s SettingsView) noteSaveError(err error) SettingsView {
	s.state = settingsError
	s.errMsg = err.Error()
	return s
}

// applyTheme refreshes the settings chrome (and the form's palette, when one
// exists) to the app's current dark/light state.
func (s SettingsView) applyTheme(dark bool, styles Styles) SettingsView {
	s.styles = styles
	if s.form != nil {
		s.form.WithTheme(formTheme(dark))
	}
	return s
}

// --- form construction ----------------------------------------------------

// policyValidator returns a huh validator that enforces the config package's
// single validation policy for one form field: it builds the config the form
// would currently save (snapshot of the bound values, which huh keeps live on
// every keystroke) and asks config.Validate, surfacing only errors that belong
// to this field via its stable marker substring. Settings no longer keeps its
// own copy of ranges or URL rules — that policy drifted once (max tool
// iterations 1..256 here vs 1..100 in config) — so Load, Save, and this form
// all share config.Validate. shape, when non-nil, first rejects a value that
// does not even parse as its type (config.Validate works on typed values).
func (s SettingsView) policyValidator(marker string, shape func(string) error) func(string) error {
	return func(raw string) error {
		if shape != nil {
			if err := shape(raw); err != nil {
				return err
			}
		}
		if err := config.Validate(s.snapshot()); err != nil {
			if marker == "" || strings.Contains(err.Error(), marker) {
				return err
			}
			// The config is invalid because of another field; that field's own
			// validator will surface it, and Save re-validates regardless.
		}
		return nil
	}
}

// numberShape rejects a value that does not parse as a number (the form fields
// are strings; config.Validate operates on the typed config).
func numberShape(name string) func(string) error {
	return func(raw string) error {
		if _, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err != nil {
			return fmt.Errorf("%s must be a number", name)
		}
		return nil
	}
}

// integerShape rejects a value that does not parse as an integer.
func integerShape(name string) func(string) error {
	return func(raw string) error {
		if _, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err != nil {
			return fmt.Errorf("%s must be an integer", name)
		}
		return nil
	}
}

// buildForm assembles the editing form from the shared bound values (s.val),
// which Begin seeded from the session config.
func (s SettingsView) buildForm() *huh.Form {
	v := s.val
	hostField := huh.NewInput().
		Title("Host").
		Description("Ollama base URL — must include http:// or https://.").
		Placeholder("http://localhost:11434").
		Validate(s.policyValidator("config: host:", nil)).
		Value(&v.host)

	tokenField := huh.NewInput().
		Title("Auth token").
		Description("Optional bearer token for remote hosts. Empty = none.").
		Placeholder("unset").
		EchoMode(huh.EchoModePassword).
		Validate(s.policyValidator("bearer token requires HTTPS", nil)).
		Value(&v.authToken)

	groupConnection := huh.NewGroup(
		hostField,
		tokenField,
	).Title("Connection").Description("Which Ollama host SelfTUI talks to; applied to new requests on save.")

	groupModelDefaults := huh.NewGroup(
		huh.NewInput().
			Title("Default model").
			Description("Auto-selected for Agent chat when nothing is picked. Empty = first installed model.").
			Placeholder("qwen3:8b").
			Value(&v.defaultModel),
		huh.NewInput().
			Title("Temperature").
			Description("Sampling randomness; 0 = deterministic.").
			Placeholder("0.7").
			Validate(s.policyValidator("config: agent: temperature", numberShape("temperature"))).
			Value(&v.temperature),
		huh.NewInput().
			Title("Top-p").
			Description("Nucleus sampling cutoff.").
			Placeholder("0.9").
			Validate(s.policyValidator("config: agent: top_p", numberShape("top-p"))).
			Value(&v.topP),
		huh.NewInput().
			Title("Context window").
			Description("num_ctx sent per request.").
			Placeholder("4096").
			Validate(s.policyValidator("config: agent: num_ctx", integerShape("context window"))).
			Value(&v.numCtx),
	).Title("Model defaults").Description("Session defaults for the Agent tab; applied live on save.")

	groupTheme := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Theme").
			Description("Applies immediately as you arrow; saved on submit.").
			Options(
				huh.NewOption("Dark", "dark"),
				huh.NewOption("Light", "light"),
			).
			Value(&v.theme),
	).Title("Theme").Description("The preview rolls back if you discard with esc.")

	groupAgent := huh.NewGroup(
		huh.NewText().
			Title("System prompt").
			Description("Persona for new agent runs. Doesn't rewrite old transcripts.").
			Lines(3).
			Value(&v.systemPrompt),
		huh.NewInput().
			Title("Workspace root").
			Description("Project root the agent tools are jailed to. Empty = current directory.").
			Placeholder("/home/you/project").
			Validate(s.policyValidator("config: workspace_root:", nil)).
			Value(&v.workspaceRoot),
		huh.NewInput().
			Title("Max tool iterations").
			Description("Upper bound on tool calls inside one agent run.").
			Placeholder("12").
			Validate(s.policyValidator("config: agent: max_tool_iterations", integerShape("max tool iterations"))).
			Value(&v.maxToolIterations),
	).Title("Agent").Description("Applied to the next agent run on save.")

	km := huh.NewDefaultKeyMap()
	// Esc discards instead of the default ctrl+c, which the root App keeps
	// as quit-app. WithKeyMap also re-distributes field positions.
	km.Quit = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "discard"))

	form := huh.NewForm(groupConnection, groupModelDefaults, groupTheme, groupAgent).
		WithKeyMap(km).
		WithTheme(formTheme(v.theme != "light"))

	switch BreakpointFor(s.w) {
	case Wide:
		// Two-column form sections on wide screens (PLAN §7); narrow stays
		// single-column with one group per page.
		form.WithLayout(huh.LayoutColumns(2))
	default:
		form.WithLayout(huh.LayoutDefault)
	}

	if s.w > 0 {
		form.WithWidth(s.w)
	}
	// huh draws a footer (scroll/next hints) below the field area when a
	// group overflows the given height, so the budget must leave that row
	// inside the body: h-2 rows are available, of which one goes to the
	// footer (measured at the 72x30 device geometry in M5).
	if bodyH := maxInt(s.h-3, 8); s.h > 0 {
		form.WithHeight(bodyH)
	}
	return form
}

// formTheme adapts a huh palette to the app's dark/light state. huh only
// auto-detects the background when the runtime reports one, which this program
// does not enable, so the theme is pinned to the config choice.
func formTheme(dark bool) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles { return huh.ThemeCharm(dark) })
}

// --- update ---------------------------------------------------------------

func (s SettingsView) Update(msg tea.Msg) (SettingsView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return s.resize(msg.Width, msg.Height), nil
	case settingsSaveDoneMsg:
		return s, nil // the App owns the live apply (settingsSaveDoneMsg there)
	}

	if s.state != settingsEditing || s.form == nil {
		return s, nil
	}

	// Everything else — keys, plus the internal next/prev field/group and
	// updateField messages bubble tea re-feeds from form commands — goes to
	// the form.
	updated, cmd := s.form.Update(msg)
	if f, ok := updated.(*huh.Form); ok {
		s.form = f
	}

	switch s.form.State {
	case huh.StateCompleted:
		// Submitted: persist off-loop, then report back for the live apply.
		// The form has quit; show a transient saving panel until the write
		// resolves (settingsSaveDoneMsg handled by the App).
		s.state = settingsSaving
		snapshot := s.snapshot()
		return s, func() tea.Msg {
			return settingsSaveDoneMsg{cfg: snapshot, err: config.Save(snapshot)}
		}

	case huh.StateAborted:
		// Discarded: nothing was written. Roll a live theme preview back to
		// the theme this editing session started from.
		restore := s.appliedTheme != s.startTheme
		s = s.noteDiscarded()
		if restore {
			return s, func() tea.Msg { return settingsThemeMsg{theme: s.startTheme} }
		}
		return s, nil
	}

	// Live theme preview: huh pushes the Theme selection into the bound
	// accessor as the user arrows, so a changed value re-themes the app now.
	// cfg.Theme stays untouched until a save commits it.
	if s.val.theme != s.appliedTheme {
		s.appliedTheme = s.val.theme
		return s, func() tea.Msg { return settingsThemeMsg{theme: s.val.theme} }
	}

	return s, cmd
}

// resize updates geometry, reflowing an active form. Crossing the wide/medium
// boundary changes the group layout (columns vs single page), which needs a
// rebuild; the bound values survive because the form writes into this struct.
func (s SettingsView) resize(w, h int) SettingsView {
	if s.w == w && s.h == h {
		return s
	}
	wideChanged := BreakpointFor(s.w) == Wide && BreakpointFor(w) != Wide ||
		BreakpointFor(s.w) != Wide && BreakpointFor(w) == Wide
	s.w, s.h = w, h

	if s.form == nil || s.state != settingsEditing {
		return s
	}
	if wideChanged {
		s.form = s.buildForm()
		if s.form != nil {
			s.form.Init()
		}
		return s
	}
	if w > 0 {
		s.form.WithWidth(w)
	}
	if bodyH := maxInt(h-3, 8); h > 0 {
		// h-3 reserves huh's footer row inside the body (see buildForm).
		s.form.WithHeight(bodyH)
	}
	return s
}

// snapshot assembles the config value a submission persists and the App
// live-applies. The field validators guarantee the numeric strings parse.
func (s SettingsView) snapshot() config.Config {
	c := *s.cfg
	c.Host = strings.TrimSpace(s.val.host)
	c.AuthToken = s.val.authToken
	c.DefaultModel = strings.TrimSpace(s.val.defaultModel)
	c.Theme = s.val.theme
	c.WorkspaceRoot = strings.TrimSpace(s.val.workspaceRoot)
	a := c.Agent
	a.SystemPrompt = s.val.systemPrompt
	if f, err := strconv.ParseFloat(strings.TrimSpace(s.val.temperature), 64); err == nil {
		a.Temperature = f
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(s.val.topP), 64); err == nil {
		a.TopP = f
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(s.val.numCtx), 10, 64); err == nil {
		a.NumCtx = int(n)
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(s.val.maxToolIterations), 10, 64); err == nil {
		a.MaxToolIterations = int(n)
	}
	c.Agent = a
	return c
}

// --- rendering ------------------------------------------------------------

func (s SettingsView) View() string {
	switch s.state {
	case settingsIdle:
		return s.renderPanel("Settings", "press enter to edit", []string{
			"host · auth token · model defaults · theme · agent",
			"enter edit · ctrl+c quit",
		})
	case settingsEditing:
		if s.form == nil {
			return ""
		}
		return s.form.View()
	case settingsSaving:
		return s.renderPanel("Saving settings", "writing config…", nil)
	case settingsSaved:
		return s.renderPanel("Settings saved", "written to "+s.path, []string{
			"changes are live in this session",
			"enter edit again · tab switch tabs · ctrl+c quit",
		})
	case settingsDiscarded:
		return s.renderPanel("Changes discarded", "nothing was written", []string{
			"enter edit again · tab switch tabs · ctrl+c quit",
		})
	default: // settingsError
		return s.renderPanel("Could not save settings", s.errMsg, []string{
			"enter to try again · esc / switch tabs to cancel",
		})
	}
}

// renderPanel centers a bordered box over the settings body.
func (s SettingsView) renderPanel(title, sub string, hints []string) string {
	bodyW, bodyH := maxInt(s.w, 1), maxInt(s.h-2, 1)

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(s.styles.accent).Render(title),
		"",
		sub,
	}
	for _, h := range hints {
		lines = append(lines, "", s.styles.Placeholder.Render(h))
	}
	content := strings.Join(lines, "\n")

	box := s.styles.Pane.Render(content)
	return lipgloss.Place(bodyW, maxInt(bodyH, 1), lipgloss.Center, lipgloss.Center, box)
}
