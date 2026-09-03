package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/ollama"
)

// ModelsView is the Models tab (PLAN.md §7): a selectable model list plus an
// inspect pane fed by POST /api/show. It is M1a's deliverable — list and
// inspect models live. Layout (side-by-side vs stacked) comes from the shared
// breakpoint system; theme comes from the shared Styles.
type ModelsView struct {
	client *ollama.Client
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

	selIdx int

	list list.Model
	w, h int
}

// NewModelsView builds the Models tab.
func NewModelsView(client *ollama.Client, styles Styles, theme string) ModelsView {
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

	return ModelsView{
		client: client,
		styles: styles,
		list:   l,
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
	accent := lipgloss.Color("63")
	if !dark {
		accent = lipgloss.Color("57")
	}
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

// Init starts the first list fetch. Called once from the root App.
func (v ModelsView) Init() tea.Cmd {
	v.loading = true
	return v.loadCmd()
}

func (v ModelsView) loadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		details, err := v.client.Show(ctx, name)
		if err != nil {
			return modelsShowErrMsg{name: name, err: err.Error()}
		}
		return modelsShowMsg{name: name, details: details}
	}
}

// --- update ---------------------------------------------------------------

// Update handles messages aimed at the Models tab. The root App forwards
// keys only when this tab is active; async results are forwarded always.
func (v ModelsView) Update(msg tea.Msg) (ModelsView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.w, v.h = msg.Width, msg.Height

	case modelsLoadedMsg:
		v, cmd := v.onLoaded(msg.list)
		return v, cmd

	case modelsLoadErrMsg:
		v.loading = false
		v.listErr = msg.err
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

	case tea.KeyMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

// onLoaded replaces the model list. On wide/medium-split layouts the inspect
// pane is always visible, so the first model is inspected immediately.
func (v ModelsView) onLoaded(models []ollama.Model) (ModelsView, tea.Cmd) {
	v.models = models
	v.loading = false
	v.listErr = ""
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

// handleKey is the Models-tab key dispatcher. List navigation (j/k/arrows,
// g/G, pgup/pgdn, …) is delegated to the bubbles list; the keys below are
// Models-view specific.
func (v ModelsView) handleKey(msg tea.KeyMsg) (ModelsView, tea.Cmd) {
	if _, isPress := msg.(tea.KeyPressMsg); !isPress {
		return v, nil
	}
	k := msg.Key()

	switch {
	case k.Code == tea.KeyEnter:
		return v.inspectSelected()

	case k.Code == tea.KeyEsc:
		if !ForModels(v.w).SideBySide && v.showPane {
			v.showPane = false
		}
		return v, nil

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

// --- view -----------------------------------------------------------------

// View renders list + inspect per the breakpoint layout. Border wrapped, so
// pane dimensions shrink by two cells for the inner content.
func (v ModelsView) View() string {
	layout := ForModels(v.w)
	bodyH := v.h - 2 // tab bar + status bar

	if len(v.models) == 0 {
		return v.fullSizePane(layout, bodyH)
	}

	listW, listH := v.w, bodyH
	detailW, detailH := 0, 0
	if layout.SideBySide {
		listW, detailW = layout.ListWidth, layout.DetailWidth(v.w)
	} else if v.showPane {
		listH = bodyH * 40 / 100
		detailH = bodyH - listH
	}

	listPane := v.renderList(listW-2, listH-2)

	if !layout.SideBySide && !v.showPane {
		if v.listErr != "" {
			return listPane + "\n" + v.styles.Error.Render("⚠ "+v.listErr+" — press r to retry")
		}
		return listPane
	}

	detailW, detailH = v.detailPaneDims()
	detail := v.renderDetail(detailW, detailH)
	if layout.SideBySide {
		return lipgloss.JoinHorizontal(lipgloss.Top, listPane, detail)
	}
	return listPane + "\n" + detail
}

// fullSizePane renders the loading / error / empty states that fill the body.
func (v ModelsView) fullSizePane(layout ModelsLayout, bodyH int) string {
	pane := v.styles.Pane.Width(v.w - 2).Height(bodyH - 2)
	var content string
	switch {
	case v.loading:
		content = v.styles.Placeholder.Render("⏳ loading models…")
	case v.listErr != "":
		content = v.styles.Error.Render("⚠ "+v.listErr) + "\n" +
			v.styles.Placeholder.Render("press r to retry")
	case layout.SideBySide:
		content = v.styles.Placeholder.Render("no models installed")
	default:
		content = v.styles.Placeholder.Render("no models installed — pull will land in M1b") + "\n" +
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
// POST /api/show. Returns "" when the geometry has no room.
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
	v.scroll = clampInt(v.scroll, 0, maxInt(0, len(lines)-h))
	window := lines[v.scroll:]
	if len(window) > h {
		window = window[:h]
	}
	return body.Render(lipgloss.JoinVertical(lipgloss.Left, window...))
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
func wrapLines(lines []string, width int) []string {
	if width < 1 {
		return lines
	}
	var out []string
	for _, line := range lines {
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
