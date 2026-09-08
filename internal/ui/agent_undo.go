package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
)

// V2e undo/redo UX (docs/v2e-multifile-undo-design.md §4.3): /undo and /redo
// slash commands + palette entries, each guarded by the standard y/esc
// confirm, with the result surfaced as a status-bar notice. The journal op
// itself is host-side plain Go (never sandboxed) and runs in a tea.Cmd so the
// update loop never blocks on the filesystem (M-04 discipline).
type undoDoneMsg struct {
	action string // "undo" | "redo"
	res    agent.UndoResult
	err    error
}

// beginUndoConfirm opens the y/esc confirm for /undo (or /redo when redo is
// true). Empty-journal cases short-circuit to a notice instead of a dialog.
func (v AgentView) beginUndoConfirm(redo bool) (AgentView, tea.Cmd) {
	action := "undo"
	if redo {
		action = "redo"
	}
	if v.streaming {
		v.notice = "wait for the turn to finish, then " + action
		return v, nil
	}
	j := v.undo
	if j == nil {
		v.notice = "undo is unavailable in this session"
		return v, nil
	}
	if (redo && !j.CanRedo()) || (!redo && !j.CanUndo()) {
		v.notice = "nothing to " + action
		return v, nil
	}
	if redo {
		v.redoConfirm = true
	} else {
		v.undoConfirm = true
	}
	return v, nil
}

// runUndoCmd executes the journal op off the update loop.
func (v AgentView) runUndoCmd(redo bool) tea.Cmd {
	j := v.undo
	if j == nil {
		return nil
	}
	action := "undo"
	if redo {
		action = "redo"
	}
	return func() tea.Msg {
		var res agent.UndoResult
		var err error
		if redo {
			res, err = j.Redo()
		} else {
			res, err = j.Undo()
		}
		return agentEventMsg{msg: undoDoneMsg{action: action, res: res, err: err}}
	}
}

// undoConfirmKey owns the /undo and /redo confirm dialogs: y/enter runs the
// journal op, n/esc cancels — same guard rails as /clear (a stray key changes
// nothing).
func (v AgentView) undoConfirmKey(k tea.Key, redo bool) (AgentView, tea.Cmd) {
	action := "undo"
	if redo {
		action = "redo"
	}
	switch {
	case k.Text == "y" || k.Code == tea.KeyEnter:
		v.undoConfirm = false
		v.redoConfirm = false
		v.notice = action + "…"
		return v, v.runUndoCmd(redo)
	case k.Text == "n" || k.Code == tea.KeyEsc:
		v.undoConfirm = false
		v.redoConfirm = false
		v.notice = action + " cancelled"
	}
	return v, nil
}

// WithUndoDir attaches the crash-artifact directory for this session's undo
// journal (V2e §4.3): entries die with the process, but their fsynced
// pre-image blobs live under dir for crash recovery and are GC'd at clean
// exit (Close). An empty dir keeps the journal memory-only (tests, hosts
// without a state home). Re-wiring replaces the journal and re-attaches it to
// the current runner.
func (v AgentView) WithUndoDir(dir string) AgentView {
	if v.undo != nil {
		v.undo.Close()
	}
	j, err := agent.NewUndoJournal(dir)
	if err != nil {
		v.notice = "undo journal: " + err.Error()
		j, _ = agent.NewUndoJournal("")
	}
	v.undo = j
	if v.runner != nil {
		v.runner = v.runner.WithJournal(j)
	}
	// Crash recovery report (design §4.3): a previous process that died
	// mid-apply left stale entries; they are reported, never auto-reverted.
	if stale := j.Stale(); len(stale) > 0 {
		v.notice = fmt.Sprintf("found %d interrupted undo entries from a previous run — not auto-reverted", len(stale))
	}
	return v
}

// CloseUndo is the clean-exit lifecycle boundary for the undo journal: it
// GCs the crash-artifact blobs (session stacks die with the process anyway).
func (v AgentView) CloseUndo() error {
	if v.undo == nil {
		return nil
	}
	return v.undo.Close()
}

// applyUndoDone lands one completed journal op and turns it into the
// status-bar notice. Refuse-guard refusals and empty-stack results carry the
// journal's own text (sanitized — it can name workspace paths).
func (v *AgentView) applyUndoDone(m undoDoneMsg) {
	if m.err != nil {
		v.notice = sanitizeTerminalText(m.action + ": " + m.err.Error())
		return
	}
	files := m.res.Files
	label := m.action
	if m.res.Action != "" {
		label = m.res.Action
	}
	if len(files) == 0 {
		v.notice = sanitizeTerminalText(label + ": nothing to " + m.action)
		return
	}
	shown := files
	const maxShown = 3
	if len(shown) > maxShown {
		shown = shown[:maxShown]
	}
	names := strings.Join(shown, ", ")
	if len(files) > maxShown {
		names += fmt.Sprintf(" (+%d more)", len(files)-maxShown)
	}
	v.notice = sanitizeTerminalText(label + ": " + names)
}
