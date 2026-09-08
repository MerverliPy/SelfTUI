package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// mergeThinkingDeltas folds pendingThinking into thinkingText (N6). Pure
// state merge on the repaint tick, mirroring mergeStreamDeltas; thinking
// never touches streamText so the assistant content stream stays aligned
// with the N2 render cache.
func (v *AgentView) mergeThinkingDeltas() {
	if v.pendingThinking == "" {
		return
	}
	// Reasoning comes off the remote response and can carry control bytes
	// like any model text; sanitize at the merge so every stored copy — the
	// live block and the committed turn — is clean (same discipline as
	// boundedToolLine).
	v.thinkingText += sanitizeTerminalText(v.pendingThinking)
	v.pendingThinking = ""
}

// boundedToolLine caps one tool-activity line's stored length so a 64 KiB
// tool result cannot flood the /details block (the full result already went
// back to the model through ToolResultMsg).
func boundedToolLine(s string) string {
	s = sanitizeTerminalText(s)
	const max = 2048
	if len(s) <= max {
		return s
	}
	return s[:max] + "…[truncated]"
}

// toolDetailRows bounds one tool-event line and splits it into one stored
// element per display row: streamLineCount accounts one row per element, so
// a multi-line tool summary stored as a single element would render taller
// than its count and push the composer/status area off-screen during the
// live tool phase.
func toolDetailRows(s string) []string {
	return strings.Split(boundedToolLine(s), "\n")
}

// reasoningBlock composes a turn's reasoning for display (N6): a muted
// marker row above the raw reasoning text. Plain text, no glamour —
// reasoning is verbose and the block must stay cheap to compose per frame.
func (v AgentView) reasoningBlock(thinking string) string {
	return v.styles.mutedText().Render("· reasoning\n" + thinking)
}

// plainChatFallbackNote is the persistent F1 caveat appended to a
// plain-chat-fallback assistant turn. The runner downgraded the turn before
// any tool executed, so a narrated tool claim ("I wrote file X") is not
// real; the note says so inline and in the exported/resumed transcript. It
// is a markdown blockquote, so the raw transcript file and the rendered
// terminal agree. reason is runner-supplied display text, sanitized when it
// entered state and again here (idempotent) before it joins the markdown
// body.
func plainChatFallbackNote(reason string) string {
	return "\n\n> ⚠ **plain chat** — no tool ran this turn: " + sanitizeTerminalText(reason)
}

// onChatDone finalizes a turn: commits the streamed text as an assistant
// message whose header carries elapsed + terminal reason right-aligned
// (M7-B/opencode-style), then surfaces an error unless the user stopped the
// stream with esc (a stop is not an error).
func (v AgentView) onChatDone(m agent.AgentDoneMsg) (AgentView, tea.Cmd) {
	// N2: land any deltas still awaiting the repaint tick before the commit —
	// the final chunk must flush immediately (no tick-interval tail latency
	// for the committed turn or the N3 footer metrics riding its meta row).
	v.mergeStreamDeltas()
	v.mergeThinkingDeltas() // a ThinkingMsg racing AgentDoneMsg flushes here too
	v.streaming = false
	v.stopCancel = nil
	v.stopArmed = false
	v.chatCh = nil
	v.chatDone = nil
	v.chatTerminal = nil // cA: the turn's retained terminal slot is consumed; next turn allocates a fresh one
	v.toolStatus = ""
	v.confirmation = nil

	// N3: the done event's metrics describe the payload that was just sent,
	// independent of whether any text streamed back. When the final chunk
	// carried a measured prompt token count, the ctx meter shows it until
	// the draft or turn changes; an older host (no metrics) clears any
	// stale measurement instead.
	if m.Metrics.PromptTokens > 0 {
		v.measuredPromptTokens = int(m.Metrics.PromptTokens)
		v.measuredDraft = v.input.Value()
	} else {
		v.measuredPromptTokens = 0
	}
	// N4: remember the measured rate for the shell status row; a user stop
	// keeps no rate, exactly like the turn footer.
	if t := turnTokPerSec(m.Metrics); t > 0 && !v.stopRequest {
		v.lastTokPerSec = t
	}

	var recCmd tea.Cmd // ack waiter for the committed assistant turn, if any

	if v.streamText != "" {
		// The Ollama done_reason on the done event is remote text rendered on
		// the assistant header; sanitize it before it becomes turn meta.
		// N3: when the final chunk carried generation metrics, the measured
		// tok/s rides the same meta row. The N6 extras (reasoning, tool
		// lines) commit with the content; a turn that produced no content
		// (error mid-loop) commits nothing, exactly as before N6.
		meta := turnFooter(v.turnStart, sanitizeTerminalText(m.Reason), v.stopRequest, turnTokPerSec(m.Metrics))
		// F1: a plain-chat fallback (agent.FallbackMsg) means the runner
		// downgraded this turn before any tool executed — a tool claim the
		// reply narrates is not real. Append the persistent caveat to the
		// committed content once, so the in-memory turn, its render cache,
		// the /export transcript, and a later /resume reload all carry it
		// (the transcript mirrors what the terminal shows).
		content := v.streamText
		if v.plainChatReason != "" {
			content += plainChatFallbackNote(v.plainChatReason)
			v.plainChatReason = ""
		}
		v.turns = append(v.turns, turn{
			msg:      ollama.ChatMessage{Role: ollama.RoleAssistant, Content: content},
			model:    v.model,
			meta:     meta,
			render:   v.renderBlock(v.assistantHeaderRow(v.model, meta), content),
			thinking: v.thinkingText,
			tools:    v.streamTools,
		})
		v, recCmd = v.enqueueSessionTurn("assistant", v.model, content, meta, time.Now())
		v.streamText = ""
		v.thinkingText = "" // N6: the turn's extras committed with it
		v.streamTools = nil
		v.resetStreamRender() // the active block committed; nothing cached to show
	}

	switch {
	case v.stopRequest:
		v.notice = "stopped" // esc asked to stop, even if the stream just finished
	case m.Err != "":
		// The error body can come from the remote host; sanitize before the
		// statusline renders it (H-05).
		v.chatErr = sanitizeTerminalText(m.Err)
	default:
		v.notice = ""
	}
	v.stopRequest = false
	v.follow = true
	return v, recCmd
}
