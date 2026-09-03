package ui

// M7 tests: UX polish — composer + slash commands (A), transcript feel (B),
// context meter + filterable picker (C). See PLAN.md §10 (M7).

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/agent"
	"selftui/internal/config"
	"selftui/internal/ollama"
)

// --- Package A: composer + slash commands ---------------------------------

// seedTranscript gives an AgentView a committed conversation without any
// server traffic (used to test /clear and paging). The body is a fenced code
// block so every line survives glamour (plain lines collapse into paragraphs).
func seedTranscript(v *AgentView, lines int) {
	var body strings.Builder
	body.WriteString("```\n")
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&body, "line %03d\n", i)
	}
	body.WriteString("```\n")
	v.history = append(v.history,
		ollama.ChatMessage{Role: ollama.RoleUser, Content: "make a long answer"},
		ollama.ChatMessage{Role: ollama.RoleAssistant, Content: body.String()},
	)
	v.turnModel = []string{"qwen3:8b", "qwen3:8b"}
	v.render = []string{
		v.renderBlock(v.userHeader(), "make a long answer"),
		v.renderBlock(v.assistantHeader("qwen3:8b"), body.String()),
	}
}

func TestAgentViewSlashMenuFiltersLive(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// "/" alone shows every command.
	typeText(t, &v, "/")
	if !v.slashMenu() {
		t.Fatal("'/' should open the command menu")
	}
	out := stripANSI(v.View())
	for _, want := range []string{"/clear", "/model", "/theme", "/help", "/refresh"} {
		if !strings.Contains(out, want) {
			t.Errorf("menu missing %q:\n%s", want, out)
		}
	}

	// Typing narrows it: "/cl" matches only clear.
	typeText(t, &v, "cl")
	out = stripANSI(v.View())
	if !strings.Contains(out, "/clear") || strings.Contains(out, "/theme") {
		t.Errorf("filter should leave only clear:\n%s", out)
	}

	// A draft matching no command is ordinary prose (no menu).
	for _, ch := range "ear all of it" {
		v, _ = v.Update(tea.KeyPressMsg{Text: string(ch)})
	}
	if v.slashMenu() {
		t.Error("unmatched slash draft must not keep the menu open")
	}

	// esc while a slash draft is showing clears the whole draft.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	typeText(t, &v, "/clear")
	if !v.slashMenu() {
		t.Fatal("'/clear' should keep the menu open")
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := v.input.Value(); got != "" {
		t.Errorf("esc should drop the draft, input = %q", got)
	}
}

func TestAgentViewSlashModelCommandOpensPicker(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/model")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.selectorOpen {
		t.Fatal("'/model' should open the picker")
	}
	if got := v.input.Value(); got != "" {
		t.Errorf("command should consume the draft, input = %q", got)
	}
}

func TestAgentViewSlashClearAsksThenWipes(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	seedTranscript(&v, 3)

	typeText(t, &v, "/clear")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.clearConfirm {
		t.Fatal("'/clear' should ask for confirmation first")
	}
	if !strings.Contains(stripANSI(v.View()), "Clear the conversation") {
		t.Fatalf("confirmation dialog missing:\n%s", stripANSI(v.View()))
	}

	// n/esc cancels: history stays.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.clearConfirm || len(v.history) != 2 {
		t.Fatalf("esc should cancel the clear (confirm=%v history=%d)", v.clearConfirm, len(v.history))
	}
	if v.notice != "clear cancelled" {
		t.Errorf("notice = %q, want clear cancelled", v.notice)
	}

	// y wipes everything and resets the budget state.
	typeText(t, &v, "/clear")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	v, _ = v.Update(tea.KeyPressMsg{Text: "y"})
	if v.clearConfirm || len(v.history) != 0 || len(v.turnMeta) != 0 {
		t.Fatalf("y should wipe the conversation (confirm=%v history=%d)", v.clearConfirm, len(v.history))
	}
	if v.notice != "conversation cleared" {
		t.Errorf("notice = %q, want conversation cleared", v.notice)
	}
}

func TestAgentViewSlashClearEmptyIsNotice(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/clear")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.clearConfirm {
		t.Error("empty conversation must not open the confirm dialog")
	}
	if v.notice != "nothing to clear" {
		t.Errorf("notice = %q, want nothing to clear", v.notice)
	}
}

func TestAgentViewSlashHelpOverlay(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/help")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.helpOpen || !v.ModalOpen() {
		t.Fatal("'/help' should open the reference overlay")
	}
	out := stripANSI(v.View())
	for _, want := range []string{"slash commands", "/theme", "ctrl+p", "pgup/pgdn"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.helpOpen || v.ModalOpen() {
		t.Error("esc should close the help overlay")
	}
}

func TestAgentViewSlashThemeEmitsAppMessage(t *testing.T) {
	v := testAgent(t, nil)
	v.dark = true
	typeText(t, &v, "/theme")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("'/theme' should emit an app theme message")
	}
	msg := cmd()
	if m, ok := msg.(agentThemeMsg); !ok || m.theme != "light" {
		t.Errorf("msg = %#v, want agentThemeMsg{light} from a dark session", msg)
	}
}

func TestEscClearsDraftWhenIdle(t *testing.T) {
	v := testAgent(t, nil)
	typeText(t, &v, "a drafted prompt")
	if v.input.Value() == "" {
		t.Fatal("setup: draft should be present")
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.input.Value() != "" {
		t.Errorf("esc should clear the drafted prompt, input = %q", v.input.Value())
	}
	// A second esc on an empty input is a no-op.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.input.Value() != "" {
		t.Errorf("esc on empty input must not do anything, input = %q", v.input.Value())
	}
}

func TestPaletteOpensFiltersAndCloses(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})

	// ctrl+p from the Models tab.
	m = updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if !m.paletteOpen {
		t.Fatal("ctrl+p should open the palette")
	}
	if m.tab != 0 {
		t.Fatalf("palette should open over the current tab, tab=%d", m.tab)
	}
	out := stripANSI(m.View().Content)
	for _, want := range []string{"Command palette", "Clear conversation", "Toggle theme"} {
		if !strings.Contains(out, want) {
			t.Errorf("palette missing %q:\n%s", want, out)
		}
	}

	// Filtering narrows the list ("toggle" matches one command).
	for _, r := range "toggle" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	out = stripANSI(m.View().Content)
	if !strings.Contains(out, "Toggle theme") || strings.Contains(out, "Change model") {
		t.Errorf("palette filter failed:\n%s", out)
	}

	// esc closes and restores the tab keys.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.paletteOpen {
		t.Fatal("esc should close the palette")
	}
	m = updateTab(t, m, tea.KeyPressMsg{Text: "3"})
	if m.tab != 2 {
		t.Errorf("tab after esc = %d, want 2 (digit keys restored)", m.tab)
	}
}

func TestPaletteOwnsKeysWhileOpen(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})
	m = updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})

	// Tab and digit jumps must not escape the palette; the digit feeds the
	// filter instead.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "3"})
	if !m.paletteOpen || m.tab != 0 {
		t.Fatalf("palette lost keys: open=%v tab=%d", m.paletteOpen, m.tab)
	}
}

func TestPaletteChangeModelGoesToAgentPicker(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})
	m = updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})

	// Filter to the one item whose description mentions the picker.
	for _, r := range "change model" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.paletteOpen {
		t.Fatal("enter should close the palette")
	}
	if m.tab != agentTab {
		t.Fatalf("tab = %d, want Agent after Change model", m.tab)
	}
	if !m.agent.selectorOpen {
		t.Error("Change model should open the Agent picker")
	}
}

func TestPaletteThemeToggleShowsToastAndClears(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	for _, r := range "toggle theme" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.curTheme != "light" {
		t.Errorf("curTheme = %q, want light after the dark→light toggle", m.curTheme)
	}
	if m.note == "" || !strings.Contains(m.note, "light") {
		t.Errorf("note = %q, want a theme toast", m.note)
	}
	if out := stripANSI(m.View().Content); !strings.Contains(out, "theme light") {
		t.Errorf("toast not shown in the status bar:\n%s", out)
	}
	// The next keypress dismisses the toast.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.note != "" {
		t.Errorf("note = %q, want cleared on the next key", m.note)
	}
}

func TestAgentThemeMsgAppliesShellWide(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultModel = "qwen3:8b"
	m := New(&cfg, NewStyles("dark"), ollama.New(cfg.Host, cfg.AuthToken))
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"}) // Agent tab
	if m.curTheme != "dark" {
		t.Fatalf("curTheme = %q, want dark at boot", m.curTheme)
	}
	m = updateTab(t, m, agentThemeMsg{theme: "light"})
	if m.curTheme != "light" || m.agent.dark {
		t.Errorf("theme msg not applied shell-wide: curTheme=%q agent.dark=%v", m.curTheme, m.agent.dark)
	}
	if !strings.Contains(m.note, "light") {
		t.Errorf("note = %q, want a theme toast", m.note)
	}
}

// --- Package B: transcript feel -------------------------------------------

func TestStreamingCaretAppearsAndDisappears(t *testing.T) {
	// A server that streams one delta then holds until the client stops it —
	// deterministic window to assert the caret mid-stream.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"draft"},"done":false}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	v := testAgent(t, ollama.New(srv.URL, ""))
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "hi")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	select {
	case msg := <-v.chatCh:
		var cmd tea.Cmd
		v, cmd = v.Update(msg)
		if cmd == nil {
			t.Fatal("expected a resubscribed command")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first token never arrived")
	}
	if !v.streaming {
		t.Fatal("should be streaming")
	}
	if out := stripANSI(v.View()); !strings.Contains(out, "▍") {
		t.Errorf("caret missing while streaming:\n%s", out)
	}

	// Stop the stream: the partial text commits and the caret disappears.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	drainChat(t, &v)
	if out := stripANSI(v.View()); strings.Contains(out, "▍") {
		t.Errorf("caret must disappear at rest:\n%s", out)
	}
}

// doneEvent builds a streamed final event with an explicit done_reason.
func doneEvent(content, reason string) string {
	return fmt.Sprintf(`{"message":{"role":"assistant","content":%q},"done":true,"done_reason":%q}`+"\n", content, reason)
}

func TestTurnFooterShowsElapsedAndStopReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			w.Header().Set("Content-Type", "application/x-ndjson")
			io.WriteString(w, doneEvent("final answer", "stop"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	v := testAgent(t, ollama.New(srv.URL, ""))
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)

	// turnMeta parallels history (a user placeholder at 0, the footer at 1).
	if len(v.turnMeta) != 2 {
		t.Fatalf("turnMeta = %v, want [placeholder footer]", v.turnMeta)
	}
	if meta := v.turnMeta[1]; !strings.Contains(meta, "s · stop") {
		t.Errorf("footer = %q, want elapsed + stop reason", meta)
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "· stop") {
		t.Errorf("footer not rendered in transcript:\n%s", out)
	}
}

func TestStopReasonLength(t *testing.T) {
	f := turnFooter(time.Now(), "length", false)
	if !strings.Contains(f, "· length") {
		t.Errorf("footer = %q, want length reason", f)
	}
	if f2 := turnFooter(time.Now(), "", true); !strings.Contains(f2, "· stopped") {
		t.Errorf("stopped footer = %q, want stopped", f2)
	}
	if f3 := turnFooter(time.Time{}, "stop", false); f3 != "" {
		t.Errorf("footer without a start = %q, want empty", f3)
	}
}

func TestPageScrollAndFollowToggle(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	seedTranscript(&v, 220) // long enough to page several times
	v.follow = true

	// pgup pages away from the tail and disables follow.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if v.follow {
		t.Error("pgup should leave auto-follow")
	}
	if v.scroll != v.pageHeight() {
		t.Errorf("scroll = %d, want %d after one page up", v.scroll, v.pageHeight())
	}

	// f snaps back to the tail.
	v, _ = v.Update(tea.KeyPressMsg{Text: "f"})
	if !v.follow || v.scroll != 0 {
		t.Errorf("f should re-engage follow (follow=%v scroll=%d)", v.follow, v.scroll)
	}

	// pgdn back to the tail re-engages follow.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if !v.follow {
		t.Error("pgdn back to the tail should re-engage follow")
	}
	if v.scroll != 0 {
		t.Errorf("scroll = %d after pgdn to tail, want 0", v.scroll)
	}

	// u/d still scroll line by line (M5 behavior) with d returning to follow.
	v, _ = v.Update(tea.KeyPressMsg{Text: "u"})
	if v.follow || v.scroll != 1 {
		t.Errorf("u should scroll one line away (follow=%v scroll=%d)", v.follow, v.scroll)
	}
	v, _ = v.Update(tea.KeyPressMsg{Text: "d"})
	if !v.follow || v.scroll != 0 {
		t.Errorf("d to the tail should follow (follow=%v scroll=%d)", v.follow, v.scroll)
	}
}

// --- Package C: context meter + picker upgrade -----------------------------

func TestContextMeterShowsWhileComposing(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	out := stripANSI(v.View())
	if strings.Contains(out, "ctx ") {
		t.Errorf("no meter expected on an idle empty conversation:\n%s", out)
	}
	typeText(t, &v, "hello model")
	out = stripANSI(v.View())
	if !strings.Contains(out, "ctx ") || !strings.Contains(out, "%") {
		t.Errorf("meter missing while composing:\n%s", out)
	}
}

func TestTruncationMarkerAndCtxFull(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.numCtx = 80 // tiny budget: limit = 60 tokens
	big := strings.Repeat("a very long line of prose that eats the budget fast ", 6)

	v.history = append(v.history, ollama.ChatMessage{Role: ollama.RoleUser, Content: big})
	v.turnModel = append(v.turnModel, v.model)
	v.render = append(v.render, v.renderBlock(v.userHeader(), big))
	v.checkContextBudget()

	if !v.truncated {
		t.Fatal("an over-budget send should flag the transcript as truncated")
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, agent.TruncationNotice) {
		t.Errorf("truncation marker not visible in the transcript:\n%s", out)
	}
	if !strings.Contains(out, "ctx full") {
		t.Errorf("idle hint should warn ctx full:\n%s", out)
	}
}

func TestModelPickerFiltersAsYouType(t *testing.T) {
	v := testAgent(t, nil)
	v.defaultModel = "gemma3:12b"
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v, cmd := v.Update(tea.KeyPressMsg{Text: "m"})
	if cmd != nil || !v.selectorOpen {
		t.Fatal("m should open the picker")
	}

	// The default model is starred.
	out := stripANSI(v.View())
	if !strings.Contains(out, "★") {
		t.Errorf("default model should be starred:\n%s", out)
	}

	// Typing filters the list to the gemma model.
	for _, r := range "gemma" {
		v, _ = v.Update(tea.KeyPressMsg{Text: string(r)})
	}
	if got := v.filteredModels(); len(got) != 1 || got[0].Name != "gemma3:12b" {
		t.Fatalf("filtered = %+v, want only gemma3:12b", got)
	}
	out = stripANSI(v.View())
	if !strings.Contains(out, "gemma3:12b") || strings.Contains(out, "qwen3:8b") {
		t.Errorf("filtered picker shows the wrong rows:\n%s", out)
	}

	// Backspace narrows to "gemm" and still matches; an impossible filter
	// shows the no-match row; repairing the filter by editing and entering
	// picks the filtered model.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	for _, r := range "3:8b" {
		v, _ = v.Update(tea.KeyPressMsg{Text: string(r)})
	}
	if len(v.filteredModels()) != 0 {
		t.Fatalf("expected no match for gemma3:8b, got %+v", v.filteredModels())
	}
	out = stripANSI(v.View())
	if !strings.Contains(out, "no model matches") {
		t.Errorf("no-match row missing:\n%s", out)
	}
	for len(v.selFilter) > 0 {
		v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	for _, r := range "gemma3:12b" {
		v, _ = v.Update(tea.KeyPressMsg{Text: string(r)})
	}
	if got := v.filteredModels(); len(got) != 1 || got[0].Name != "gemma3:12b" {
		t.Fatalf("filter after backspace repair = %+v", got)
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.selectorOpen {
		t.Fatal("enter should close the picker")
	}
	if v.model != "gemma3:12b" {
		t.Errorf("model = %q, want gemma3:12b (the default, starred)", v.model)
	}
}

func TestModelPickerFilterResetsOnClose(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Text: "m"})
	for _, r := range "qwen" {
		v, _ = v.Update(tea.KeyPressMsg{Text: string(r)})
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.selFilter != "" {
		t.Errorf("selFilter = %q after esc, want reset", v.selFilter)
	}
	v, _ = v.Update(tea.KeyPressMsg{Text: "m"})
	if len(v.filteredModels()) != 2 {
		t.Errorf("reopened picker should show all models, got %d", len(v.filteredModels()))
	}
}

func TestContextMeterMathMirrorsAgentEstimator(t *testing.T) {
	// The same message list must estimate identically in the agent package and
	// the UI meter (M7-C single source of truth). NewAgentView leaves the
	// system prompt empty (compat constructor), so set the real one.
	v := testAgent(t, nil)
	v.systemPrompt = config.Default().Agent.SystemPrompt
	msgs := v.payloadMessages()
	if len(msgs) == 0 {
		t.Fatal("payload should at least carry the system prompt")
	}
	if agent.ApproxTokens(msgs) != v.ctxTokens() {
		t.Errorf("estimator mismatch: agent=%d ui=%d", agent.ApproxTokens(msgs), v.ctxTokens())
	}
}
