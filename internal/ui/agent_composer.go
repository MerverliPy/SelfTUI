package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/config"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// --- keys -----------------------------------------------------------------

func (v AgentView) handleKey(msg tea.KeyMsg) (AgentView, tea.Cmd) {
	if _, isPress := msg.(tea.KeyPressMsg); !isPress {
		return v, nil
	}
	k := msg.Key()

	// Hard modals own every key: a pending mutation approval, the model
	// selector, the /clear confirmation, and the /help overlay.
	if v.confirmation != nil {
		return v.confirmKey(k)
	}
	if v.selectorOpen {
		return v.selectorKey(k)
	}
	if v.clearConfirm {
		return v.clearConfirmKey(k)
	}
	if v.resumeConfirm {
		return v.resumeConfirmKey(k)
	}
	if v.resumeOpen {
		return v.resumeKey(k)
	}
	if v.helpOpen {
		if k.Code == tea.KeyEsc || k.Code == tea.KeyEnter || k.Text == "x" {
			v.helpOpen = false
		}
		return v, nil
	}

	// While a slash draft is showing, the draft's keys steer the menu
	// (slashDraftKey); every other key keeps editing the draft below.
	// The @-file picker (N6) sits one priority above it: while armed, its
	// nav keys steer the file list and every other key keeps editing the
	// draft (which is also the live filter).
	if v.fileMenuActive() {
		av, handled, cmd := v.filePickerKey(k)
		if handled {
			return av, cmd
		}
		v = av
	}
	if v.slashMenu() {
		av, handled, cmd := v.slashDraftKey(k)
		if handled {
			return av, cmd
		}
		v = av
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
		switch {
		case v.streaming && v.stopCancel != nil:
			// Armed interrupt (opencode-style): the first esc while running
			// only warns — the statusline flips to "esc again to interrupt" —
			// and only the second cancels, so a stray esc can't kill a long
			// generation.
			if v.stopArmed {
				v.stopRequest = true
				v.stopCancel()
			} else {
				v.stopArmed = true
			}
		case v.input.Value() != "":
			// Idle with a drafted prompt: esc clears it (M7-A). A second esc
			// on an already-empty input is a no-op.
			ti := v.input
			ti.Reset()
			v.input = ti
			v.slashQuery = ""
			v.slashIdx = 0
			v.fileOpen = false // N6: a cleared draft disarms the @-picker
			return v.fitComposer(), nil
		}
		return v, nil
	case (k.Code == tea.KeyPgUp || k.Code == tea.KeyPgDown) && v.input.Value() == "" && !v.streaming:
		return v.pageScroll(k.Code == tea.KeyPgUp)
	case k.Text == "f" && v.input.Value() == "" && !v.streaming:
		// Auto-follow toggle (M7-B): off lets pgup/u scroll away; on snaps
		// back to the live tail.
		if v.follow {
			v.follow = false
		} else {
			v.follow = true
			v.scroll = 0
		}
		return v, nil
	case k.Text == "m" && v.input.Value() == "" && !v.streaming:
		// The letter commands (m/r/u/d/f) only fire while the input is empty,
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
		v.clampScroll()
		if v.scroll <= 0 {
			v.follow = true // reaching the tail re-engages auto-follow
		}
		return v, nil
	}

	ta, cmd := v.input.Update(msg)
	v.input = ta
	// N6: a fresh "@" arms the file picker (typing, not streaming — the
	// slash menu follows the same rule). Every other edit re-checks the
	// trigger word so the picker closes itself when the '@' or the query is
	// edited away.
	if k.Text == "@" && !v.streaming && !v.fileOpen {
		v.fileOpen = true
		v.fileLoading = true
		v.fileList = nil
		v.fileIdx = 0
		v.fileFilter = ""
		return v.fitComposer(), v.listFilesCmd()
	}
	if v.fileOpen {
		if _, _, ok := v.atQuery(); !ok {
			v.fileOpen = false
		}
	}
	return v.fitComposer(), cmd
}

// confirmKey owns the mutation-approval dialog's keys: y or enter approves,
// n or esc declines, and any other key is swallowed (the dialog has no
// default affirmative key). The runner is unblocked either way.
func (v AgentView) confirmKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Text == "y" || k.Code == tea.KeyEnter:
		v.confirmation.Respond(true)
		v.notice = "approved " + v.confirmation.Name
		v.confirmation = nil
	case k.Text == "n" || k.Code == tea.KeyEsc:
		v.confirmation.Respond(false)
		v.notice = "declined " + v.confirmation.Name
		v.confirmation = nil
	}
	return v, nil
}

// slashDraftKey owns the composer keys while the "/" command menu is
// showing: arrows steer the highlighted row, enter runs it, and esc drops
// the whole draft. Every other key keeps editing the draft (handled=false),
// so the menu filters live as characters land (backspace too).
func (v AgentView) slashDraftKey(k tea.Key) (AgentView, bool, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		ti := v.input
		ti.Reset()
		v.input = ti
		v.slashQuery = ""
		v.slashIdx = 0
		return v.fitComposer(), true, nil
	case k.Code == tea.KeyEnter && !k.Mod.Contains(tea.ModShift):
		av, cmd := v.runSlashCommand()
		return av, true, cmd
	case k.Code == tea.KeyUp:
		if v.slashIdx > 0 {
			v.slashIdx--
		}
		return v, true, nil
	case k.Code == tea.KeyDown:
		if n := len(v.slashMatches()); v.slashIdx < n-1 {
			v.slashIdx++
		}
		return v, true, nil
	}
	// Draft text changed: reset the highlight to the top row unless the
	// filter still selects the same command (keep it simple: top row).
	if q := v.slashQueryOf(); q != v.slashQuery {
		v.slashQuery = q
		v.slashIdx = 0
	}
	return v, false, nil
}

// fitComposer re-sizes the prompt textarea to the current width and content
// height. The composer grows from one to composerMaxRows rows as a
// multi-line prompt is typed (opencode-style auto-grow); sizing happens on
// every text change, geometry change, and reset so wrapping and the pane
// height stay correct.
func (v AgentView) fitComposer() AgentView {
	ta := v.input
	ta.SetWidth(maxInt(v.w-2, 10))
	ta.SetHeight(composerRowsFor(ta.Value(), maxInt(v.w-2, 10)))
	v.input = ta
	return v
}

// composerMaxRows caps the auto-growing composer prompt.
const composerMaxRows = 4

// composerRowsFor counts how many terminal rows a prompt occupies in a
// textarea taW columns wide: one per physical line plus extra rows for
// wrapping (the prompt glyph shortens the first line).
func composerRowsFor(value string, taW int) int {
	if taW < 1 {
		taW = 1
	}
	lines := strings.Split(value, "\n")
	n := 0
	for j, l := range lines {
		w := lipgloss.Width(l)
		if w == 0 {
			n++
			continue
		}
		usable := taW
		if j == 0 {
			usable = maxInt(taW-2, 1) // the "❯ " prompt shares the first row
		}
		n += 1 + (w-1)/usable
	}
	return clampInt(n, 1, composerMaxRows)
}

// pageScroll moves the transcript window one visible page (pgup up, pgdn
// down). Reaching the tail on the way down re-engages auto-follow.
func (v AgentView) pageScroll(up bool) (AgentView, tea.Cmd) {
	page := maxInt(1, v.pageHeight())
	if up {
		v.scroll += page
		v.follow = false
	} else {
		v.scroll -= page
		if v.scroll <= 0 {
			v.scroll = 0
			v.follow = true
			return v, nil
		}
		v.follow = false
	}
	v.clampScroll()
	return v, nil
}

// pageHeight is the number of visible transcript rows used as the pgup/pgdn
// page size — the same accounting renderChatPane uses for its window
// (composer height is dynamic: it grows with the prompt).
func (v AgentView) pageHeight() int {
	bodyH := maxInt(v.h-2, 1)
	chatH := maxInt(bodyH-v.composerRows()-3-1, 1)
	return maxInt(chatH-2, 1)
}

// sendInput appends the typed text as a user message and starts a stream.
func (v AgentView) sendInput() (AgentView, tea.Cmd) {
	text := strings.TrimSpace(v.input.Value())
	if text == "" {
		return v, nil
	}
	if v.resumePending {
		// A transcript load is in flight and will replace the conversation:
		// refuse the send (the draft stays in the input) instead of letting
		// the import destroy it on landing.
		v.notice = "resume in progress — the transcript is still loading"
		return v, nil
	}
	if v.model == "" {
		v.notice = "no model selected — press m or pull one in the Models tab"
		return v, nil
	}
	if v.expandPending {
		// The previous send's @-references are still expanding off the update
		// loop; a second send would race the first turn's start.
		v.notice = "attachments are still expanding — try again in a moment"
		return v, nil
	}
	i := v.input
	i.Reset()
	v.input = i
	v.fileOpen = false
	v = v.fitComposer()
	v.measuredPromptTokens = 0 // a new draft/send: budget check is approximate again

	// N6: expand @-references through the jailed read_file. The transcript
	// renders the draft; the wire content carries the inline file blocks.
	// The reads are bounded by the read_file ceiling (maxReadBytes per file).
	// When the draft carries references the expansion runs in a command —
	// one filesystem read per token must never block the update loop (a
	// slow/network filesystem or a referenced FIFO would freeze repaint and
	// even Esc) — and beginTurn starts the turn when it lands.
	if refs := agent.FileRefTokens(text); len(refs) > 0 {
		root := v.runner.Root()
		ctx := v.ctx
		v.expandPending = true
		return v, func() tea.Msg {
			return attachExpandedMsg{draft: text, wire: agent.ExpandFileRefs(ctx, root, text)}
		}
	}
	return v.beginTurn(text, "")
}

// beginTurn commits the user turn and starts the chat. It is the landing
// path for the deferred attachment expansion (attachExpandedMsg) and the
// direct path when the draft carries no @-references. The remote-attachment
// warning rides after startChat (which resets the notice for the new turn)
// so it survives to the post-turn statusline.
func (v AgentView) beginTurn(text, wire string) (AgentView, tea.Cmd) {
	v.expandPending = false
	if v.resumePending {
		// A transcript load raced the (now async) expansion: refuse the send
		// and put the draft back — same rule as sendInput — instead of
		// letting the import destroy it on landing.
		ta := v.input
		ta.SetValue(text)
		v.input = ta
		v.notice = "resume in progress — the transcript is still loading"
		return v, nil
	}
	warn := ""
	if wire != "" && wire != text && v.host != "" && !config.LoopbackHost(v.host) {
		warn = "⚠ attached workspace content sent to " + v.host
	}
	v.turns = append(v.turns, turn{
		msg:    ollama.ChatMessage{Role: ollama.RoleUser, Content: text},
		model:  v.model,
		render: v.renderBlock(v.userHeader(), text),
		wire:   wire,
	})
	v.checkContextBudget()
	v, recCmd := v.enqueueSessionTurn("user", v.model, text, "", time.Now())
	av, chatCmd := v.startChat()
	// The remote-attachment warning rides after startChat (which resets the
	// notice for the new turn) so it survives to the post-turn statusline.
	if warn != "" {
		av.notice = warn
	}
	return av, tea.Batch(recCmd, chatCmd)
}
