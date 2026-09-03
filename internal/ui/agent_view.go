package ui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

// AgentView is the Agent tab (PLAN.md §7): a streaming plain-chat transcript
// rendered with glamour markdown (syntax-highlighted code blocks), a
// multi-line input, a model selector (m), graceful errors, and cancellation
// (esc). The tool loop lands in M3; M2's chat sends no tools, so any
// chat-capable model — tool-capable or not — works here: that is the
// explicit no-tool fallback (PLAN §6, never silent).
type AgentView struct {
	client *ollama.Client
	styles Styles
	dark   bool

	// Model selector state (loaded from /api/tags at init; r refreshes).
	models       []ollama.Model
	modelsErr    string
	loading      bool
	defaultModel string // from config; used when no selection exists yet
	model        string // selected model for the next send

	// Conversation state. history holds committed user/assistant messages;
	// turnModel records the model each committed message belongs to (the
	// assistant header shows it); render is the glamour-rendered block
	// (header + content) for each history message, cached per width.
	history   []ollama.ChatMessage
	turnModel []string
	render    []string

	streaming   bool         // generation in flight
	streamText  string       // in-flight assistant content (deltas appended)
	stopRequest bool         // esc asked to stop; treat stream end as a stop
	stopCancel  func()       // cancels the in-flight chat context
	chatCh      chan tea.Msg // activity channel (PLAN §8), one stream owner

	// Chat parameters (from config until M4).
	temperature float64
	topP        float64
	numCtx      int

	// Input + feedback.
	input   textarea.Model
	chatErr string
	notice  string

	// Transcript scroll: follow auto-tails the newest content while
	// streaming or after a new turn; u/d scroll away from the tail.
	scroll int
	follow bool

	// Model selector overlay.
	selectorOpen bool
	selIdx       int

	// glamour renderer, rebuilt when width changes (wrap is width-bound).
	tr      *glamour.TermRenderer
	renderW int

	chatChOnce bool
	w, h       int
}

// NewAgentView builds the Agent tab.
func NewAgentView(client *ollama.Client, styles Styles, theme, defaultModel string, agentCfg config.AgentConfig) AgentView {
	ta := textarea.New()
	ta.Prompt = "❯ "
	ta.Placeholder = "chat with the selected model…"
	ta.ShowLineNumbers = false // line numbers waste width on a phone
	ta.Focus()                 // the input is the Agent tab's primary surface
	return AgentView{
		client:       client,
		styles:       styles,
		dark:         theme != "light",
		loading:      true,
		defaultModel: defaultModel,
		temperature:  agentCfg.Temperature,
		topP:         agentCfg.TopP,
		numCtx:       agentCfg.NumCtx,
		follow:       true,
		input:        ta,
		selectorOpen: false,
		selIdx:       0,
		renderW:      -1,
	}
}

// --- messages -------------------------------------------------------------

type agentModelsLoadedMsg struct{ models []ollama.Model }
type agentModelsErrMsg struct{ err string }
type agentTokenMsg struct{ text string }
type agentDoneMsg struct{ err string }

// Init starts the model list fetch for the selector.
func (v AgentView) Init() tea.Cmd {
	return v.loadModelsCmd()
}

func (v AgentView) loadModelsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		models, err := v.client.List(ctx)
		if err != nil {
			return agentModelsErrMsg{err: err.Error()}
		}
		return agentModelsLoadedMsg{models: models}
	}
}

// startChat begins a streaming chat turn in a background goroutine.
// Tokens arrive as agentTokenMsg; the trailing result as one agentDoneMsg.
func (v AgentView) startChat() (AgentView, tea.Cmd) {
	ch := make(chan tea.Msg, 64)
	ctx, cancel := context.WithCancel(context.Background())

	v.chatCh = ch
	v.stopCancel = cancel
	v.streaming = true
	v.streamText = ""
	v.stopRequest = false
	v.chatErr = ""
	v.notice = ""
	v.follow = true

	model := v.model
	history := append([]ollama.ChatMessage(nil), v.history...)
	opts := &ollama.ChatOptions{Temperature: v.temperature, TopP: v.topP, NumCtx: v.numCtx}

	go func() {
		defer close(ch)
		defer cancel()
		req := ollama.ChatRequest{Model: model, Messages: history, Stream: true, Options: opts}
		err := v.client.Chat(ctx, req, func(delta string) {
			ch <- agentTokenMsg{text: delta}
		})
		if err != nil {
			ch <- agentDoneMsg{err: err.Error()}
			return
		}
		ch <- agentDoneMsg{}
	}()

	return v, v.waitChatCmd()
}

// waitChatCmd is the resubscribed activity command (PLAN §8; same pattern as
// the Models pull): it blocks until the chat goroutine posts its next message.
func (v AgentView) waitChatCmd() tea.Cmd {
	ch := v.chatCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg { return <-ch }
}

// ModalOpen reports whether the Agent tab is showing a modal. The root App
// uses it so the 1/2/3 tab-jump keys cannot steal from the selector.
func (v AgentView) ModalOpen() bool { return v.selectorOpen }

// --- update ---------------------------------------------------------------

func (v AgentView) Update(msg tea.Msg) (AgentView, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, 3)
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.w, v.h = msg.Width, msg.Height
		// Wrap width changed: force a renderer + cache rebuild at the new width.
		v.renderW = -1
		v.rebuildRenderer()
		v.rebuildRenderCache()
		return v, nil

	case agentModelsLoadedMsg:
		v, cmd = v.onModelsLoaded(msg.models)
		cmds = append(cmds, cmd)

	case agentModelsErrMsg:
		v.loading = false
		v.modelsErr = msg.err
		return v, nil

	case agentTokenMsg:
		if v.streaming {
			v.streamText += msg.text
			v.follow = true
		}
		return v, v.waitChatCmd()

	case agentDoneMsg:
		return v.onChatDone(msg)

	case tea.KeyMsg:
		v, cmd = v.handleKey(msg)
		cmds = append(cmds, cmd)
	}
	if len(cmds) == 0 {
		return v, nil
	}
	return v, tea.Batch(cmds...)
}

// onModelsLoaded picks the model to chat with: the config default when it is
// installed, otherwise the first entry (PLAN §5: "first /api/tags entry at
// runtime when empty"). An existing selection survives a refresh.
func (v AgentView) onModelsLoaded(models []ollama.Model) (AgentView, tea.Cmd) {
	v.models = models
	v.loading = false
	v.modelsErr = ""
	if len(models) == 0 {
		v.model = ""
		v.selIdx = 0
		return v, nil
	}

	keep := v.model
	v.model = ""
	for _, m := range models {
		if m.Name == keep {
			v.model = keep
			break
		}
	}
	if v.model == "" && v.defaultModel != "" {
		for _, m := range models {
			if m.Name == v.defaultModel {
				v.model = v.defaultModel
				break
			}
		}
	}
	if v.model == "" {
		v.model = models[0].Name
	}
	for i, m := range models {
		if m.Name == v.model {
			v.selIdx = i
			break
		}
	}
	return v, nil
}

// onChatDone finalizes a turn: commits the streamed text as an assistant
// message (when non-empty), then surfaces an error unless the user stopped
// the stream with esc (a stop is not an error).
func (v AgentView) onChatDone(m agentDoneMsg) (AgentView, tea.Cmd) {
	v.streaming = false
	v.stopCancel = nil
	v.chatCh = nil

	if v.streamText != "" {
		v.history = append(v.history, ollama.ChatMessage{Role: ollama.RoleAssistant, Content: v.streamText})
		v.turnModel = append(v.turnModel, v.model)
		v.render = append(v.render, v.renderBlock(v.assistantHeader(v.model), v.streamText))
		v.streamText = ""
	}

	switch {
	case v.stopRequest:
		v.notice = "stopped" // esc asked to stop, even if the stream just finished
	case m.err != "":
		v.chatErr = m.err
	default:
		v.notice = ""
	}
	v.stopRequest = false
	v.follow = true
	return v, nil
}

// --- keys -----------------------------------------------------------------

func (v AgentView) handleKey(msg tea.KeyMsg) (AgentView, tea.Cmd) {
	if _, isPress := msg.(tea.KeyPressMsg); !isPress {
		return v, nil
	}
	k := msg.Key()

	if v.selectorOpen {
		return v.selectorKey(k)
	}

	switch {
	case k.Code == tea.KeyEnter && k.Mod.Contains(tea.ModShift):
		// shift+enter inserts a newline: forward a plain enter to the textarea
		// (bubbles key matching is modifier-sensitive, so the real event would
		// fall through every binding).
		ta, cmd := v.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		v.input = ta
		return v, cmd
	case k.Code == tea.KeyEnter && !k.Mod.Contains(tea.ModShift):
		// Plain enter sends; the textarea never sees it.
		if v.streaming {
			return v, nil // one turn at a time; swallow silently
		}
		return v.sendInput()
	case k.Code == tea.KeyEsc:
		if v.streaming && v.stopCancel != nil {
			v.stopRequest = true
			v.stopCancel()
		}
		return v, nil
	case k.Text == "m" && v.input.Value() == "" && !v.streaming:
		// The letter commands (m/r/u/d) only fire while the input is empty,
		// so typing ordinary prose never triggers them (confirmed live: the
		// 'm' in "stop me" opened the selector — M2 lesson).
		return v.openSelector()
	case k.Text == "r" && v.input.Value() == "" && !v.streaming:
		v.loading = true
		v.modelsErr = ""
		return v, v.loadModelsCmd()
	case k.Text == "u" && v.input.Value() == "":
		v.scroll++
		v.follow = false
		v.clampScroll()
		return v, nil
	case k.Text == "d" && v.input.Value() == "":
		v.scroll--
		v.follow = false
		v.clampScroll()
		return v, nil
	}

	ta, cmd := v.input.Update(msg)
	v.input = ta
	return v, cmd
}

// sendInput appends the typed text as a user message and starts a stream.
func (v AgentView) sendInput() (AgentView, tea.Cmd) {
	text := strings.TrimSpace(v.input.Value())
	if text == "" {
		return v, nil
	}
	if v.model == "" {
		v.notice = "no model selected — press m or pull one in the Models tab"
		return v, nil
	}
	ti := v.input
	ti.Reset()
	v.input = ti

	v.history = append(v.history, ollama.ChatMessage{Role: ollama.RoleUser, Content: text})
	v.turnModel = append(v.turnModel, v.model)
	v.render = append(v.render, v.renderBlock(v.userHeader(), text))
	return v.startChat()
}

// --- model selector -------------------------------------------------------

func (v AgentView) openSelector() (AgentView, tea.Cmd) {
	if len(v.models) == 0 {
		v.notice = "no models — pull one from the Models tab (p)"
		return v, nil
	}
	v.selectorOpen = true
	return v, nil
}

func (v AgentView) selectorKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		v.selectorOpen = false
	case k.Code == tea.KeyEnter:
		if v.selIdx >= 0 && v.selIdx < len(v.models) {
			v.model = v.models[v.selIdx].Name
			v.notice = "model " + v.model
		}
		v.selectorOpen = false
	case k.Text == "j" || k.Code == tea.KeyDown:
		if v.selIdx < len(v.models)-1 {
			v.selIdx++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if v.selIdx > 0 {
			v.selIdx--
		}
	}
	return v, nil
}

// --- rendering ------------------------------------------------------------

// assistantHeader is the "who replied" line shown above each assistant block.
func (v AgentView) assistantHeader(model string) string {
	if model == "" {
		model = "assistant"
	}
	return v.styles.agentAccent().Render("◈ " + model)
}

// userHeader is the marker above each user block.
func (v AgentView) userHeader() string {
	return v.styles.agentAccent().Render("❯ you")
}

// ensureRenderer builds the glamour renderer when it is missing or the width
// changed. renderBlock calls it lazily so streaming renders never fail.
func (v *AgentView) ensureRenderer(width int) {
	width = maxInt(width, 1)
	if v.tr != nil && v.renderW == width {
		return
	}
	style := "dark"
	if !v.dark {
		style = "light"
	}
	tr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		v.tr = nil
		return
	}
	v.tr = tr
	v.renderW = width
}

// rebuildRenderer (re)builds the renderer for the current width after a
// geometry change. A failed build keeps the previous renderer; View falls
// back to raw text so output is never silent.
func (v *AgentView) rebuildRenderer() {
	v.ensureRenderer(maxInt(v.w-2, 1))
}

// rebuildRenderCache re-renders every committed block at the current width.
// Only called on geometry changes (rare); renderBlock uses the shared cache
// every frame otherwise.
func (v *AgentView) rebuildRenderCache() {
	if v.renderW != maxInt(v.w-2, 1) {
		return // renderer is stale; ensureRenderer on next renderBlock fixes it
	}
	for i := range v.history {
		v.render[i] = v.renderBlock(v.headerFor(i), v.history[i].Content)
	}
}

// headerFor returns the role header for a committed message index.
func (v AgentView) headerFor(i int) string {
	if i < len(v.turnModel) && v.turnModel[i] != "" && v.history[i].Role == ollama.RoleAssistant {
		return v.assistantHeader(v.turnModel[i])
	}
	if v.history[i].Role == ollama.RoleAssistant {
		return v.assistantHeader("")
	}
	return v.userHeader()
}

// renderBlock renders header + markdown content as one block. It never
// returns empty text for non-empty input: glamour failures fall back to the
// raw markdown so chat output is never silent (PLAN §6).
func (v AgentView) renderBlock(header, md string) string {
	if md == "" {
		if header == "" {
			return ""
		}
		return header
	}
	v.ensureRenderer(maxInt(v.w-2, 1))
	if v.tr != nil {
		if out, err := v.tr.RenderBytes([]byte(md)); err == nil {
			return header + "\n" + strings.TrimSuffix(string(out), "\n")
		}
	}
	return header + "\n" + md
}

// chatLines assembles the full rendered transcript as individual display
// lines: the cached history blocks (header + markdown content) plus the live
// streaming block. Block content is split on newlines so scroll/window
// arithmetic counts real rows, not multi-line entries.
func (v AgentView) chatLines() []string {
	var lines []string
	for i, m := range v.history {
		block := m.Content
		if i < len(v.render) && v.render[i] != "" {
			block = v.render[i]
		}
		if block == "" {
			continue
		}
		lines = append(lines, strings.Split(block, "\n")...)
		lines = append(lines, "") // separator after each message
	}
	if v.streaming || v.streamText != "" {
		lines = append(lines, strings.Split(v.renderBlock(v.assistantHeader(v.model), v.streamText), "\n")...)
		lines = append(lines, "")
	}
	// Drop trailing blanks.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// View renders transcript + hint + input per the current geometry. A modal
// (model selector) replaces the body with a centered overlay.
func (v AgentView) View() string {
	bodyH := maxInt(v.h-2, 1)

	if v.selectorOpen {
		return v.renderSelectorOverlay(bodyH)
	}

	if len(v.models) == 0 {
		return v.fullSizePane(bodyH)
	}

	inputOuter, hintH := 4, 1
	chatH := maxInt(bodyH-inputOuter-hintH, 1)

	chatPane := v.renderChatPane(chatH)
	hint := v.hintLine()
	input := v.renderInput()

	return chatPane + "\n" + hint + "\n" + input
}

// renderChatPane windows the transcript into the scroll region.
func (v AgentView) renderChatPane(h int) string {
	if h < 2 {
		return ""
	}
	lines := v.chatLines()
	contentH := h - 2
	if v.follow {
		v.scroll = 0 // anchored to the tail while auto-following
	}
	v.scroll = clampInt(v.scroll, 0, maxInt(0, len(lines)-contentH))

	// Window honors the scroll offset: scroll 0 shows the tail; scrolling
	// up shifts the window toward the head.
	end := len(lines) - v.scroll
	start := maxInt(0, end-contentH)
	window := lines[start:end]
	pane := v.styles.Pane.Width(v.w).Height(h)
	return pane.Render(lipgloss.JoinVertical(lipgloss.Left, window...))
}

// renderInput draws the bordered textarea at the bottom of the tab.
func (v AgentView) renderInput() string {
	w := maxInt(v.w-2, 10)
	ta := v.input
	ta.SetWidth(w)
	ta.SetHeight(2)
	v.input = ta
	return v.styles.Pane.Width(v.w).Height(4).Render(v.input.View())
}

// hintLine is the one-row strip between transcript and input: an error, the
// streaming state, a transient notice, or the legend.
func (v AgentView) hintLine() string {
	switch {
	case v.chatErr != "":
		return v.styles.Error.Render("⚠ " + v.chatErr + " — press enter to retry")
	case v.streaming:
		model := v.model
		if model == "" {
			model = "?"
		}
		return v.styles.Placeholder.Render("⏳ " + model + " · esc stop")
	case v.notice != "":
		return v.styles.Placeholder.Render(v.notice)
	}
	// While the user is composing, the letter commands (m/r/u/d) are off;
	// the legend only advertises them for an empty input.
	if v.input.Value() != "" {
		return v.styles.Placeholder.Render("enter send · shift+enter newline")
	}
	return v.styles.Placeholder.Render("enter send · shift+enter newline · m model · r refresh · esc stop")
}

// fullSizePane renders the loading / error / empty model-list states.
func (v AgentView) fullSizePane(bodyH int) string {
	pane := v.styles.Pane.Width(v.w).Height(maxInt(bodyH-2, 1))
	var content string
	switch {
	case v.loading:
		content = v.styles.Placeholder.Render("⏳ loading chat models…")
	case v.modelsErr != "":
		content = v.styles.Error.Render("⚠ "+v.modelsErr) + "\n" +
			v.styles.Placeholder.Render("press r to retry")
	default:
		content = v.styles.Placeholder.Render("no models installed") + "\n" +
			v.styles.Placeholder.Render("pull one from the Models tab (p), then press r")
	}
	return pane.Render(content)
}

// renderSelectorOverlay centers the model picker over the body. The list is
// windowed around the selection so long model lists stay readable on a phone.
func (v AgentView) renderSelectorOverlay(bodyH int) string {
	innerW := maxInt(v.w-8, 20)
	maxRows := maxInt(bodyH-8, 3)
	start := clampInt(v.selIdx-maxRows/2, 0, maxInt(0, len(v.models)-maxRows))
	end := start + maxRows
	if end > len(v.models) {
		end = len(v.models)
	}

	var lines []string
	for i := start; i < end; i++ {
		m := v.models[i]
		marker := "  "
		label := m.Name
		if i == v.selIdx {
			marker = "❯ "
			label = lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(m.Name)
		}
		row := marker + label
		if summary := modelSummary(m); summary != "" && lipgloss.Width(row)+1+lipgloss.Width(summary) <= innerW {
			row += "  " + v.styles.Placeholder.Render(summary)
		}
		lines = append(lines, row)
	}

	return v.renderOverlayTitle(bodyH, "Model", lines)
}

// renderOverlayTitle centers a bordered dialog over the whole Agent body.
func (v AgentView) renderOverlayTitle(bodyH int, title string, lines []string) string {
	innerW := maxInt(v.w-6, 16)
	wrapped := wrapLines(lines, innerW)
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

// clampScroll bounds the stored scroll offset by the currently visible window.
func (v *AgentView) clampScroll() {
	h := maxInt(v.h-2, 1) - 4 - 1 // same accounting as View()
	lines := len(v.chatLines())
	maxScroll := maxInt(0, lines-maxInt(h-2, 1))
	if v.scroll > maxScroll {
		v.scroll = maxScroll
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
}

// agentAccent returns the accent style applied to role headers.
func (s Styles) agentAccent() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(s.accent)
}
