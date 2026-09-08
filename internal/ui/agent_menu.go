package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// --- slash commands (M7-A) ------------------------------------------------

// slashCommand is one entry of the "/" command menu.
type slashCommand struct {
	name string // matched after the leading "/"
	desc string
}

func slashCommandList() []slashCommand {
	return []slashCommand{
		{"clear", "clear the conversation (asks first)"},
		{"model", "pick a model (m)"},
		{"resume", "resume a saved chat transcript"},
		{"theme", "toggle dark/light for this session"},
		{"details", "toggle tool-output blocks"},
		{"thinking", "toggle reasoning blocks"},
		{"export", "flush + reveal the transcript file path"},
		{"help", "list slash commands and keys"},
		{"refresh", "reload the model list (r)"},
	}
}

// slashQueryOf is the lowercased filter text after the leading "/".
func (v AgentView) slashQueryOf() string {
	return strings.ToLower(strings.TrimPrefix(v.input.Value(), "/"))
}

// slashMenu reports whether the command menu should show: the input holds a
// "/" draft that still matches at least one command. A draft matching
// nothing ("how do I write a /"? chat about a file named /x) is treated as
// ordinary prose and typed/sent normally.
func (v AgentView) slashMenu() bool {
	if v.streaming || v.input.Value() == "" || !strings.HasPrefix(v.input.Value(), "/") {
		return false
	}
	return len(v.slashMatches()) > 0
}

// slashMatches returns the commands whose name starts with the draft filter.
func (v AgentView) slashMatches() []slashCommand {
	q := v.slashQueryOf()
	var out []slashCommand
	for _, c := range slashCommandList() {
		if strings.HasPrefix(c.name, q) {
			out = append(out, c)
		}
	}
	return out
}

// runSlashCommand executes the highlighted command, consuming the draft. Only
// reachable while the menu is open (the highlight is in range).
func (v AgentView) runSlashCommand() (AgentView, tea.Cmd) {
	matches := v.slashMatches()
	if len(matches) == 0 {
		return v, nil
	}
	if v.slashIdx < 0 || v.slashIdx >= len(matches) {
		v.slashIdx = 0
	}
	name := matches[v.slashIdx].name

	ti := v.input
	ti.Reset()
	v.input = ti
	v.slashQuery = ""
	v.slashIdx = 0
	v.fileOpen = false
	v = v.fitComposer() // the draft was consumed; shrink the composer back

	switch name {
	case "clear":
		if len(v.turns) == 0 && v.streamText == "" {
			v.notice = "nothing to clear"
			return v, nil
		}
		v.clearConfirm = true
		return v, nil
	case "resume":
		return v.openResume()
	case "model":
		return v.openSelector()
	case "theme":
		next := "dark"
		if v.dark {
			next = "light"
		}
		// The theme lives on the root App (shared with Settings and Models);
		// emit a message and let App apply it shell-wide.
		return v, func() tea.Msg { return agentThemeMsg{theme: next} }
	case "help":
		v.helpOpen = true
		return v, nil
	case "details":
		// N6: gate the tool-activity blocks (display only — the agent loop's
		// wire payload is untouched). Session-scoped, default off.
		v.showDetails = !v.showDetails
		v.notice = "tool-output blocks " + toggleWord(v.showDetails)
		return v, nil
	case "thinking":
		// N6: surface per-turn reasoning blocks (qwen3 thinking; stored
		// always, shown only behind this toggle). Session-scoped, default off.
		v.showThinking = !v.showThinking
		v.notice = "reasoning blocks " + toggleWord(v.showThinking)
		return v, nil
	case "export":
		return v.exportSession()
	case "refresh":
		v.loading = true
		v.modelsErr = ""
		return v, v.loadModelsCmd()
	}
	return v, nil
}

// toggleWord renders a toggle state for notices.
func toggleWord(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// --- @-file picker (N6) ---------------------------------------------------

// fileMenuMaxRows caps the @-picker's file rows so the menu never needs its
// own scroll and the chat pane keeps room at the phone geometry.
const fileMenuMaxRows = 7

// atQuery reports the composer's live @-trigger: the byte index of the last
// '@' in the draft and the word after it. The trigger is word-bounded — any
// whitespace after the '@' ends it — so the filter is always exactly the
// text the user can see and the picker can never desync from the draft.
func (v AgentView) atQuery() (at int, query string, ok bool) {
	val := v.input.Value()
	i := strings.LastIndexByte(val, '@')
	if i < 0 {
		return 0, "", false
	}
	tail := val[i+1:]
	if strings.ContainsAny(tail, " \t\n\r") {
		return 0, "", false
	}
	return i, tail, true
}

// fileMenuActive reports whether the @-picker owns navigation keys: armed
// and the trigger word still intact.
func (v AgentView) fileMenuActive() bool {
	if !v.fileOpen || v.confirmation != nil {
		return false
	}
	_, _, ok := v.atQuery()
	return ok
}

// fileMatches applies the live filter (the trigger word) to the workspace
// listing. Ranking is fuzzy but predictable: substring matches first (by
// position, then path length), then subsequence matches (fuzzy, by path
// length, then lexicographic). The list is already jail-bounded — the walk
// never leaves the workspace — and every picked path is re-verified through
// the jailed read at expansion time.
func (v AgentView) fileMatches() []string {
	_, q, ok := v.atQuery()
	if !ok || (v.fileLoading && len(v.fileList) == 0) {
		return nil
	}
	q = strings.ToLower(q)
	type scored struct {
		path  string
		class int // 0 = substring, 1 = subsequence
		index int // substring position (lower ranks earlier)
	}
	out := make([]scored, 0, len(v.fileList))
	for _, p := range v.fileList {
		low := strings.ToLower(p)
		if q == "" {
			out = append(out, scored{p, 0, 0})
			continue
		}
		if i := strings.Index(low, q); i >= 0 {
			out = append(out, scored{p, 0, i})
			continue
		}
		if isFuzzySubsequence(low, q) {
			out = append(out, scored{p, 1, 0})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.class != y.class {
			return x.class < y.class
		}
		if x.class == 0 && x.index != y.index {
			return x.index < y.index
		}
		if len(x.path) != len(y.path) {
			return len(x.path) < len(y.path)
		}
		return x.path < y.path
	})
	paths := make([]string, len(out))
	for i, s := range out {
		paths[i] = s.path
	}
	return paths
}

// isFuzzySubsequence reports whether q appears in low as an in-order
// subsequence (byte-wise; the filter is lowercased on both sides).
func isFuzzySubsequence(low, q string) bool {
	i := 0
	for j := 0; j < len(low) && i < len(q); j++ {
		if low[j] == q[i] {
			i++
		}
	}
	return i == len(q)
}

// filePickerKey owns the composer keys while the @-picker is active:
// arrows/j-k steer the highlighted row, enter attaches the selection, esc
// disarms (the draft keeps the typed query). Every other key keeps editing
// the draft — which is the live filter (handled=false falls through to the
// textarea; the trigger re-check closes the picker when the word ends).
// Navigation and enter claim a key only while the filtered list has rows:
// with nothing to act on (listing still loading, no match), the keys fall
// through so enter still sends the draft instead of dying in an empty menu.
func (v AgentView) filePickerKey(k tea.Key) (AgentView, bool, tea.Cmd) {
	matches := v.fileMatches()
	switch {
	case k.Code == tea.KeyEsc:
		v.fileOpen = false
		v.fileIdx = 0
		v.fileFilter = ""
		return v, true, nil
	case k.Code == tea.KeyEnter && !k.Mod.Contains(tea.ModShift) && len(matches) > 0:
		av, cmd := v.pickFile()
		return av, true, cmd
	case (k.Text == "j" || k.Code == tea.KeyDown) && len(matches) > 0:
		if v.fileIdx < len(matches)-1 {
			v.fileIdx++
		}
		return v, true, nil
	case (k.Text == "k" || k.Code == tea.KeyUp) && len(matches) > 0:
		if v.fileIdx > 0 {
			v.fileIdx--
		}
		return v, true, nil
	}
	// Draft text changed: reset the highlight to the top row when the
	// filter changed (same convention as the slash menu).
	if _, q, ok := v.atQuery(); ok && q != v.fileFilter {
		v.fileFilter = q
		v.fileIdx = 0
	}
	return v, false, nil
}

// pickFile inserts the highlighted file as "@path " in place of the typed
// query and disarms the picker. The send path — not the picker — does the
// attachment: the reference is expanded through the jailed read_file when
// the draft is sent, so what the user picked and what the model receives
// always go through the same gate.
func (v AgentView) pickFile() (AgentView, tea.Cmd) {
	matches := v.fileMatches()
	if len(matches) == 0 {
		v.fileOpen = false
		v.fileIdx = 0
		v.fileFilter = ""
		return v, nil
	}
	// Read the highlighted entry before resetting the picker state: the
	// reset must not clobber which row the user picked.
	path := matches[v.fileIdx]
	v.fileOpen = false
	v.fileIdx = 0
	v.fileFilter = ""
	at, _, ok := v.atQuery()
	if !ok {
		return v, nil
	}
	ta := v.input
	// Spaces are escaped ("\ ") so the send-path tokenizer keeps the whole
	// path as one reference (paths with spaces are offered by the picker).
	ta.SetValue(v.input.Value()[:at] + "@" + agent.EscapeFileRef(path) + " ")
	v.input = ta
	return v.fitComposer(), nil
}

// listFilesCmd walks the jailed workspace off the update loop (M-04
// discipline: no filesystem work in Update). The listing is advisory; a
// failure lists as empty ("no files"), never an error surface.
func (v AgentView) listFilesCmd() tea.Cmd {
	root := v.runner.Root()
	ctx := v.ctx
	return func() tea.Msg {
		return agentEventMsg{msg: fileListMsg{files: agent.WorkspaceFiles(ctx, root)}}
	}
}

// fileMenuHeight is the bordered menu's row budget while the @-picker is
// active (rows + hint row + box borders).
func (v AgentView) fileMenuHeight() int {
	rows := len(v.fileMatches())
	if rows > fileMenuMaxRows {
		rows = fileMenuMaxRows
	}
	if rows == 0 {
		rows = 1 // the "listing…" / "no files" row
	}
	return rows + 3
}

// renderFileMenu draws the bordered @-file menu between the transcript and
// the input (same anatomy as the slash menu): the windowed, filtered file
// rows plus one hint row.
func (v AgentView) renderFileMenu() string {
	matches := v.fileMatches()
	innerW := maxInt(v.w-2, 16)

	var rows []string
	if len(matches) == 0 {
		label := "no files match"
		if v.fileLoading {
			label = "listing workspace…"
		}
		rows = append(rows, v.styles.Placeholder.Render(truncateToWidth(label, innerW)))
	} else {
		start := clampInt(v.fileIdx-fileMenuMaxRows/2, 0, maxInt(0, len(matches)-fileMenuMaxRows))
		end := start + fileMenuMaxRows
		if end > len(matches) {
			end = len(matches)
		}
		for i := start; i < end; i++ {
			marker := "  "
			row := marker + matches[i]
			if i == v.fileIdx {
				marker = "❯ "
				row = marker + lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(matches[i])
			}
			rows = append(rows, truncateToWidth(row, innerW))
		}
	}
	rows = append(rows, v.styles.Placeholder.Render(truncateToWidth("type to filter · ↑/↓ or j/k move · enter attach · esc close", innerW)))
	return v.styles.Pane.Width(v.w).Height(v.fileMenuHeight()).Render(strings.Join(rows, "\n"))
}

// --- model selector (M7-C: filter as you type) ----------------------------

func (v AgentView) openSelector() (AgentView, tea.Cmd) {
	if len(v.models) == 0 {
		v.notice = "no models — pull one from the Models tab (p)"
		return v, nil
	}
	v.selectorOpen = true
	v.selFilter = ""
	v.reanchorSelector()
	return v, nil
}

// filteredModels applies the live selector filter, a case-insensitive
// substring across name, family, parameter size, and quantization.
func (v AgentView) filteredModels() []ollama.Model {
	f := strings.ToLower(strings.TrimSpace(v.selFilter))
	if f == "" {
		return v.models
	}
	var out []ollama.Model
	for _, m := range v.models {
		hay := strings.ToLower(strings.Join([]string{m.Name, m.Family, m.ParameterSize, m.Quantization}, " "))
		if strings.Contains(hay, f) {
			out = append(out, m)
		}
	}
	return out
}

// reanchorSelector points the highlight at the current chat model when a
// filter edit still contains it, else at the top of the filtered list.
func (v *AgentView) reanchorSelector() {
	if v.model != "" {
		for i, m := range v.filteredModels() {
			if m.Name == v.model {
				v.selIdx = i
				return
			}
		}
	}
	v.selIdx = 0
}

func (v AgentView) selectorKey(k tea.Key) (AgentView, tea.Cmd) {
	list := v.filteredModels()
	switch {
	case k.Code == tea.KeyEsc:
		v.selectorOpen = false
		v.selFilter = ""
		v.selIdx = 0
	case k.Code == tea.KeyEnter:
		if len(list) > 0 && v.selIdx >= 0 && v.selIdx < len(list) {
			v.model = list[v.selIdx].Name
			v.notice = "model " + v.model
		}
		v.selectorOpen = false
		v.selFilter = ""
	case k.Text == "j" || k.Code == tea.KeyDown:
		if v.selIdx < len(list)-1 {
			v.selIdx++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if v.selIdx > 0 {
			v.selIdx--
		}
	case k.Code == tea.KeyBackspace:
		if v.selFilter != "" {
			v.selFilter = trimLastRune(v.selFilter)
			v.reanchorSelector()
		}
	default:
		// Every other printable rune extends the live filter. j/k stay
		// reserved for navigation; spaces are legal filter characters.
		if t := k.Text; t != "" {
			v.selFilter += t
			v.reanchorSelector()
		}
	}
	return v, nil
}

// trimLastRune removes the final rune (used by the selector filter backspace).
func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return string(r[:len(r)-1])
}

// clearConfirmKey handles y/enter (wipe) vs n/esc (cancel) in the /clear dialog.
func (v AgentView) clearConfirmKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Text == "y" || k.Code == tea.KeyEnter:
		v.clearConfirm = false
		v.notice = "conversation cleared"
		v.turns = nil
		v.scroll = 0
		v.follow = true
		v.truncated = false
		v.measuredPromptTokens = 0
		v.lastTokPerSec = 0 // the conversation was wiped; the rate described its last turn
	case k.Text == "n" || k.Code == tea.KeyEsc:
		v.clearConfirm = false
		v.notice = "clear cancelled"
	}
	return v, nil
}

// --- context budget (M7-C) ------------------------------------------------

// payloadMessages mirrors exactly what the runner will send on the next turn
// (system prompt + committed conversation + in-flight/drafted text), so the
// meter and truncation marker budget against the same payload BudgetMessages
// sees.
func (v AgentView) payloadMessages() []ollama.ChatMessage {
	var msgs []ollama.ChatMessage
	if v.systemPrompt != "" {
		msgs = append(msgs, ollama.ChatMessage{Role: ollama.RoleSystem, Content: v.systemPrompt})
	}
	for i := range v.turns {
		// N6: a user turn with @-references sends its expanded wire content,
		// so the meter and truncation marker budget against the true payload.
		content := v.turns[i].msg.Content
		if v.turns[i].msg.Role == ollama.RoleUser && v.turns[i].wire != "" {
			content = v.turns[i].wire
		}
		msgs = append(msgs, ollama.ChatMessage{Role: v.turns[i].msg.Role, Content: content})
	}
	// N2: budget against the complete stream (deltas awaiting the tick
	// included) — the payload must match what the runner would send.
	if v.streaming {
		if live := v.liveStreamText(); live != "" {
			msgs = append(msgs, ollama.ChatMessage{Role: ollama.RoleAssistant, Content: live})
		}
	}
	if v.input.Value() != "" {
		msgs = append(msgs, ollama.ChatMessage{Role: ollama.RoleUser, Content: v.input.Value()})
	}
	return msgs
}

// ctxTokens counts the approximate tokens of the next payload using the same
// estimator as agent.BudgetMessages (4 chars per token).
func (v AgentView) ctxTokens() int {
	// N3: while the draft is untouched since the measured turn completed,
	// the meter shows the host-reported prompt token count (exact) instead
	// of the approximate estimator; any edit or new turn falls back to
	// ApproxTokens (see the measuredPromptTokens field comment).
	if v.measuredPromptTokens > 0 && v.input.Value() == v.measuredDraft {
		return v.measuredPromptTokens
	}
	return agent.ApproxTokens(v.payloadMessages())
}

// checkContextBudget flags the conversation as truncated when a send exceeds
// the input budget the runner enforces (three quarters of numCtx). The flag
// surfaces the truncation marker in the transcript head until /clear (M7-C:
// make the marker visible, not a silent wire-only drop).
func (v *AgentView) checkContextBudget() {
	limit := v.ctxLimit()
	if limit > 0 && v.ctxTokens() > limit {
		v.truncated = true
	}
}

// ctxLimit is the approximate input-token budget: three quarters of numCtx,
// the same bound BudgetMessages enforces (the model keeps the rest to reply).
func (v AgentView) ctxLimit() int {
	return v.numCtx * 3 / 4
}

// ctxPct is the budget percentage used, 0..100, computed from the same
// approximate tokens BudgetMessages counts.
func (v AgentView) ctxPct() int {
	limit := v.ctxLimit()
	if limit <= 0 {
		return 0
	}
	pct := v.ctxTokens() * 100 / limit
	if pct > 100 {
		pct = 100
	}
	return pct
}

// amberCtxPct is the amber context tier (N4): at or above 80% of the meter's
// existing displayed scale — the ¾-num_ctx input budget the runner enforces —
// usage warns amber before the red-100% tier. OWNER DECISION (PLAN §12 N4):
// the plan's “~80% of num_ctx” is read as 80% on the meter's displayed scale
// so the amber tier sits before (not beyond) today's red-100% tier.
const amberCtxPct = 80

// ctxTier classifies context usage for the meter's tier colors. Shared by the
// composer header and the shell status row so the tiers cannot drift.
type ctxTier int

const (
	ctxOK ctxTier = iota
	ctxAmber
	ctxRed
)

// ctxTierFor maps a meter percentage onto its tier: red at 100% (the budget
// is full — sending past it truncates the model's reply), amber from
// amberCtxPct, OK below.
func ctxTierFor(pct int) ctxTier {
	switch {
	case pct >= 100:
		return ctxRed
	case pct >= amberCtxPct:
		return ctxAmber
	default:
		return ctxOK
	}
}

// ctxMeterPlain renders the plain (unstyled) live context meter, e.g.
// "ctx ▓▓░░░ 38%", shown on the composer header's right side.
func (v AgentView) ctxMeterPlain() string {
	limit := v.ctxLimit()
	if limit <= 0 {
		return ""
	}
	pct := v.ctxPct()
	filled := (pct + 9) / 20
	if filled > 5 {
		filled = 5
	}
	bar := strings.Repeat("▓", filled) + strings.Repeat("░", 5-filled)
	return "ctx " + bar + " " + fmt.Sprintf("%d%%", pct)
}

// ctxMeterSegment renders the status row's compact context meter (N4), with
// its tier color: amber at the amber tier, red at 100%, the shell's plain
// muted style below. Empty when there is nothing to meter (no num_ctx, or a
// zero-token conversation) so a no-data frame stays byte-identical to the
// pre-N4 status row.
func (v AgentView) ctxMeterSegment() string {
	plain := v.ctxMeterPlain()
	if plain == "" || v.ctxTokens() == 0 {
		return ""
	}
	switch ctxTierFor(v.ctxPct()) {
	case ctxAmber:
		return v.styles.warnText().Render(plain)
	case ctxRed:
		return v.styles.Error.Render(plain)
	default:
		return plain
	}
}
