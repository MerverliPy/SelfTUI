package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// --- rendering ------------------------------------------------------------

// assistantHeader is the "who replied" chip shown at the left of each
// assistant block header line (streaming and committed).
func (v AgentView) assistantHeader(model string) string {
	if model == "" {
		model = "assistant"
	}
	return v.styles.agentAccent().Render("◈ " + model)
}

// assistantHeaderRow is a full-width header line: the model chip on the left
// and, when the turn finished, elapsed + reason meta right-aligned on the
// same row (opencode session headers carry time/status on the right). The
// pad uses the chat pane's inner width so meta always lands at the margin.
func (v AgentView) assistantHeaderRow(model, meta string) string {
	left := v.assistantHeader(model)
	if meta == "" {
		return left
	}
	// Pad to the bucketed render width (N1 micro-item), not the exact pane:
	// the header is baked into each cached block, so its padding must be a
	// pure function of the same bucket the markdown used. A same-bucket
	// resize then leaves every cached header still correct (bucket ≤ pane,
	// so it can never exceed the pane), and a cross-bucket rebuild realigns
	// it. Cost: right-aligned meta may sit a few columns short of the margin
	// on non-bucket-aligned panes; the 72×30 device pane (70) is exact.
	inner := chatRenderWidth(maxInt(v.w-2, 10))
	pad := inner - lipgloss.Width(left) - lipgloss.Width(meta)
	if pad < 1 {
		return left + "  " + v.styles.mutedText().Render(meta)
	}
	return left + strings.Repeat(" ", pad) + v.styles.mutedText().Render(meta)
}

// userHeader is the marker above each user block.
func (v AgentView) userHeader() string {
	return v.styles.agentAccent().Render("❯ you")
}

// turnFooter renders the per-turn meta (M7-B): elapsed wall time plus the
// terminal reason — the model's ollama done_reason ("stop"/"length") or
// "stopped" when the user cut the stream with esc — and, when the turn's
// final chunk carried generation metrics, the measured tok/s (N3). Returns
// "" when no start time was recorded (an assistant block committed without
// startChat), so hand-constructed transcripts in tests render no meta.
// Displayed on the assistant header's right side. tokPerSec <= 0 (metrics
// absent, zero duration, or a user stop) keeps the footer byte-identical to
// the pre-N3 shape.
func turnFooter(start time.Time, reason string, stopped bool, tokPerSec int) string {
	elapsed := ""
	if !start.IsZero() {
		elapsed = fmt.Sprintf("%.1fs", time.Since(start).Seconds())
	}
	if elapsed == "" {
		return ""
	}
	var meta string
	switch {
	case stopped:
		return elapsed + " · stopped"
	case reason != "":
		meta = elapsed + " · " + reason
	default:
		meta = elapsed
	}
	if tokPerSec > 0 {
		meta += fmt.Sprintf(" · %d tok/s", tokPerSec)
	}
	return meta
}

// turnTokPerSec converts a done event's final-chunk metrics into the footer's
// integer tok/s (eval_count / eval_duration, the duration in nanoseconds).
// 0 means no displayable rate: metrics absent or a zero duration.
func turnTokPerSec(m ollama.ChatMetrics) int {
	if m.Tokens <= 0 || m.Nanos <= 0 {
		return 0
	}
	return int(math.Round(float64(m.Tokens) * 1e9 / m.Nanos))
}

// chatRenderWidth buckets the chat pane's render width down to a multiple of
// five (N1 micro-item, PLAN §12 N7): resize jitter inside one bucket reuses
// the bucketed glamour renderer and the per-width caches instead of
// rebuilding and re-rendering on every wiggle. Rounding is down so a bucketed
// render never exceeds the real pane; panes narrower than one bucket keep
// their exact width.
func chatRenderWidth(pane int) int {
	pane = maxInt(pane, 1)
	if pane < 5 {
		return pane
	}
	return pane - pane%5
}

// ensureRenderer builds the glamour renderer when it is missing or the
// (bucketed) width changed. renderBlock calls it lazily so streaming renders
// never fail.
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

// rebuildRenderer (re)builds the renderer for the current (bucketed) width
// after a geometry change. A failed build keeps the previous renderer; View
// falls back to raw text so output is never silent.
func (v *AgentView) rebuildRenderer() {
	v.ensureRenderer(chatRenderWidth(v.w - 2))
}

// rebuildRenderCache re-renders every committed block at the current width.
// Only called on geometry changes (rare); renderBlock uses the shared cache
// every frame otherwise.
func (v *AgentView) rebuildRenderCache() {
	if v.renderW != chatRenderWidth(v.w-2) {
		return // renderer is stale; ensureRenderer on next renderBlock fixes it
	}
	for i := range v.turns {
		v.turns[i].render = v.renderBlock(v.headerFor(i), v.turns[i].msg.Content)
	}
}

// headerFor returns the role header line for a committed turn index.
// Assistant rows carry their model chip (and right-aligned meta when the
// turn recorded one); user rows keep the plain "❯ you" marker.
func (v AgentView) headerFor(i int) string {
	t := v.turns[i]
	if t.msg.Role == ollama.RoleAssistant {
		return v.assistantHeaderRow(t.model, t.meta)
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
	// H-05 boundary: chat content (streamed tokens and committed turns, from
	// either role) is the audit-cited leak site — glamour passes ESC payload
	// bytes through its styled output, and the raw-markdown fallback below
	// returns md verbatim. Sanitize before both so neither branch can carry
	// a hostile sequence into the terminal; user text is local but harmless
	// to strip here (display-only, idempotent).
	md = sanitizeTerminalText(md)
	v.ensureRenderer(chatRenderWidth(v.w - 2))
	if v.tr != nil {
		if out, err := v.tr.RenderBytes([]byte(md)); err == nil {
			return header + "\n" + strings.TrimSuffix(string(out), "\n")
		}
	}
	return header + "\n" + md
}

// chatLines assembles the full rendered transcript as individual display
// lines: an optional truncation marker (M7-C), the cached history blocks
// (header + markdown content + per-turn footer), plus the live streaming
// block with the streaming caret (M7-B). It is the O(total) convenience
// view over the same windowing machinery renderChatPane uses — production
// frames go through chatWindow so only the visible rows are materialized
// (N1); a test-side naive copy of this assembly pins the two paths equal.
func (v AgentView) chatLines() []string {
	return v.chatWindow(0, math.MaxInt)
}

// truncationMarkerLines returns the transcript-head marker shown when the
// runner silently omitted older turns to fit the budget (M7-C); empty when
// nothing was truncated.
func (v AgentView) truncationMarkerLines() []string {
	if !v.truncated {
		return nil
	}
	return []string{
		v.styles.mutedText().Render("… " + agent.TruncationNotice),
		"",
	}
}

// effectiveBlock returns the turn's display block: the cached per-width
// render, or the sanitized raw content when the cache entry is empty. The
// fallback covers a render gap (e.g. a hand-built transcript in tests); raw
// history must never reach the transcript unsanitized (H-05).
func (t turn) effectiveBlock() string {
	if t.render != "" {
		return t.render
	}
	return sanitizeTerminalText(t.msg.Content)
}

// turnBlock composes one committed turn's full display block (N6): its
// gated extras — the reasoning block behind /thinking, the tool-activity
// lines behind /details, in that order (reasoning precedes the tool calls,
// the tool calls precede the final answer) — prepended to the effective
// content block. With both toggles off or empty extras this is exactly
// effectiveBlock, so default frames are byte-identical to the pre-N6
// renderer by construction.
func (v AgentView) turnBlock(i int) string {
	t := v.turns[i]
	block := t.effectiveBlock()
	var extras []string
	if v.showThinking && t.thinking != "" {
		extras = append(extras, v.reasoningBlock(t.thinking))
	}
	if v.showDetails {
		for _, l := range t.tools {
			extras = append(extras, v.styles.mutedText().Render(l))
		}
	}
	if len(extras) == 0 {
		return block
	}
	if block == "" {
		return strings.Join(extras, "\n")
	}
	return strings.Join(extras, "\n") + "\n" + block
}

// turnDisplayLines returns the display lines one committed turn contributes
// to the transcript: its composed block split on newlines plus the trailing
// separator row. An empty block contributes nothing at all — not even the
// separator — so blank turns never leave a gap in the transcript.
func (v AgentView) turnDisplayLines(i int) []string {
	block := v.turnBlock(i)
	if block == "" {
		return nil
	}
	lines := strings.Split(block, "\n")
	return append(lines, "") // separator after each message
}

// turnLineCount is the row count turnDisplayLines would produce, computed by
// newline arithmetic on the composed block: a non-empty block splits into
// Count("\n")+1 lines plus the separator row. With the toggles off this
// composes nothing new (turnBlock returns the cached block directly), so
// chatLineCount walks a 2,000-turn transcript with this alone.
func (v AgentView) turnLineCount(i int) int {
	block := v.turnBlock(i)
	if block == "" {
		return 0
	}
	return strings.Count(block, "\n") + 2
}

// streamExtrasVisible reports whether the in-flight turn currently has
// gated extras to render (N6): live reasoning behind /thinking or tool
// activity behind /details. The content stream is not part of this.
func (v AgentView) streamExtrasVisible() bool {
	return (v.showThinking && v.thinkingText != "") || (v.showDetails && len(v.streamTools) > 0)
}

// streamDisplayLines renders the live streaming block: the gated extras
// (reasoning behind /thinking, tool lines behind /details), then header +
// content split on newlines, the streaming caret riding the last line while
// a turn streams (M7-B), and the trailing separator. Rendered only once the
// model is actually producing something — no phantom empty header/caret
// while a tool runs (that state lives on the statusline; N6 moves thinking
// behind the toggle instead of leaving it invisible).
func (v AgentView) streamDisplayLines() []string {
	if v.streamText == "" && !v.streamExtrasVisible() {
		return nil
	}
	var sb []string
	if v.showThinking && v.thinkingText != "" {
		sb = append(sb, strings.Split(v.reasoningBlock(v.thinkingText), "\n")...)
	}
	if v.showDetails {
		for _, l := range v.streamTools {
			sb = append(sb, v.styles.mutedText().Render(l))
		}
	}
	if v.streamText != "" {
		block := strings.Split(v.streamBlockRender(), "\n") // N2: cache-backed
		if v.streaming {
			block = withStreamingCaret(block)
		}
		sb = append(sb, block...)
	}
	return append(sb, "")
}

// streamLineCount is the row count streamDisplayLines would produce, by
// newline arithmetic on the same sources (no composition on the count path):
// the reasoning block is one marker row plus its body lines, tool lines
// count one each, and the content block is cache-backed. Zero when the turn
// contributes nothing.
func (v AgentView) streamLineCount() int {
	n := 0
	if v.showThinking && v.thinkingText != "" {
		n += 2 + strings.Count(v.thinkingText, "\n")
	}
	if v.showDetails {
		n += len(v.streamTools)
	}
	if v.streamText != "" {
		block := v.streamBlockRender()      // N2: cache-backed
		n += strings.Count(block, "\n") + 1 // split lines
		if v.streaming && strings.HasSuffix(block, "\n") {
			n++ // caret becomes its own row after the blank last line
		}
	}
	if n == 0 {
		return 0
	}
	return n + 1 // separator
}

// trailingBlankCount counts the blank lines chatLines drops from the tail of
// the full transcript. Blanks can only trail inside the last contributing
// source: every earlier source ends with its separator row immediately
// followed by the next source's header, and empty sources contribute
// nothing. Only the tail source is examined.
func (v AgentView) trailingBlankCount() int {
	if tail := v.streamDisplayLines(); len(tail) > 0 {
		// The stream source contributes: count its trailing blanks (the
		// separator row included — it is what the tail trim drops).
		n := 0
		for i := len(tail) - 1; i >= 0 && tail[i] == ""; i-- {
			n++
		}
		return n
	}
	for i := len(v.turns) - 1; i >= 0; i-- {
		block := v.turnBlock(i)
		if block == "" {
			continue
		}
		return strings.Count(block[len(strings.TrimRight(block, "\n")):], "\n") + 1
	}
	if v.truncated {
		return 1 // marker row + its separator; the marker itself is non-empty
	}
	return 0
}

// chatLineCount returns the total number of transcript display lines —
// exactly len(chatLines()) — without splitting any block: committed turns
// are counted by newline arithmetic (O(turns), zero allocs) and only the
// tail source is walked for the trailing-blank drop.
func (v AgentView) chatLineCount() int {
	total := len(v.truncationMarkerLines())
	for i := range v.turns {
		total += v.turnLineCount(i)
	}
	total += v.streamLineCount()
	if blanks := v.trailingBlankCount(); blanks > 0 {
		if blanks > total {
			blanks = total
		}
		total -= blanks
	}
	return total
}

// chatWindow materializes only the transcript display lines in [start, end)
// — the N1 render window. Committed turns are advanced by their line
// counts; only blocks overlapping the window are split and copied, so a
// tail window (the follow-mode common case) touches O(visible) rows no
// matter how long the session. Output is byte-identical to the O(total)
// assembly; a test-side naive copy pins that equivalence.
func (v AgentView) chatWindow(start, end int) []string {
	return v.chatWindowTotal(start, end, v.chatLineCount())
}

// chatWindowTotal is chatWindow with the transcript total supplied by the
// caller (renderChatPane already computed it for the scroll clamp; this
// avoids a second counting walk per frame).
func (v AgentView) chatWindowTotal(start, end, total int) []string {
	if end > total {
		end = total
	}
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	var out []string
	pos := 0
	take := func(lines []string) {
		lo := start - pos
		if lo < 0 {
			lo = 0
		}
		hi := end - pos
		if hi > len(lines) {
			hi = len(lines)
		}
		if lo < hi {
			out = append(out, lines[lo:hi]...)
		}
		pos += len(lines)
	}
	if marker := v.truncationMarkerLines(); len(marker) > 0 {
		take(marker)
	}
	for i := range v.turns {
		if pos >= end {
			break
		}
		n := v.turnLineCount(i)
		if n == 0 {
			continue
		}
		if pos+n <= start {
			pos += n // entirely before the window: skip the split
			continue
		}
		take(v.turnDisplayLines(i))
	}
	if pos < end {
		take(v.streamDisplayLines())
	}
	// A window ending at the transcript tail must end where chatLines ends:
	// the trailing-blank drop.
	if end == total {
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
	}
	return out
}

// withStreamingCaret appends the streaming caret ("▍") to the live block. It
// rides the last visible line while a turn streams text and disappears the
// moment the turn commits and streaming goes false (M7-B).
func withStreamingCaret(lines []string) []string {
	if len(lines) == 0 {
		return []string{"▍"}
	}
	last := len(lines) - 1
	if strings.TrimSpace(lines[last]) != "" {
		lines[last] += "▍"
	} else {
		lines = append(lines, "▍")
	}
	return lines
}
