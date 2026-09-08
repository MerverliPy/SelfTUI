package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

	// clientGen counts client replacements (ApplyClient). Every async
	// list/show result stamps the generation it was issued under; a
	// completion whose generation differs from the current one belongs to an
	// obsolete host and is dropped (M-03).
	clientGen uint64

	// showReq/showTarget track the latest issued detail (show) request.
	// showTarget is the model that request asked for; showReq is its
	// monotonically increasing id. A completion is applied only when it
	// answers the latest request (its id matches) and comes from the current
	// client generation — a superseded or obsolete /api/show can never
	// repaint the pane under a newer selection or host (M-03).
	showReq    uint64
	showTarget string

	// showCancel cancels the in-flight /api/show request so an obsolete
	// detail fetch stops as soon as the selection changes or the client is
	// replaced (M-03). The tea loop owns it: commands only read it.
	showCancel context.CancelFunc

	// Transient feedback ("deleted qwen3:8b"…), cleared on the next reload.
	notice string

	// Delete flow (x → confirm → y/esc).
	confirmDelete bool
	deleteTarget  string
	deleting      bool
	deleteErr     string

	// Pull flow (p → name input → stream; esc cancels).
	inputMode      bool
	input          textinput.Model
	pulling        bool
	pullName       string
	pullErr        string
	pullStatus     string
	pullDigest     string
	pullTotal      int64
	pullDone       int64
	pullCh         chan tea.Msg  // activity channel (PLAN §8), owned by one pull
	pullStreamDone chan struct{} // closed by the pull producer when its goroutine exits (M-06)
	pullCancel     func()        // cancels the in-flight pull context

	// Reusable dialog widgets.
	spinner  spinner.Model
	progress progress.Model

	// spinnerPending is the M-09 deterministic test seam: it counts live
	// dialog-spinner chains — the one seed scheduled when pulling/deleting
	// opens, plus each FPS-paced successor scheduled after a consumed tick —
	// whose TickMsg has not yet arrived. It stays 0 or 1 in every steady
	// state and is reset when a busy state closes, so tests can assert that
	// progress events and unrelated input never create extra chains without
	// waiting on real spinner.FPS timers.
	spinnerPending int

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
	// List secondary rows and the picker summary render these remote values;
	// sanitize the joined display string (H-05).
	return sanitizeTerminalText(strings.Join(parts, " · "))
}

// --- messages -------------------------------------------------------------

type modelsLoadedMsg struct {
	gen  uint64 // client generation at issue; mismatched completions are dropped
	list []ollama.Model
}
type modelsLoadErrMsg struct {
	gen uint64
	err string
}
type modelsShowMsg struct {
	gen     uint64 // client generation at issue
	req     uint64 // request id (0 = hand-built/legacy result)
	name    string
	details ollama.Details
}
type modelsShowErrMsg struct {
	gen  uint64
	req  uint64
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

// modelsEventMsg is the single envelope the root App accepts for every
// asynchronous Models command result: the list/show/delete fetches
// (modelsLoadedMsg/modelsLoadErrMsg/modelsShowMsg/modelsShowErrMsg/
// modelsDeleteDoneMsg), the streamed pull progress + completion
// (modelsPullMsg/modelsPullDoneMsg), and the pull dialog's spinner ticks
// (spinner.TickMsg). App.Update has exactly one routing case per child and
// unwraps before delegating, so any payload that is produced is routed by
// construction — a newly added async result can no longer be dropped at the
// shell (the 2026-09-06 ToolConfirmMsg routing bug).
type modelsEventMsg struct{ msg tea.Msg }

// Init starts the first list fetch. Called once from the root App.
func (v ModelsView) Init() tea.Cmd {
	v.loading = true
	return v.loadCmd()
}

func (v ModelsView) loadCmd() tea.Cmd {
	gen := v.clientGen
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(v.ctx, 60*time.Second)
		defer cancel()
		models, err := v.client.List(ctx)
		if err != nil {
			return modelsEventMsg{msg: modelsLoadErrMsg{gen: gen, err: err.Error()}}
		}
		return modelsEventMsg{msg: modelsLoadedMsg{gen: gen, list: models}}
	}
}

// requestShow starts a /api/show fetch for name, replacing any in-flight
// detail request (whose context is canceled and whose late completion is
// dropped by request id). The returned command stamps the request id and the
// client generation, so a completion can never repaint the pane after the
// selection moved or the host changed (M-03). All state mutation happens on
// the tea update loop; the command only reads and reports.
func (v ModelsView) requestShow(name string) (ModelsView, tea.Cmd) {
	if v.showCancel != nil {
		v.showCancel()
	}
	ctx, cancel := context.WithTimeout(v.ctx, 60*time.Second)
	v.showReq++
	req, gen := v.showReq, v.clientGen
	v.showTarget = name
	v.showCancel = cancel
	v.loadingShow = true
	v.detailErr = ""
	return v, func() tea.Msg {
		defer cancel()
		details, err := v.client.Show(ctx, name)
		if err != nil {
			return modelsEventMsg{msg: modelsShowErrMsg{gen: gen, req: req, name: name, err: err.Error()}}
		}
		return modelsEventMsg{msg: modelsShowMsg{gen: gen, req: req, name: name, details: details}}
	}
}

// releaseShow clears the in-flight show state: nothing is pending, so the
// pane can no longer sit on a dangling "inspecting…" state after the latest
// request's result was dropped or superseded.
func (v ModelsView) releaseShow() ModelsView {
	v.loadingShow = false
	v.showTarget = ""
	if v.showCancel != nil {
		v.showCancel()
	}
	v.showCancel = nil
	return v
}

// acceptShow reports whether a delivered /api/show result may repaint the
// detail pane. It must (1) come from the current client generation, (2) not
// belong to a superseded request, and (3) not name a model the user no
// longer has selected (M-03). Real completions carry the request id they
// answer and must match both the latest id and the current selection. A
// hand-built result without an id (req 0 — legacy deliveries, routing/
// render tests) applies only when there is no selection to protect (empty
// list) or when it names the model of the pending request, so a stale
// hand-built result cannot race a real one.
func (v ModelsView) acceptShow(gen, req uint64, name string) bool {
	if gen != v.clientGen {
		return false
	}
	if req == 0 {
		if len(v.models) == 0 {
			return true
		}
		if name != v.modelName(v.list.Index()) {
			return false
		}
		return v.showTarget == "" || v.showTarget == name
	}
	if len(v.models) == 0 {
		return false
	}
	return req == v.showReq && name == v.modelName(v.list.Index())
}

func (v ModelsView) deleteCmd(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(v.ctx, 60*time.Second)
		defer cancel()
		if err := v.client.Delete(ctx, name); err != nil {
			return modelsEventMsg{msg: modelsDeleteDoneMsg{name: name, err: err.Error()}}
		}
		return modelsEventMsg{msg: modelsDeleteDoneMsg{name: name}}
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
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(v.ctx)

	v.pullCh = ch
	v.pullStreamDone = done
	v.pullCancel = cancel
	v.pulling = true
	v.pullName = name
	v.pullErr = ""
	v.pullStatus = "starting"
	v.pullDigest = ""
	v.pullTotal, v.pullDone = 0, 0

	go func() {
		defer close(done)
		defer close(ch)
		defer cancel()
		err := v.client.Pull(ctx, name, func(p ollama.PullProgress) {
			emitEvent(ctx, ch, modelsEventMsg{msg: modelsPullMsg{name: name, progress: p}})
		})
		if err != nil {
			emitEvent(ctx, ch, modelsEventMsg{msg: modelsPullDoneMsg{name: name, err: err.Error()}})
			return
		}
		emitEvent(ctx, ch, modelsEventMsg{msg: modelsPullDoneMsg{name: name}})
	}()

	return v, v.waitPullCmd()
}

// spinnerSeed starts the single dialog-spinner chain: one immediate TickMsg
// whose consumption makes spinner.Update hand back the first FPS-paced
// successor. Only the transition into pulling/deleting seeds a chain (M-09);
// the name-input and confirm states have no spinner.
func (v ModelsView) spinnerSeed() tea.Cmd {
	return func() tea.Msg { return modelsEventMsg{msg: v.spinner.Tick()} }
}

// spinnerResume keeps the one chain alive: it wraps the FPS-paced successor
// command spinner.Update returned for a consumed TickMsg so the next tick
// crosses the App shell inside modelsEventMsg (the envelope every async
// Models result uses). Update schedules it only while the busy state that
// owns the spinner is still active, so completion stops rescheduling (M-09).
func (v ModelsView) spinnerResume(successor tea.Cmd) tea.Cmd {
	return func() tea.Msg { return modelsEventMsg{msg: successor()} }
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
	spinnerBusy := v.deleting || v.pulling
	switch msg := msg.(type) {
	case modelsEventMsg:
		// The App shell normally unwraps the envelope before delegating; when
		// the view runs standalone (or a test drives its commands directly)
		// the wrapped result comes back as-is, so unwrap and re-dispatch to
		// the same switch.
		return v.Update(msg.msg)

	case tea.WindowSizeMsg:
		v.w, v.h = msg.Width, msg.Height
		return v, nil

	case modelsLoadedMsg:
		// A list from an obsolete client generation must not replace the
		// current host's state (M-03).
		if msg.gen != v.clientGen {
			return v, nil
		}
		v, cmd = v.onLoaded(msg.list)
		cmds = append(cmds, cmd)

	case modelsLoadErrMsg:
		if msg.gen != v.clientGen {
			return v, nil
		}
		v.loading = false
		v.listErr = sanitizeTerminalText(msg.err)
		v.notice = ""
		return v, nil

	case modelsShowMsg:
		// A stale detail result — from an obsolete host, a superseded
		// request, or for a model the user no longer has selected — must
		// never repaint the pane (M-03).
		if msg.gen != v.clientGen {
			return v, nil
		}
		if !v.acceptShow(msg.gen, msg.req, msg.name) {
			// The result answered the latest request but for a model that is
			// no longer selected (the selection or host moved on without a
			// replacement request): drop the payload and release the pending
			// show state so the pane never dangles on "inspecting…".
			if msg.req != 0 && msg.req == v.showReq {
				v = v.releaseShow()
			}
			return v, nil
		}
		d := sanitizeDetails(msg.details) // H-05: remote /api/show payload
		v.detail = &d
		v.detailName = sanitizeTerminalText(msg.name)
		v.detailErr = ""
		v.loadingShow = false
		v.showTarget = ""
		v.showCancel = nil
		v.scroll = 0
		return v, nil

	case modelsShowErrMsg:
		if msg.gen != v.clientGen {
			return v, nil
		}
		if !v.acceptShow(msg.gen, msg.req, msg.name) {
			if msg.req != 0 && msg.req == v.showReq {
				v = v.releaseShow()
			}
			return v, nil
		}
		v.detailErr = sanitizeTerminalText(msg.err)
		v.loadingShow = false
		v.showTarget = ""
		v.showCancel = nil
		return v, nil

	case spinner.TickMsg:
		v.spinner, cmd = v.spinner.Update(msg)
		if msg.ID == v.spinner.ID() && v.spinnerPending > 0 {
			v.spinnerPending-- // this scheduled tick arrived
		}
		if cmd != nil && (v.deleting || v.pulling) {
			v.spinnerPending++
			cmds = append(cmds, v.spinnerResume(cmd))
		}

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

	// Seed exactly one spinner chain on the transition into a busy state
	// (pulling / deleting). While a busy state is already open, progress,
	// keys, and every other message schedule nothing: the spinner's own
	// successor (above) is the only continuation, paced by spinner.FPS
	// instead of by event volume. Before M-09 every message re-seeded an
	// unpaced tick, so a long pull multiplied scheduled chains.
	if !spinnerBusy && (v.deleting || v.pulling) {
		v.spinnerPending++
		cmds = append(cmds, v.spinnerSeed())
	}
	if len(cmds) == 0 {
		return v, nil
	}
	return v, tea.Batch(cmds...)
}

// onLoaded replaces the model list. The bubbles list preserves (clamped) its
// cursor across SetItems, so after a reload the selection is the model now
// under that cursor — not necessarily the first item, and not necessarily the
// model the retained detail payload belongs to. The inspect pane must always
// describe the model under the cursor (its header is that model's name): a
// reload that drops or reorders the previously inspected model therefore has
// to drop the stale payload and re-inspect the new selection, or the pane
// paints one model's header over another's facts (P1-3).
func (v ModelsView) onLoaded(models []ollama.Model) (ModelsView, tea.Cmd) {
	models = sanitizeModelNames(models) // H-05: /api/tags names are remote
	v.models = models
	v.loading = false
	v.listErr = ""
	v.pullErr = ""
	v.notice = ""

	items := make([]list.Item, len(models))
	for i, m := range models {
		items[i] = modelsItem{m}
	}
	cmds := []tea.Cmd{v.list.SetItems(items)}
	// Sync our selection mirror to the list's actual (clamped) cursor: the
	// bubbles cursor survives SetItems, so mirroring it here keeps selIdx,
	// the highlight, and any later auto-inspect in agreement (P1-3).
	cur := v.list.Index()
	if len(models) > 0 {
		cur = clampInt(cur, 0, len(models)-1)
	} else {
		cur = 0
	}
	v.selIdx = cur

	// Reconcile the detail pane with the reloaded list. PaneVisible is the
	// geometry where the pane is drawn: side-by-side always, stacked after
	// enter.
	paneVisible := ForModels(v.w).SideBySide || v.showPane
	switch {
	case len(models) == 0:
		// Nothing to inspect: no pane can render, drop any stale payload so a
		// later reload cannot resurrect it under a fresh list.
		v.detail = nil
		v.detailName = ""
		v = v.releaseShow()

	case paneVisible:
		name := models[cur].Name
		if v.detailName != name || v.detail == nil {
			// The retained payload belongs to a model that is no longer under
			// the cursor (removed or reordered by the reload): drop it and
			// inspect the new selection so header and body always agree.
			// requestShow replaces any in-flight fetch for the pre-reload list
			// (cancel + id guard), so a superseded completion cannot repaint.
			v.detail = nil
			v.detailName = ""
			v = v.releaseShow()
			var sc tea.Cmd
			v, sc = v.requestShow(name)
			cmds = append(cmds, sc)
		}
		// detailName == name with a payload: the pane already shows the model
		// under the cursor; retain it (no redundant refetch on every reload).

	default:
		// Stacked layout with the pane closed: nothing is drawn, but a stale
		// payload for a model no longer listed would be resurrected by the
		// next enter on that name. Drop it when the inspected model vanished.
		if v.detailName != "" && !containsModel(models, v.detailName) {
			v.detail = nil
			v.detailName = ""
			v = v.releaseShow()
		}
	}
	return v, tea.Batch(cmds...)
}

// containsModel reports whether models lists a model with the given name.
func containsModel(models []ollama.Model, name string) bool {
	for _, m := range models {
		if m.Name == name {
			return true
		}
	}
	return false
}

// onDeleteDone finalizes a DELETE round-trip. Success closes the dialog and
// reloads the list; failure keeps the dialog open so the error sits right
// under the question for an immediate retry.
func (v ModelsView) onDeleteDone(m modelsDeleteDoneMsg) (ModelsView, tea.Cmd) {
	v.deleting = false
	v.spinnerPending = 0 // M-09: no live spinner chain once the busy state closes
	if m.err != "" {
		v.confirmDelete = true
		v.deleteErr = sanitizeTerminalText(m.err) // remote DELETE error body
		return v, nil
	}
	v.confirmDelete = false
	v.deleteTarget = ""
	v.notice = "deleted " + m.name
	if v.detailName == m.name || v.showTarget == m.name {
		v.detail = nil
		v.detailName = ""
		if v.showTarget == m.name {
			// A pending show for the deleted model is pointless; release it
			// so its late completion cannot resurrect the detail.
			v = v.releaseShow()
		}
	}
	return v, v.loadCmd()
}

// pullPill renders the background-job pill for a streaming pull (N4): the
// shell status row shows it while the pull runs, so progress stays visible
// from any tab. Empty when no pull is in flight. Without server-reported
// sizes (pullTotal == 0, e.g. before the first layer digest) the pill
// degrades to name + ellipsis — the pull dialog still carries the phase.
// The name is user-typed, but it is sanitized here like every other
// remote-adjacent display surface.
func (v ModelsView) pullPill() string {
	if !v.pulling {
		return ""
	}
	name := sanitizeTerminalText(v.pullName)
	if v.pullTotal > 0 {
		pct := int(clampFloat(float64(v.pullDone)/float64(v.pullTotal), 0, 1) * 100)
		return fmt.Sprintf("⇣ %s %d%%", name, pct)
	}
	return "⇣ " + name + "…"
}

// onPullProgress applies one streamed pull event and resubscribes the
// activity command. A new layer digest resets the progress numbers.
func (v ModelsView) onPullProgress(m modelsPullMsg) (ModelsView, tea.Cmd) {
	p := m.progress
	v.pullStatus = sanitizeTerminalText(p.Status) // remote pull status text
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
	v.spinnerPending = 0 // M-09: no live spinner chain once the busy state closes
	v.pullCh = nil
	v.pullStreamDone = nil
	v.pullCancel = nil
	if m.err != "" {
		v.pullErr = sanitizeTerminalText(m.err) // remote pull error body
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
	// M-03: a show that is still in flight for the previously selected model
	// must not suppress the fetch for the new selection — requestShow
	// replaces it (cancel + id guard), so the user always ends up looking at
	// the model they actually selected.
	if ForModels(v.w).SideBySide && len(v.models) > 0 {
		cur := v.list.Index()
		if cur != v.selIdx {
			v.selIdx = cur
			name := v.modelName(cur)
			alreadyShown := v.detailName == name && v.detail != nil && !v.loadingShow
			if !alreadyShown && name != v.showTarget {
				var sc tea.Cmd
				v, sc = v.requestShow(name)
				return v, tea.Batch(cmd, sc)
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
// (confirm / input / pull / deleting) replaces the whole body with a
// centered dialog.
func (v ModelsView) View() string {
	layout := ForModels(v.w)
	bodyH := v.h - 2 // tab bar + status bar

	switch {
	case v.deleting:
		// M-07: the in-flight delete keeps its own busy overlay from approval
		// until the DELETE completes; without this branch the view fell back
		// to the interactive list while keys were ignored (a frozen look).
		return v.renderOverlay(bodyH, "Deleting "+v.deleteTarget, v.deleteProgressLines())
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

// deleteProgressLines builds the in-flight delete dialog body (M-07): the
// spinner plus the model being removed. Esc deliberately does not cancel —
// the DELETE HTTP round-trip is not safely interruptible.
func (v ModelsView) deleteProgressLines() []string {
	return []string{v.spinner.View() + " deleting " + v.deleteTarget + "…"}
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
	return v.requestShow(name)
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
// Rows that already fit (judged by visible width, so styled rows with ANSI
// escapes are never re-split mid-sequence) pass through untouched; only
// genuinely long lines are wrapped.
//
// Wrapping is display-cell- and grapheme-aware (M-05): the overflow path
// delegates to the pinned Charm wrap primitive (github.com/charmbracelet/x/ansi
// — the same width model lipgloss.Width uses), which preserves ANSI sequences
// whole and measures wide CJK/emoji at 2 cells and ZWJ clusters atomically,
// then rejoinSplitMarks repairs the one cluster defect that primitive has
// with zero-width combining marks. Never slice by len/byte offsets here:
// bytes are not cells and a cut inside a rune or an escape sequence corrupts
// the terminal output.
func wrapLines(lines []string, width int) []string {
	if width < 1 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if lipgloss.Width(line) <= width {
			out = append(out, line)
			continue
		}
		wrapped := ansi.Wrap(line, width, "")
		out = append(out, rejoinSplitMarks(strings.Split(wrapped, "\n"))...)
	}
	return out
}

// rejoinSplitMarks repairs the one cluster defect of the Charm wrap primitive:
// ansi.Wrap measures combining marks and ZWJ as zero width, so when a word
// fills its line exactly the row break can land right after the base rune,
// stranding the mark at the head of the next row — visually detached from the
// glyph it modifies. Moving a stranded mark to the end of the previous row
// changes neither row's cell count (marks are zero-width) and never loses or
// reorders visible text. Style sequences at the row head (SelfTUI's own SGR
// opens; remote text is ANSI-sanitized before it reaches wrapLines) are
// skipped so a styled run starting a row is not mistaken for a stranded mark.
func rejoinSplitMarks(rows []string) []string {
	for i := 1; i < len(rows); i++ {
		for {
			head := ansiHeadLen(rows[i])
			if head >= len(rows[i]) {
				break // style-only row
			}
			r, size := utf8.DecodeRuneInString(rows[i][head:])
			if !isZeroWidthMark(r) {
				break
			}
			rows[i-1] += rows[i][head : head+size]
			rows[i] = rows[i][:head] + rows[i][head+size:]
		}
	}
	return rows
}

// isZeroWidthMark reports whether r is a combining mark or a zero-width joiner
// — zero-width runes that must stay glued to the rune they modify and must
// never open a wrapped row.
func isZeroWidthMark(r rune) bool {
	return r == '\u200d' ||
		unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

// ansiHeadLen returns the byte length of the leading ANSI escape sequences in
// s (CSI through its final byte, OSC through BEL/ST, two-byte ESC pairs), so
// callers can inspect the first visible rune of a styled row.
func ansiHeadLen(s string) int {
	i := 0
	for i < len(s) && s[i] == '\x1b' {
		if i+1 >= len(s) {
			return len(s)
		}
		switch s[i+1] {
		case '[': // CSI: through the final byte (0x40-0x7e).
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		case ']': // OSC: through BEL or ST (ESC \).
			j := i + 2
			for j < len(s) && s[j] != '\a' && !(s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\') {
				j++
			}
			if j < len(s) {
				j++ // consume BEL (or the ESC of an ST pair)
				if j-1 < len(s) && s[j-1] == '\x1b' && j < len(s) {
					j++ // consume the '\' of an ST pair
				}
			}
			i = j
		default: // Two-byte escape (ESC X).
			i += 2
		}
	}
	return i
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

func minInt(a, b int) int {
	if a < b {
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
	// A client replacement also ends an in-flight pull: its goroutine would
	// otherwise keep streaming the old host's progress into the new host's
	// view and surface the old host's result on completion. CancelFunc is
	// idempotent, so a later Esc double-cancel stays safe (D2).
	if v.pullCancel != nil {
		v.pullCancel()
	}
	v.client = c
	// A client replacement invalidates every in-flight result of the old
	// host: bump the generation and cancel the pending show so a stale
	// list/show completion is dropped on arrival (M-03).
	v.clientGen++
	v = v.releaseShow()
	v.detail = nil
	v.detailName = ""
	v.detailErr = ""
	v.models = nil
	v.listErr = ""
	v.notice = ""
	v.loading = true
	v.list.SetItems(nil)
	return v, v.loadCmd()
}
