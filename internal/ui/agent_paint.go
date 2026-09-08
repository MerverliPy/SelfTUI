package ui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MerverliPy/SelfTUI/internal/config"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// View renders transcript + hint + input per the current geometry. A modal
// (model selector, mutation approval, write_files batch review, /help, /clear,
// /undo confirm) replaces the body with a centered overlay; the slash-command
// menu (M7-A) floats between the transcript and the input while a "/" draft is
// being composed.
func (v AgentView) View() string {
	bodyH := maxInt(v.h-2, 1)

	if v.confirmation != nil {
		return v.renderConfirmationOverlay(bodyH)
	}
	if v.batchReview != nil {
		return v.renderBatchReviewOverlay(bodyH)
	}
	if v.undoConfirm {
		return v.renderUndoOverlay(bodyH, false)
	}
	if v.redoConfirm {
		return v.renderUndoOverlay(bodyH, true)
	}
	if v.selectorOpen {
		return v.renderSelectorOverlay(bodyH)
	}
	if v.resumeOpen {
		return v.renderResumeOverlay(bodyH)
	}
	if v.resumeConfirm {
		return v.renderResumeConfirmOverlay(bodyH)
	}
	if v.helpOpen {
		return v.renderHelpOverlay(bodyH)
	}
	if v.clearConfirm {
		return v.renderClearConfirmOverlay(bodyH)
	}

	if len(v.models) == 0 {
		return v.fullSizePane(bodyH)
	}

	// Bottom region (opencode footer anatomy): the composer pane (a header
	// row with model chip + context usage, then the auto-growing prompt),
	// below it a one-row statusline (spinner/status · interrupt, legend).
	// The slash menu or the @-file picker (N6) floats between the transcript
	// and the composer while its draft is being composed.
	menuH := 0
	switch {
	case v.fileMenuActive():
		menuH = v.fileMenuHeight()
	case v.slashMenu():
		menuH = v.slashMenuHeight()
	}
	composerH := v.composerRows() + 3
	chatH := maxInt(bodyH-composerH-1-menuH, 1)

	chatPane := v.renderChatPane(chatH)
	composer := v.renderComposer()
	status := v.statusLine()

	out := chatPane
	if menuH > 0 {
		menu := v.renderSlashMenu()
		if v.fileMenuActive() {
			menu = v.renderFileMenu()
		}
		out += "\n" + menu
	}
	return out + "\n" + composer + "\n" + status
}

// renderChatPane windows the transcript into the scroll region.
func (v AgentView) renderChatPane(h int) string {
	if h < 2 {
		return ""
	}
	// N1 windowing: count the transcript by newline arithmetic (no block
	// splits), then materialize only the visible rows. Rendering never
	// mutates state: the effective offset is computed locally. Follow
	// anchors to the tail (offset 0); otherwise the stored offset clamps to
	// the current content, so a shrink never shows past the head.
	total := v.chatLineCount()
	contentH := h - 2
	scroll := 0
	if !v.follow {
		scroll = clampInt(v.scroll, 0, maxInt(0, total-contentH))
	}

	// Window honors the scroll offset: scroll 0 shows the tail; scrolling
	// up shifts the window toward the head.
	end := total - scroll
	start := maxInt(0, end-contentH)
	window := v.chatWindowTotal(start, end, total)
	pane := v.styles.Pane.Width(v.w).Height(h)
	return pane.Render(lipgloss.JoinVertical(lipgloss.Left, window...))
}

// composerRows is the current prompt height in terminal rows (1..4).
func (v AgentView) composerRows() int {
	return composerRowsFor(v.input.Value(), maxInt(v.w-2, 10))
}

// renderComposer draws the composer block: a header row (model chip left,
// context meter + token usage right) above the growing prompt textarea, all
// inside one bordered pane (opencode footer anatomy).
func (v AgentView) renderComposer() string {
	innerW := maxInt(v.w-2, 10)
	ta := v.input
	rows := composerRowsFor(ta.Value(), innerW)
	ta.SetWidth(innerW)
	ta.SetHeight(rows)
	content := v.composerHeader() + "\n" + ta.View()
	return v.styles.Pane.Width(v.w).Height(rows + 3).Render(content)
}

// composerHeader is the composer's top row: model chip on the left, context
// usage (bar + percent + k-tokens) on the right — identity left, activity
// right, like opencode's footer.
func (v AgentView) composerHeader() string {
	innerW := maxInt(v.w-2, 10)
	left := v.assistantHeader(v.model)
	right := ""
	plain := v.ctxMeterPlain()
	if usage := v.ctxUsage(); usage != "" {
		plain += " · " + usage
	}
	// N4: tier colors via the shared ctxTierFor so the composer header and
	// the shell status row cannot drift apart.
	switch ctxTierFor(v.ctxPct()) {
	case ctxRed:
		right = v.styles.Error.Render("ctx full — /clear")
	case ctxAmber:
		right = v.styles.warnText().Render(plain)
	default:
		right = v.styles.mutedText().Render(plain)
	}
	pad := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if pad >= 1 {
		return left + strings.Repeat(" ", pad) + right
	}
	// Very narrow: keep the identity, drop the activity rather than wrap.
	room := innerW - lipgloss.Width(right) - 1
	if room > 0 {
		return truncateToWidth(left, room) + " " + right
	}
	return truncateToWidth(left, innerW)
}

// ctxUsage renders the approximate payload tokens as "1.2k/3.1k" against the
// input budget, mirroring opencode's token usage meta.
func (v AgentView) ctxUsage() string {
	limit := v.ctxLimit()
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1fk/%.1fk", float64(v.ctxTokens())/1000, float64(limit)/1000)
}

// composing reports whether the chat input holds text. The shell's digit-key
// tab jumps are disabled while composing so bare digits type into the prompt
// (M6 fix found by the reconnect smoke); with an empty input digits still
// switch tabs.
func (v AgentView) composing() bool {
	return v.input.Value() != ""
}

// statusLine is the one-row strip under the composer (opencode footer
// anatomy): an error, the running state with its interrupt hint, a transient
// notice, or the width-fitted key legend. Context usage lives in the
// composer header above, so this row stays a status/legend only.
func (v AgentView) statusLine() string {
	maxW := maxInt(v.w-2, 24)

	switch {
	case v.chatErr != "":
		return v.styles.Error.Render(truncateToWidth("⚠ "+v.chatErr+" — enter to retry", maxW))
	case v.streaming:
		left := "running…"
		if v.toolStatus != "" {
			left = firstLine(v.toolStatus)
		}
		right := "esc interrupt"
		if v.stopArmed {
			right = "esc again to interrupt"
		}
		return v.statusRow(maxW, []string{left}, right, v.stopArmed)
	case v.notice != "":
		return v.styles.Placeholder.Render(truncateToWidth(v.notice, maxW))
	case v.remoteHost():
		// Persistent remote-host warning (Phase 4): tools against a
		// non-loopback host can carry workspace content off the machine, so
		// the statusline says so until the user turns tools off or points at
		// a local host. It outranks the key legend on purpose.
		warn := "⚠ tools on — workspace content may be sent to " + v.host
		return v.styles.Error.Render(truncateToWidth(warn, maxW))
	}

	// Legend. While composing, the letter commands (m/r/u/d/f) are off; with
	// an empty input they are advertised (the interrupt/stop hint only shows
	// while running). The leading identity segment — tools state + canonical
	// workspace — is the one thing that survives width pressure (the legend
	// drops from the tail first), so the Agent view always answers "what can
	// this model touch?" even on a phone.
	if v.input.Value() != "" {
		return v.statusRow(maxW, []string{"enter send", "shift+enter newline", "esc clear draft"}, "", false)
	}
	identity := toolsChip(v.toolsEnabled)
	if v.workspace != "" {
		identity += " · " + v.workspace
	}
	segs := []string{identity, "enter send", "/ commands", "m model", "r refresh", "shift+enter newline"}
	return v.statusRow(maxW, segs, "", false)
}

// toolsChip is the stable tools-state label shared by the status bar and the
// Agent statusline.
func toolsChip(on bool) string {
	if on {
		return "tools on"
	}
	return "tools off"
}

// remoteHost reports whether tools are armed against a host that is not
// loopback — the only combination that can send workspace content off this
// machine. An unknown (empty) host never warns.
func (v AgentView) remoteHost() bool {
	return v.toolsEnabled && v.host != "" && !config.LoopbackHost(v.host)
}

// statusRow styles one statusline: Placeholder legend segments joined with
// " · " plus an optional right chip (warn=true renders it red). Segments
// drop from the tail until the row fits maxW so it never wraps on a phone.
func (v AgentView) statusRow(maxW int, segments []string, right string, warn bool) string {
	plain := strings.Join(segments, " · ")
	if right != "" {
		plain += " · " + right
	}
	for lipgloss.Width(plain) > maxW && len(segments) > 1 {
		segments = segments[:len(segments)-1]
		plain = strings.Join(segments, " · ")
		if right != "" {
			plain += " · " + right
		}
	}
	if lipgloss.Width(plain) > maxW {
		room := maxW
		if right != "" {
			room -= lipgloss.Width(right) + 3 // " · "
		}
		if room > 0 {
			segments[0] = truncateToWidth(segments[0], room)
		}
	}

	styled := v.styles.Placeholder.Render(strings.Join(segments, " · "))
	if right == "" {
		return styled
	}
	if warn {
		return styled + " · " + v.styles.Error.Render(right)
	}
	return styled + " · " + v.styles.mutedText().Render(right)
}

// truncateToWidth trims s to at most maxW visible columns and, when it had to
// cut, appends "…" so truncated text is visibly truncated (lipgloss MaxWidth
// alone cuts silently). ANSI sequences count as zero width and are never
// split mid-sequence, so styled text keeps its styling.
func truncateToWidth(s string, maxW int) string {
	if maxW < 1 {
		return ""
	}
	if lipgloss.Width(s) <= maxW {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	cols := 0
	limit := maxW - 1 // reserve the ellipsis column
	for i := 0; i < len(runes); {
		r := runes[i]
		if r == '\x1b' {
			// Copy the whole escape sequence without counting width.
			j := i + 1
			for j < len(runes) && !isAnsiFinal(runes[j]) {
				j++
			}
			if j < len(runes) {
				j++
			}
			b.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		w := lipgloss.Width(string(r))
		if cols+w > limit {
			break
		}
		b.WriteRune(r)
		cols += w
		i++
	}
	return b.String() + "…"
}

// isAnsiFinal reports whether r can end an ANSI escape sequence (final bytes
// are letters; lipgloss only emits SGR sequences ending in 'm').
func isAnsiFinal(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// fullSizePane renders the loading / error / empty model-list states.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

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

// renderConfirmationOverlay asks for the one explicit approval required for
// every write, edit, and constrained command. It intentionally has no default
// affirmative key; only y or enter approves, and esc declines.
func (v AgentView) renderConfirmationOverlay(bodyH int) string {
	c := v.confirmation
	if c == nil {
		return ""
	}
	title := "Confirm mutation"
	if c.Name == "run_command" {
		title = "Confirm sandboxed command"
	}
	lines := []string{
		"Allow " + c.Name + "?",
		"workspace: " + c.Workspace,
		"timeout: " + c.Timeout.String(),
		"input: " + c.Input,
		"",
		"y / enter approve · n / esc decline",
	}
	return v.renderOverlayTitle(bodyH, title, lines)
}

// renderBatchReviewOverlay is the write_files review stage (V2e §4.2): one
// summary row per file plus its rendered one-column unified diff. The batch
// pages one file at a time (V2e residual: review-overlay density at 72×30 for
// a 16-op batch) — an early tall diff can never push later files below the
// fitContent cut, so approving all can never silently include files that were
// off-screen. The page is view state (batchPage, pgup/pgdn); y/enter approves
// the whole batch and n/esc declines from any page.
func (v AgentView) renderBatchReviewOverlay(bodyH int) string {
	b := v.batchReview
	if b == nil {
		return ""
	}
	if len(b.Files) == 0 {
		// Defensive: the runner refuses empty batches at proposal
		// (validateWriteFilesArgs), but this overlay must never index an
		// empty Files slice if one ever arrives — render a decline-able
		// shell instead of panicking (reviewer P1, V2e residual session).
		lines := []string{"empty batch — nothing to apply", "",
			"n / esc decline"}
		return v.renderOverlayTitle(bodyH, "Review write_files batch", lines)
	}
	page := v.batchPage
	if page < 0 {
		page = 0
	}
	if page >= len(b.Files) {
		page = maxInt(len(b.Files)-1, 0)
	}
	lines := []string{}
	// The note names the whole change set and rides on the first page only.
	if page == 0 && b.Note != "" {
		lines = append(lines, "note: "+b.Note)
	}
	if len(b.Files) > 1 {
		lines = append(lines, fmt.Sprintf("file %d/%d", page+1, len(b.Files)))
	}
	f := b.Files[page]
	lines = append(lines, f.Summary)
	lines = append(lines, f.Rows...)
	lines = append(lines, "")
	legend := "y / enter apply all · n / esc decline"
	if len(b.Files) > 1 {
		legend += " · pgup/pgdn · ↑/↓ or j/k"
	}
	lines = append(lines, legend+" · window "+b.Timeout.String())
	return v.renderOverlayTitle(bodyH, "Review write_files batch", lines)
}

// renderUndoOverlay is the /undo and /redo confirm guard (same shape as the
// /clear dialog): y/enter runs the journal op, n/esc cancels, nothing happens
// on a stray key.
func (v AgentView) renderUndoOverlay(bodyH int, redo bool) string {
	title := "Undo"
	action := "undo the agent's last file change"
	if redo {
		title = "Redo"
		action = "redo the last undone change"
	}
	lines := []string{
		action + "?",
		"",
		"y / enter " + title + " · n / esc cancel",
	}
	return v.renderOverlayTitle(bodyH, title, lines)
}

// slashMenuMaxRows fits the whole command set (eleven commands as of the
// V2e /undo + /redo additions) so the menu never needs its own scroll.
// Raised from 9: every command must stay reachable through the menu.
const slashMenuMaxRows = 11

// renderSelectorOverlay centers the model picker over the body. The picker
// filters as you type (any printable key extends the filter across name,
// family, size, quant), stars the config default model, and windows the list
// around the selection so long model lists stay readable on a phone (M7-C).
func (v AgentView) renderSelectorOverlay(bodyH int) string {
	list := v.filteredModels()
	maxRows := maxInt(bodyH-12, 3)

	lines := []string{"filter: " + v.selFilter}
	if len(list) == 0 {
		lines = append(lines, v.styles.Placeholder.Render("no model matches “"+v.selFilter+"”"))
	} else {
		start := clampInt(v.selIdx-maxRows/2, 0, maxInt(0, len(list)-maxRows))
		end := start + maxRows
		if end > len(list) {
			end = len(list)
		}
		if start > 0 {
			lines = append(lines, fmt.Sprintf("… %d earlier", start))
		}
		for i := start; i < end; i++ {
			m := list[i]
			marker := "  "
			name := m.Name
			if i == v.selIdx {
				marker = "❯ "
			}
			base := marker + name
			if m.Name == v.defaultModel {
				base += " ★"
			}
			// Row width is measured on plain text; styling never changes it.
			row := marker + name
			if m.Name == v.defaultModel {
				row += " " + v.styles.agentAccent().Render("★")
			}
			if i == v.selIdx {
				row = marker + lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(name) + strings.TrimPrefix(row, marker+name)
			}
			if summary := modelSummary(m); summary != "" && lipgloss.Width(base)+2+lipgloss.Width(summary) <= maxInt(v.w-8, 20) {
				row += "  " + v.styles.Placeholder.Render(summary)
			}
			lines = append(lines, row)
		}
		if end < len(list) {
			lines = append(lines, fmt.Sprintf("… %d more", len(list)-end))
		}
	}
	lines = append(lines, "type to filter · ↑/↓ or j/k move · enter pick · esc close")

	return v.renderOverlayTitle(bodyH, "Model", lines)
}

// renderHelpOverlay is the /help reference: slash commands, keys, and the
// command palette. A compact list, not the onboarding flow (package D stays
// out of v0.1 scope).
func (v AgentView) renderHelpOverlay(bodyH int) string {
	lines := []string{
		"slash commands",
		" /clear    clear the conversation (asks first)",
		" /undo     undo the agent's last file change",
		" /redo     redo the last undone change",
		" /model    pick a model",
		" /resume   resume a saved chat transcript",
		" /theme    toggle dark/light for this session",
		" /details  toggle tool-output blocks",
		" /thinking toggle reasoning blocks",
		" /export   flush + reveal the transcript file path",
		" /help     show this reference",
		" /refresh  reload the model list",
		"",
		"composer",
		" enter send · shift+enter newline · esc clear draft",
		" the header row: model chip + live ctx usage",
		"",
		"running",
		" esc arms the interrupt · esc again cancels",
		"",
		"transcript (empty input)",
		" m model · r refresh · u/d scroll · f auto-follow",
		" pgup/pgdn page · digits switch tab",
		"",
		"any tab",
		" ctrl+p command palette · ctrl+c quit",
		"",
		"y / enter approve · n / esc decline",
	}
	return v.renderOverlayTitle(bodyH, "Help", lines)
}

// renderClearConfirmOverlay asks before /clear wipes the conversation (same
// guard rails as every destructive action: y/enter to confirm, esc to back
// out; nothing is cleared on a stray key).
func (v AgentView) renderClearConfirmOverlay(bodyH int) string {
	lines := []string{
		"Clear the conversation?",
		"",
		"y / enter clear · n / esc cancel",
	}
	return v.renderOverlayTitle(bodyH, "Clear conversation", lines)
}

// renderResumeOverlay is the /resume picker: one windowed row per saved
// transcript (newest first) with a humanized mtime + size summary, styled
// after the model picker and height-capped by the shared overlay helper.
func (v AgentView) renderResumeOverlay(bodyH int) string {
	list := v.resumeList
	maxRows := maxInt(bodyH-12, 3)

	lines := []string{"saved chats — enter to resume"}
	switch {
	case v.resumeLoading && len(list) == 0:
		lines = append(lines, v.styles.Placeholder.Render("loading…"))
	case len(list) == 0:
		lines = append(lines, v.styles.Placeholder.Render("no saved sessions"))
	default:
		start := clampInt(v.resumeIdx-maxRows/2, 0, maxInt(0, len(list)-maxRows))
		end := start + maxRows
		if end > len(list) {
			end = len(list)
		}
		if start > 0 {
			lines = append(lines, fmt.Sprintf("… %d earlier", start))
		}
		for i := start; i < end; i++ {
			s := list[i]
			marker := "  "
			if i == v.resumeIdx {
				marker = "❯ "
			}
			// UTC on purpose: the picker row must render identically on any
			// machine (golden fixtures pin it byte-for-byte).
			summary := s.ModTime.UTC().Format("2006-01-02 15:04") + " · " + humanSize(s.Size)
			row := marker + summary
			if i == v.resumeIdx {
				row = marker + lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(summary)
			}
			lines = append(lines, row)
		}
		if end < len(list) {
			lines = append(lines, fmt.Sprintf("… %d more", len(list)-end))
		}
	}
	lines = append(lines, "↑/↓ or j/k move · enter resume · esc close")

	return v.renderOverlayTitle(bodyH, "Resume", lines)
}

// humanSize renders a file size the way picker rows show it ("312 B",
// "4.2 kB", "1.3 MB") — one decimal only above the byte range.
func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// renderResumeConfirmOverlay asks before a resume replaces the live
// conversation (same guard rails as /clear: y/enter confirms, esc backs
// out; nothing is replaced on a stray key).
func (v AgentView) renderResumeConfirmOverlay(bodyH int) string {
	name := v.resumePick
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	lines := []string{
		"Resume over the current conversation?",
		name,
		"",
		"y / enter resume · n / esc cancel",
	}
	return v.renderOverlayTitle(bodyH, "Resume conversation", lines)
}

// slashMenuHeight is the bordered menu's row budget while a slash draft is
// showing (rows + box borders), capped so the chat pane keeps room on a phone.
func (v AgentView) slashMenuHeight() int {
	rows := len(v.slashMatches())
	if rows > slashMenuMaxRows {
		rows = slashMenuMaxRows
	}
	return rows + 2
}

// renderSlashMenu draws the bordered "/" command menu between the transcript
// and the input. The highlighted row is bold/accent; the filter is the draft
// itself (typing narrows the menu live — M7-A).
func (v AgentView) renderSlashMenu() string {
	matches := v.slashMatches()
	if len(matches) == 0 {
		return ""
	}
	rows := len(matches)
	if rows > slashMenuMaxRows {
		rows = slashMenuMaxRows
	}
	innerW := maxInt(v.w-2, 16)

	out := make([]string, 0, rows)
	for i, c := range matches[:rows] {
		marker := "  "
		name := "/" + c.name
		if i == v.slashIdx {
			marker = "❯ "
			name = lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render("/" + c.name)
		}
		prefix := marker + name
		desc := c.desc
		if room := innerW - lipgloss.Width(prefix) - 2; room > 0 {
			desc = truncateToWidth(c.desc, room)
			prefix += "  " + v.styles.Placeholder.Render(desc)
		} else {
			prefix = truncateToWidth(prefix, innerW)
		}
		out = append(out, prefix)
	}
	return v.styles.Pane.Width(v.w).Height(rows + 2).Render(strings.Join(out, "\n"))
}

// renderOverlayTitle centers a bordered dialog over the whole Agent body via
// the shared overlay helper (fitContent keeps every decision row on screen at
// the measured phone geometry).
func (v AgentView) renderOverlayTitle(bodyH int, title string, lines []string) string {
	return renderCenteredOverlay(v.w, bodyH, v.styles, title, lines)
}

// clampScroll bounds the stored scroll offset by the currently visible window
// (composer height is dynamic — same accounting as renderChatPane). It runs
// on the key paths that change scroll; rendering computes its effective
// offset locally and never writes state.
func (v *AgentView) clampScroll() {
	bodyH := maxInt(v.h-2, 1)
	chatH := maxInt(bodyH-v.composerRows()-3-1, 1)
	lines := v.chatLineCount()
	maxScroll := maxInt(0, lines-maxInt(chatH-2, 1))
	if v.scroll > maxScroll {
		v.scroll = maxScroll
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
}

// warnText returns the amber foreground style for the context meter's amber
// tier (N4) — a “getting close” warning between the muted idle bar and the
// red budget-full state.
func (s Styles) warnText() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.warn)
}

// agentAccent returns the accent style applied to role headers.
func (s Styles) agentAccent() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(s.accent)
}

// mutedText returns the muted foreground style for secondary rows (transcript
// footers, the context meter's idle bar) — plain, not the italic Placeholder.
func (s Styles) mutedText() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.muted)
}

// --- M4 live apply: re-theme + config apply -------------------------------

// applyTheme re-tints the Agent tab and rebuilds the glamour renderer at the
// new dark/light state so committed blocks re-render in the active theme.
func (v AgentView) applyTheme(dark bool, styles Styles) AgentView {
	v.styles = styles
	v.dark = dark
	v.rebuildRenderer()
	v.rebuildRenderCache()
	v.primeStreamRender() // N2: active block re-tints with the shell theme
	return v
}

// ApplyConfig applies a successful settings save in-session: scalar chat
// parameters, the default model, the workspace tool trust switch, and the
// host take effect for the next send, and the runner is rebuilt so a new
// workspace root, system prompt, iteration cap, tools state, and client
// apply. An in-flight turn keeps the runner it started with (startChat copies
// the pointer before the goroutine runs), so swapping here is safe
// mid-stream. reload=true (host/token change) clears the selector models and
// refetches from the new host.
func (v AgentView) ApplyConfig(cfg config.Config, c *ollama.Client, reload bool) (AgentView, tea.Cmd) {
	v.client = c
	v.defaultModel = cfg.DefaultModel
	v.temperature = cfg.Agent.Temperature
	v.topP = cfg.Agent.TopP
	v.numCtx = cfg.Agent.NumCtx
	v.systemPrompt = cfg.Agent.SystemPrompt
	root := cfg.WorkspaceRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	v.toolsEnabled = cfg.ToolsEnabled
	v.host = cfg.Host
	v.workspace = canonicalWorkspaceLabel(root)
	v.runner = runnerFor(c, root, cfg.Agent.SystemPrompt, cfg.Agent.MaxToolIterations, cfg.ToolsEnabled).WithJournal(v.undo)
	if v.logger != nil {
		v.runner = v.runner.WithLogger(v.logger)
	}
	if !reload {
		return v, nil
	}
	// A host/token change invalidates every in-flight model-list result of
	// the old client: bump the generation so an obsolete completion is
	// dropped on arrival (M-03), then refetch from the new host.
	v.clientGen++
	v.loading = true
	v.modelsErr = ""
	v.models = nil
	v.model = ""
	v.selIdx = 0
	return v, v.loadModelsCmd()
}
