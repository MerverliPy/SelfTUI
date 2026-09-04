package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/ollama"
)

// ModelsView is the Models tab (PLAN.md §7): a selectable model list plus an
// inspect pane fed by POST /api/show. M1a delivered list + inspect; M1b adds
// delete (x → confirm → y/esc) and streaming pull (p → name → spinner +
// progress, esc cancels). Layout (side-by-side vs stacked) comes from the
// shared breakpoint system; theme comes from the shared Styles.
type ModelsView struct {
	client *ollama.Client

	// ctx is the parent context every operation this view starts (list/show/
	// delete fetches, pull streams) derives from. NewWithContext binds it to
	// the process root; the compatibility constructor leaves it at Background.
	ctx context.Context

	styles Styles

	// List state.
	models  []ollama.Model
	listErr string
	loading bool

	// Detail state.
	detail      *ollama.Details
	detailName  string // model the current detail belongs to
	detailErr   string
	loadingShow bool
	showPane    bool // stacked layout: detail pane toggled open
	scroll      int  // detail pane scroll offset (lines)

	// Transient feedback ("deleted qwen3:8b"…), cleared on the next reload.
	notice string

	// Delete flow (x → confirm → y/esc).
	confirmDelete bool
	deleteTarget  string
	deleting      bool
	deleteErr     string

	// Pull flow (p → name input → stream; esc cancels).
	inputMode  bool
	input      textinput.Model
	pulling    bool
	pullName   string
	pullErr    string
	pullStatus string
	pullDigest string
	pullTotal  int64
	pullDone   int64
	pullCh     chan tea.Msg // activity channel (PLAN §8), owned by one pull
	pullCancel func()       // cancels the in-flight pull context

	// Reusable dialog widgets.
	spinner  spinner.Model
	progress progress.Model

	selIdx int

	list list.Model
	w, h int
}

// NewModelsView builds the Models tab with a background parent context. It
// is the compatibility constructor for tests and callers that predate
// root-context wiring; new code should go through the App's NewWithContext
// (or newModelsView with the real parent) so operations cancel with the
// process.
func NewModelsView(client *ollama.Client, styles Styles, theme string) ModelsView {
	return newModelsView(nil, client, styles, theme)
}

// newModelsView is the private constructor: it stores ctx as the parent
// every operation this view starts derives from. nil is normalized to a
// background root (see NewModelsView).
func newModelsView(ctx context.Context, client *ollama.Client, styles Styles, theme string) ModelsView {
	ctx = normalizeCtx(ctx)
	dark := theme != "light"

	l := list.New(nil, modelsDelegate(styles, dark), 0, 0)
	l.Title = "Models"
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetStatusBarItemName("model", "models")
	l.DisableQuitKeybindings()
	// Free 'u'/'d' (hover-scroll is not wanted on a phone) for the detail
	// pane by removing them from page navigation.
	l.KeyMap.PrevPage = key.NewBinding(key.WithKeys("pgup", "left", "h", "b"))
	l.KeyMap.NextPage = key.NewBinding(key.WithKeys("pgdown", "right", "l", "f"))
	l.Styles = list.DefaultStyles(dark)

	accent := styles.accent
	sp := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)),
	)
	ti := textinput.New()
	ti.Placeholder = "qwen3:0.6b"
	ti.Prompt = "❯ "
	ti.CharLimit = 120

	return ModelsView{
		client:   client,
		ctx:      ctx,
		styles:   styles,
		spinner:  sp,
		progress: progress.New(),
		input:    ti,
		list:     l,
	}
}

// modelsDelegate themes the bubbles default delegate with our palette so the
// selected row reads like the rest of the app (violet accent).
func modelsDelegate(styles Styles, dark bool) list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	// Title line + one description line per model, no inter-item gap:
	// keeps ~6 models visible in the compact phone pane.
	d.SetHeight(2)
	d.SetSpacing(0)

	s := list.NewDefaultItemStyles(dark)
	accent := styles.accent
	s.SelectedTitle = s.SelectedTitle.BorderForeground(accent).Foreground(accent)
	s.SelectedDesc = s.SelectedDesc.Foreground(accent)
	d.Styles = s
	return d
}

// modelsItem adapts an ollama.Model to a bubbles list item.
type modelsItem struct {
	model ollama.Model
}

func (i modelsItem) Title() string       { return i.model.Name }
func (i modelsItem) Description() string { return modelSummary(i.model) }
func (i modelsItem) FilterValue() string { return i.model.Name }

// modelSummary is the one-line description under each model name.
func modelSummary(m ollama.Model) string {
	parts := []string{m.Family}
	if m.ParameterSize != "" {
		parts = append(parts, m.ParameterSize)
	}
	if m.Quantization != "" {
		parts = append(parts, m.Quantization)
	}
	return strings.Join(parts, " · ")
}

// --- messages -------------------------------------------------------------

type modelsLoadedMsg struct{ list []ollama.Model }
type modelsLoadErrMsg struct{ err string }
type modelsShowMsg struct {
	name    string
	details ollama.Details
}
type modelsShowErrMsg struct {
	name string
	err  string
}
type modelsDeleteDoneMsg struct {
	name string
	err  string
}
type modelsPullMsg struct {
	name     string
	progress ollama.PullProgress
}
type modelsPullDoneMsg struct {
	name string
	err  string
}

// Init starts the first list fetch. Called once from the root App.
func (v ModelsView) Init() tea.Cmd {
	v.loading = true
	return v.loadCmd()
}

func (v ModelsView) loadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(v.ctx, 60*time.Second)
		defer cancel()
		models, err := v.client.List(ctx)
		if err != nil {
			return modelsLoadErrMsg{err: err.Error()}
		}
		return modelsLoadedMsg{list: models}
	}
}

func (v ModelsView) showCmd(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(v.ctx, 60*time.Second)
		defer cancel()
		details, err := v.client.Show(ctx, name)
		if err != nil {
			return modelsShowErrMsg{name: name, err: err.Error()}
		}
		return modelsShowMsg{name: name, details: details}
	}
}

func (v ModelsView) deleteCmd(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(v.ctx, 60*time.Second)
		defer cancel()
		if err := v.client.Delete(ctx, name); err != nil {
			return modelsDeleteDoneMsg{name: name, err: err.Error()}
		}
		return modelsDeleteDoneMsg{name: name}
	}
}

// waitPullCmd is the resubscribed activity command (PLAN §8): it blocks until
// the pull goroutine posts its next message. Return it again from every
// progress update so the stream keeps flowing.
func (v ModelsView) waitPullCmd() tea.Cmd {
	ch := v.pullCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg { return <-ch }
}

// startPull begins a streaming pull: a background goroutine runs
// client.Pull while the UI stays interactive. Progress events arrive as
// modelsPullMsg; the trailing result as one modelsPullDoneMsg.
func (v ModelsView) startPull(name string) (ModelsView, tea.Cmd) {
	ch := make(chan tea.Msg, 64)
	ctx, cancel := context.WithCancel(v.ctx)

	v.pullCh = ch
	v.pullCancel = cancel
	v.pulling = true
	v.pullName = name
	v.pullErr = ""
	v.pullStatus = "starting"
	v.pullDigest = ""
	v.pullTotal, v.pullDone = 0, 0

	go func() {
		defer close(ch)
		defer cancel()
		err := v.client.Pull(ctx, name, func(p ollama.PullProgress) {
			ch <- modelsPullMsg{name: name, progress: p}
		})
		if err != nil {
			ch <- modelsPullDoneMsg{name: name, err: err.Error()}
			return
		}
		ch <- modelsPullDoneMsg{name: name}
	}()

	return v, v.waitPullCmd()
}

func (v ModelsView) spinnerTick() tea.Cmd {
	return func() tea.Msg { return v.spinner.Tick() }
}

// ModalOpen reports whether the Models tab is showing a modal (confirm /
// name input / pull). The root App uses it so global keys (1/2/3 tab jump)
// cannot steal characters typed into a dialog.
func (v ModelsView) ModalOpen() bool {
	return v.confirmDelete || v.inputMode || v.pulling || v.deleting
}

// --- update ---------------------------------------------------------------

// Update handles messages aimed at the Models tab. The root App forwards
// keys only when this tab is active; async results are forwarded always.
func (v ModelsView) Update(msg tea.Msg) (ModelsView, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, 3)
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.w, v.h = msg.Width, msg.Height
		return v, nil

	case modelsLoadedMsg:
		v, cmd = v.onLoaded(msg.list)
		cmds = append(cmds, cmd)

	case modelsLoadErrMsg:
		v.loading = false
		v.listErr = msg.err
		v.notice = ""
		return v, nil

	case modelsShowMsg:
		v.detail = &msg.details
		v.detailName = msg.name
		v.detailErr = ""
		v.loadingShow = false
		v.scroll = 0
		return v, nil

	case modelsShowErrMsg:
		v.detailErr = msg.err
		v.loadingShow = false
		return v, nil

	case spinner.TickMsg:
		v.spinner, _ = v.spinner.Update(msg)

	case modelsDeleteDoneMsg:
		v, cmd = v.onDeleteDone(msg)
		cmds = append(cmds, cmd)

	case modelsPullMsg:
		v, cmd = v.onPullProgress(msg)
		cmds = append(cmds, cmd)

	case modelsPullDoneMsg:
		v, cmd = v.onPullDone(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		v, cmd = v.handleKey(msg)
		cmds = append(cmds, cmd)
	}

	// Keep the dialog spinner ticking while it is actually visible
	// (deleting / pulling). The name-input and confirm states have no spinner.
	if v.deleting || v.pulling {
		cmds = append(cmds, v.spinnerTick())
	}
	if len(cmds) == 0 {
		return v, nil
	}
	return v, tea.Batch(cmds...)
}

// onLoaded replaces the model list. On wide/medium-split layouts the inspect
// pane is always visible, so the first model is inspected immediately.
func (v ModelsView) onLoaded(models []ollama.Model) (ModelsView, tea.Cmd) {
	v.models = models
	v.loading = false
	v.listErr = ""
	v.pullErr = ""
	v.notice = ""
	v.selIdx = 0

	items := make([]list.Item, len(models))
	for i, m := range models {
		items[i] = modelsItem{m}
	}
	cmds := []tea.Cmd{v.list.SetItems(items)}

	if ForModels(v.w).SideBySide && len(models) > 0 {
		v.loadingShow = true
		cmds = append(cmds, v.showCmd(models[0].Name))
	}
	return v, tea.Batch(cmds...)
}

// onDeleteDone finalizes a DELETE round-trip. Success closes the dialog and
// reloads the list; failure keeps the dialog open so the error sits right
// under the question for an immediate retry.
func (v ModelsView) onDeleteDone(m modelsDeleteDoneMsg) (ModelsView, tea.Cmd) {
	v.deleting = false
	if m.err != "" {
		v.confirmDelete = true
		v.deleteErr = m.err
		return v, nil
	}
	v.confirmDelete = false
	v.deleteTarget = ""
	v.notice = "deleted " + m.name
	if v.detailName == m.name {
		v.detail = nil
		v.detailName = ""
	}
	return v, v.loadCmd()
}

// onPullProgress applies one streamed pull event and resubscribes the
// activity command. A new layer digest resets the progress numbers.
func (v ModelsView) onPullProgress(m modelsPullMsg) (ModelsView, tea.Cmd) {
	p := m.progress
	v.pullStatus = p.Status
	if p.Digest != "" {
		if p.Digest != v.pullDigest {
			v.pullDigest = p.Digest
			v.pullTotal, v.pullDone = 0, 0
		}
		v.pullTotal, v.pullDone = p.Total, p.Completed
	}
	return v, v.waitPullCmd()
}

// onPullDone ends a pull: success reloads the list (the new model appears);
// failure surfaces the error in the view.
func (v ModelsView) onPullDone(m modelsPullDoneMsg) (ModelsView, tea.Cmd) {
	v.pulling = false
	v.pullCh = nil
	v.pullCancel = nil
	if m.err != "" {
		v.pullErr = m.err
		return v, nil
	}
	v.notice = "pulled " + m.name
	return v, v.loadCmd()
}

// handleKey is the Models-tab key dispatcher. Modal states (confirm, input,
// pull) swallow every key except their own. Otherwise list navigation is
// delegated to the bubbles list and the keys below are Models-view specific.
func (v ModelsView) handleKey(msg tea.KeyMsg) (ModelsView, tea.Cmd) {
	if _, isPress := msg.(tea.KeyPressMsg); !isPress {
		return v, nil
	}
	k := msg.Key()

	switch {
	case v.deleting:
		return v, nil // in-flight delete: ignore keys

	case v.pulling:
		if k.Code == tea.KeyEsc && v.pullCancel != nil {
			v.pullCancel() // pull goroutine reports "context canceled" next
			v.pullStatus = "cancelling…"
		}
		return v, nil

	case v.confirmDelete:
		return v.confirmKey(k)

	case v.inputMode:
		return v.inputKey(msg, k)
	}

	switch {
	case k.Code == tea.KeyEnter:
		return v.inspectSelected()

	case k.Code == tea.KeyEsc:
		if !ForModels(v.w).SideBySide && v.showPane {
			v.showPane = false
		}
		return v, nil

	case k.Text == "x":
		return v.beginDelete()

	case k.Text == "p":
		return v.beginPull()

	case k.Text == "r":
		v.loading = true
		return v, v.loadCmd()

	case k.Text == "d" && v.paneVisible() && v.detail != nil:
		v.scroll++
		v.clampScroll()
		return v, nil

	case k.Text == "u" && v.paneVisible() && v.detail != nil:
		v.scroll--
		v.clampScroll()
		return v, nil
	}

	// Everything else goes to the list component.
	updated, cmd := v.list.Update(msg)
	v.list = updated

	// Auto-inspect on selection change when the pane is always visible.
	if ForModels(v.w).SideBySide && len(v.models) > 0 {
		cur := v.list.Index()
		if cur != v.selIdx {
			v.selIdx = cur
			if !v.loadingShow && v.detailName != v.modelName(cur) {
				return v, tea.Batch(cmd, v.showCmd(v.modelName(cur)))
			}
		}
	}
	return v, cmd
}

// --- delete flow ----------------------------------------------------------

// beginDelete opens the confirm dialog for the selected model (x).
func (v ModelsView) beginDelete() (ModelsView, tea.Cmd) {
	if len(v.models) == 0 {
		return v, nil
	}
	name := v.modelName(v.list.Index())
	if name == "" {
		return v, nil
	}
	v.confirmDelete = true
	v.deleteTarget = name
	v.deleteErr = ""
	return v, nil
}

// confirmKey handles y/n/esc inside the delete dialog.
func (v ModelsView) confirmKey(k tea.Key) (ModelsView, tea.Cmd) {
	switch {
	case k.Text == "y":
		v.confirmDelete = false
		v.deleting = true
		v.deleteErr = ""
		return v, v.deleteCmd(v.deleteTarget)
	case k.Text == "n" || k.Code == tea.KeyEsc:
		v.confirmDelete = false
		v.deleteTarget = ""
		return v, nil
	}
	return v, nil
}

// --- pull flow ------------------------------------------------------------

// beginPull opens the model-name input (p).
func (v ModelsView) beginPull() (ModelsView, tea.Cmd) {
	v.inputMode = true
	v.pullErr = ""
	ti := v.input
	ti.SetValue("")
	v.input = ti
	return v, v.input.Focus()
}

// inputKey handles keys in the name-entry dialog. esc aborts; enter starts
// the pull; everything else goes to the textinput.
func (v ModelsView) inputKey(msg tea.KeyMsg, k tea.Key) (ModelsView, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		v.inputMode = false
		v.input.Blur()
		return v, nil
	case k.Code == tea.KeyEnter:
		name := strings.TrimSpace(v.input.Value())
		if name == "" {
			return v, nil
		}
		v.inputMode = false
		v.input.Blur()
		return v.startPull(name)
	}
	ti, cmd := v.input.Update(msg)
	v.input = ti
	return v, cmd
}

func (v ModelsView) targetSize(name string) string {
	for _, m := range v.models {
		if m.Name == name && m.SizeBytes > 0 {
			return humanBytes(m.SizeBytes)
		}
	}
	return ""
}

// --- view -----------------------------------------------------------------

// View renders list + inspect per the breakpoint layout. A modal state
// (confirm / input / pull) replaces the whole body with a centered dialog.
func (v ModelsView) View() string {
	layout := ForModels(v.w)
	bodyH := v.h - 2 // tab bar + status bar

	switch {
	case v.confirmDelete:
		return v.renderOverlay(bodyH, "Delete model", v.confirmLines())
	case v.inputMode:
		return v.renderOverlay(bodyH, "Pull a model", v.inputLines())
	case v.pulling:
		return v.renderOverlay(bodyH, "Pulling "+v.pullName, v.pullLines())
	}

	if len(v.models) == 0 {
		return v.fullSizePane(layout, bodyH)
	}

	listW, listH := v.w, bodyH
	detailW, detailH := 0, 0
	switch {
	case layout.SideBySide:
		listW, detailW = layout.ListWidth, layout.DetailWidth(v.w)
	case v.showPane:
		listH = bodyH * 40 / 100
		detailH = bodyH - listH
	default:
		// Compact stacked list reserves one row for the hint/notice line.
		listH = maxInt(bodyH-1, 1)
	}

	listPane := v.renderList(listW-2, listH-2)

	if !layout.SideBySide && !v.showPane {
		if v.listErr != "" {
			return listPane + "\n" + v.styles.Error.Render("⚠ "+v.listErr+" — press r to retry")
		}
		return listPane + "\n" + v.hintLine()
	}

	detailW, detailH = v.detailPaneDims()
	detail := v.renderDetail(detailW, detailH)
	if layout.SideBySide {
		return lipgloss.JoinHorizontal(lipgloss.Top, listPane, detail)
	}
	return listPane + "\n" + detail
}

// hintLine is the one-row strip under the stacked list: a transient notice,
// a surfaced pull error, or the Models-tab legend.
func (v ModelsView) hintLine() string {
	if v.pullErr != "" {
		return v.styles.Error.Render("⚠ " + v.pullErr)
	}
	if v.notice != "" {
		return v.styles.Placeholder.Render(v.notice)
	}
	return v.styles.Placeholder.Render("x delete · p pull · r refresh")
}

// fullSizePane renders the loading / error / empty states that fill the body.
func (v ModelsView) fullSizePane(layout ModelsLayout, bodyH int) string {
	pane := v.styles.Pane.Width(v.w - 2).Height(bodyH - 2)
	var content string
	switch {
	case v.loading:
		content = v.styles.Placeholder.Render("⏳ loading models…")
		if v.notice != "" {
			content += "\n" + v.styles.Placeholder.Render(v.notice)
		}
	case v.listErr != "":
		content = v.styles.Error.Render("⚠ "+v.listErr) + "\n" +
			v.styles.Placeholder.Render("press r to retry")
	case v.pullErr != "":
		content = v.styles.Error.Render("⚠ pull failed: "+v.pullErr) + "\n" +
			v.styles.Placeholder.Render("press p to try again · r to refresh")
	case layout.SideBySide:
		content = v.styles.Placeholder.Render("no models installed")
	default:
		content = v.styles.Placeholder.Render("no models installed — press p to pull one") + "\n" +
			v.styles.Placeholder.Render("press r to refresh")
	}
	return pane.Render(content)
}

func (v ModelsView) renderList(w, h int) string {
	if w < 1 || h < 1 {
		return ""
	}
	v.list.SetSize(w, h)
	return v.styles.Pane.Width(w + 2).Height(h + 2).Render(v.list.View())
}

// renderDetail renders the inspect pane: key facts + scrollable sections from
// POST /api/show, plus any dangling notice/pull error. Returns "" when the
// geometry has no room.
func (v ModelsView) renderDetail(w, h int) string {
	if w < 1 || h < 1 {
		return ""
	}
	body := v.styles.Pane.Width(w + 2).Height(h + 2)

	if v.loadingShow || (v.detail == nil && v.detailErr == "") {
		return body.Render(v.styles.Placeholder.Render("⏳ inspecting…"))
	}
	if v.detailErr != "" {
		return body.Render(v.styles.Error.Render("⚠ " + v.detailErr))
	}

	lines := wrapLines(v.detailLines(w), w)
	if prefix := v.detailPrefix(); prefix != "" {
		lines = append([]string{prefix, ""}, lines...)
	}
	v.scroll = clampInt(v.scroll, 0, maxInt(0, len(lines)-h))
	window := lines[v.scroll:]
	if len(window) > h {
		window = window[:h]
	}
	return body.Render(lipgloss.JoinVertical(lipgloss.Left, window...))
}

// detailPrefix is the notice/error line shown at the top of the detail pane
// on side-by-side layouts (the compact equivalent is hintLine).
func (v ModelsView) detailPrefix() string {
	if v.pullErr != "" {
		return v.styles.Error.Render("⚠ " + v.pullErr)
	}
	if v.notice != "" {
		return v.styles.Placeholder.Render(v.notice)
	}
	return ""
}

// --- dialogs --------------------------------------------------------------

// renderOverlay centers a bordered dialog over the whole Models body and
// returns a body-height string, so the app's header/body/status stack stays
// exactly h rows. Content is capped at bodyH-4 rows (see fitContent) so a
// dialog can never overflow the terminal height on a phone.
func (v ModelsView) renderOverlay(bodyH int, title string, lines []string) string {
	innerW := maxInt(v.w-6, 16)
	wrapped := wrapLines(lines, innerW)
	wrapped = fitContent(wrapped, maxInt(bodyH-4, 4))
	padded := make([]string, len(wrapped))
	for i, l := range wrapped {
		padded[i] = l + strings.Repeat(" ", maxInt(0, innerW-lipgloss.Width(l)))
	}
	content := strings.Join(padded, "\n")
	box := v.styles.Pane.Render(
		lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(title) + "\n\n" + content,
	)
	return lipgloss.Place(v.w, maxInt(1, bodyH), lipgloss.Center, lipgloss.Center, box)
}

// confirmLines builds the delete-confirm dialog body.
func (v ModelsView) confirmLines() []string {
	if v.deleting {
		return []string{v.spinner.View() + " deleting " + v.deleteTarget + "…"}
	}
	lines := []string{"Delete " + v.deleteTarget + "?"}
	if sz := v.targetSize(v.deleteTarget); sz != "" {
		lines = append(lines, "size "+sz)
	}
	lines = append(lines, "", "y confirm · esc cancel")
	if v.deleteErr != "" {
		lines = append(lines, "", v.styles.Error.Render("⚠ "+v.deleteErr))
	}
	return lines
}

// inputLines builds the pull name-entry dialog body.
func (v ModelsView) inputLines() []string {
	w := maxInt(v.w-14, 16)
	ti := v.input
	ti.SetWidth(w)
	v.input = ti
	return []string{
		"model name like qwen3:0.6b (default tag: latest)",
		"",
		v.input.View(),
		"",
		"enter pull · esc cancel",
	}
}

// pullLines builds the streaming pull dialog body: spinner + phase, and the
// progress bar once the server reports a layer size.
func (v ModelsView) pullLines() []string {
	lines := []string{v.spinner.View() + " " + v.pullStatus}
	if v.pullTotal > 0 {
		pct := clampFloat(float64(v.pullDone)/float64(v.pullTotal), 0, 1)
		pr := v.progress
		pr.SetWidth(maxInt(v.w-12, 20))
		v.progress = pr
		lines = append(lines, "",
			pr.ViewAs(pct),
			humanBytes(v.pullDone)+" / "+humanBytes(v.pullTotal),
		)
	}
	lines = append(lines, "", "esc cancel")
	return lines
}

// inspectSelected opens the detail pane (compact) or refreshes it, fetching
// POST /api/show when the selected model is not already shown.
func (v ModelsView) inspectSelected() (ModelsView, tea.Cmd) {
	if len(v.models) == 0 {
		return v, nil
	}
	name := v.modelName(v.list.Index())

	if !ForModels(v.w).SideBySide {
		// Compact: enter toggles the stacked pane; enter again on the same
		// model closes it.
		if v.showPane && v.detailName == name {
			v.showPane = false
			return v, nil
		}
		v.showPane = true
	}
	if v.detailName == name && v.detail != nil && !v.loadingShow {
		return v, nil // already inspecting this model
	}
	v.loadingShow = true
	v.detailErr = ""
	return v, v.showCmd(name)
}

func (v ModelsView) paneVisible() bool {
	return ForModels(v.w).SideBySide || v.showPane
}

// clampScroll bounds the detail scroll offset by the currently visible
// window so the stored offset never drifts out of range between renders.
func (v *ModelsView) clampScroll() {
	if v.scroll > v.maxScroll() {
		v.scroll = v.maxScroll()
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
}

// maxScroll returns the highest valid scroll offset for the detail pane.
func (v ModelsView) maxScroll() int {
	w, h := v.detailPaneDims()
	if w < 1 || v.detail == nil {
		return 0
	}
	return maxInt(0, len(wrapLines(v.detailLines(w), w))-h)
}

// detailPaneDims returns the inner (content) width and height of the detail
// pane for the current geometry, or (0, 0) when it has no space.
func (v ModelsView) detailPaneDims() (int, int) {
	layout := ForModels(v.w)
	bodyH := v.h - 2
	switch {
	case layout.SideBySide:
		return layout.DetailWidth(v.w) - 2, bodyH - 2
	case v.showPane:
		listH := bodyH * 40 / 100
		return v.w - 2, bodyH - listH - 2
	default:
		return 0, 0
	}
}

func (v ModelsView) modelName(i int) string {
	if i < 0 || i >= len(v.models) {
		return ""
	}
	return v.models[i].Name
}

// detailLines builds the full detail text (unwrapped) for the selected model.
func (v ModelsView) detailLines(width int) []string {
	m := v.models[clampInt(v.list.Index(), 0, len(v.models)-1)]
	d := v.detail

	lines := []string{
		lipgloss.NewStyle().Bold(true).Render(m.Name),
	}
	facts := []string{}
	if f := d.Details.Family; f != "" {
		facts = append(facts, "family "+f)
	}
	if p := d.Details.ParameterSize; p != "" {
		facts = append(facts, "param "+p)
	}
	if q := d.Details.QuantizationLevel; q != "" {
		facts = append(facts, "quant "+q)
	}
	if len(facts) > 0 {
		lines = append(lines, strings.Join(facts, " · "))
	}
	size := "unknown size"
	if m.SizeBytes > 0 {
		size = humanBytes(m.SizeBytes)
	}
	mod := "–"
	if !m.ModifiedAt.IsZero() {
		mod = m.ModifiedAt.Format("2006-01-02 15:04")
	}
	lines = append(lines, "size "+size+" · modified "+mod)
	if len(d.Capabilities) > 0 {
		lines = append(lines, "caps "+strings.Join(d.Capabilities, ", "))
	}

	sections := [][]string{
		section("PARAMETERS", d.Parameters),
		section("TEMPLATE", d.Template),
		section("MODEL FILE", d.Modelfile),
		section("MODEL INFO", modelInfoLines(d.ModelInfo)),
		section("LICENSE", d.License),
	}
	lines = append(lines, "")
	for _, sec := range sections {
		lines = append(lines, sec...)
	}

	// Drop trailing empties but keep one blank between sections.
	var out []string
	for _, l := range lines {
		if l == "" && len(out) > 0 && out[len(out)-1] == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

func section(title, content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return append([]string{"── " + title}, strings.Split(content, "\n")...)
}

// modelInfoLines flattens model_info into stable sorted "key = value" lines.
func modelInfoLines(info map[string]any) string {
	keys := make([]string, 0, len(info))
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %v\n", k, info[k])
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// wrapLines word-wraps every line to at most width cells, splitting at the
// last whitespace and hard-breaking mid-word when a single word overflows.
// wrapLines wraps each line to width columns. Rows that already fit (judged
// by visible width, so styled rows with ANSI escapes are never re-split
// mid-sequence) pass through untouched; only genuinely long lines are
// wrapped at word boundaries.
func wrapLines(lines []string, width int) []string {
	if width < 1 {
		return lines
	}
	var out []string
	for _, line := range lines {
		if lipgloss.Width(line) <= width {
			out = append(out, line)
			continue
		}
		for len(line) > width {
			cut := strings.LastIndex(line[:width+1], " ")
			if cut <= 0 {
				cut = width
			}
			out = append(out, line[:cut])
			line = strings.TrimLeft(line[cut:], " ")
		}
		out = append(out, line)
	}
	return out
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func clampFloat(n, lo, hi float64) float64 {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// --- M4 live apply: re-theme + client swap --------------------------------

// applyTheme re-tints the Models tab (list chrome, selected-row accent,
// spinner) after a settings Theme change or live preview.
func (v ModelsView) applyTheme(dark bool, styles Styles) ModelsView {
	v.styles = styles
	v.list.Styles = list.DefaultStyles(dark)
	v.list.SetDelegate(modelsDelegate(styles, dark))
	v.spinner.Style = lipgloss.NewStyle().Foreground(v.styles.accent)
	return v
}

// ApplyClient points the tab at a new Ollama client after a settings save
// that changed host or token; the stale list/detail are dropped and reloaded
// from the new host via the returned non-blocking load cmd.
func (v ModelsView) ApplyClient(c *ollama.Client) (ModelsView, tea.Cmd) {
	v.client = c
	v.models = nil
	v.detail = nil
	v.detailName = ""
	v.listErr = ""
	v.notice = ""
	v.loading = true
	v.list.SetItems(nil)
	return v, v.loadCmd()
}
