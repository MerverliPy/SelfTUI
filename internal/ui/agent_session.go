package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
	"github.com/MerverliPy/SelfTUI/internal/session"
)

// enqueueSessionTurn mirrors one committed turn onto the ordered background
// recorder (constructed once at composition-root wiring in WithSessionDir).
// It takes a pointer receiver so the recorder outcome (accepted turn, first
// failure) lands on the calling view, not a discarded copy. The update loop
// only enqueues immutable work and arms the ack command; Open/Append/Flush/
// Close all happen on the recorder worker, so a slow or stalled transcript
// directory can never block Update (M-04). On the first failure the ack
// disables the log once and one notice tells the user where it failed — a
// transcript is never worth breaking the chat for.
func (v *AgentView) enqueueSessionTurn(role, model, content, meta string, at time.Time) (AgentView, tea.Cmd) {
	if v.sessionDir == "" || v.sessionErr || v.recorder == nil {
		return *v, nil
	}
	// The transcript mirrors what the terminal shows, so committed content
	// is sanitized the same way (the file can otherwise be re-opened in a
	// terminal-paging editor where control bytes would execute).
	done, err := v.recorder.Append(role, model, sanitizeTerminalText(content), meta, at)
	if err != nil {
		// Backlog full: the sink is wedged; recording is over for this run.
		v.sessionErr = true
		v.sessionErrMsg = err.Error()
		v.notice = "session log: " + err.Error()
		return *v, nil
	}
	v.recorded = true
	return *v, func() tea.Msg {
		res := <-done
		if res.Err == nil {
			return nil // a successful append needs no UI round-trip
		}
		return agentEventMsg{msg: sessionAppendMsg{err: res.Err}}
	}
}

// WithSessionDir enables transcript persistence under dir with host recorded
// in the file header (the composition root — main via App — wires it once at
// startup; empty dir disables). The concrete recorder is constructed here,
// not lazily on the update loop: a committed turn only ever enqueues work
// onto the existing dependency. Re-wiring closes the previous recorder first
// (Close is idempotent), so repeated calls never strand a worker.
func (v AgentView) WithSessionDir(dir, host string) AgentView {
	if v.recorder != nil {
		v.recorder.Close()
	}
	v.sessionDir = dir
	v.sessionHost = host
	v.recorder = nil
	v.recorded = false
	v.sessionErr = false
	v.sessionErrMsg = ""
	if dir != "" {
		v.recorder = session.NewRecorder(dir, host)
	}
	return v
}

// CloseRecorder flushes every committed turn and stops the transcript
// recorder's worker. It is the normal-shutdown lifecycle boundary (main calls
// it after the tea program exits); a nil recorder (recording disabled or no
// turn yet) is a no-op, and Close is idempotent.
func (v AgentView) CloseRecorder() error {
	if v.recorder == nil {
		return nil
	}
	return v.recorder.Close()
}

// exportSession flushes the Markdown transcript and reports its path. The
// flush runs on the recorder worker strictly after every earlier enqueued
// turn (ordered jobs), so the reported path is exact and the export is
// append-only and cannot be resumed (chat stays in-memory) — the notice
// reports the file and never claims the conversation can be reloaded. Update
// only enqueues and processes the completion message (M-04).
func (v AgentView) exportSession() (AgentView, tea.Cmd) {
	switch {
	case v.sessionDir == "":
		v.notice = "session recording is off — no transcript is written"
		return v, nil
	case v.sessionErr:
		// Recording failed earlier and is permanently off for this run, so
		// "send a message first" would be dead-end advice: echo the real
		// failure instead (P1-5).
		v.notice = "session recording failed: " + v.sessionErrMsg
		return v, nil
	case v.recorder == nil || !v.recorded:
		// The composition root wires the recorder eagerly; "no recorder" can
		// only mean recording never accepted a turn — same advice either way.
		v.notice = "nothing recorded yet — send a message first"
		return v, nil
	}
	done, err := v.recorder.Flush()
	if err != nil {
		v.sessionErr = true
		v.sessionErrMsg = err.Error()
		v.notice = "session log: " + err.Error()
		return v, nil
	}
	return v, func() tea.Msg {
		res := <-done
		return agentEventMsg{msg: sessionExportMsg{path: res.Path, err: res.Err}}
	}
}

// applySessionExport lands one /export completion on the view. A failure
// disables recording once (same one-error surface as an append failure); an
// empty path means nothing was ever recorded.
func (v AgentView) applySessionExport(m sessionExportMsg) AgentView {
	if m.err != nil {
		v.sessionErr = true
		v.sessionErrMsg = m.err.Error()
		v.notice = "session log: " + m.err.Error()
		return v
	}
	if m.path == "" {
		v.notice = "nothing recorded yet — send a message first"
		return v
	}
	v.notice = "transcript: " + m.path
	return v
}

// --- resume (V2a: reload a saved transcript into the live conversation) ---

// openResume starts the /resume flow: the picker lists the saved transcripts
// under sessionDir. Listing runs in a command (M-04 discipline: no
// transcript filesystem work on the update loop); the notice cases below
// mirror /export's (recording off, nothing saved yet).
func (v AgentView) openResume() (AgentView, tea.Cmd) {
	switch {
	case v.sessionDir == "":
		v.notice = "session recording is off — nothing to resume"
		return v, nil
	case v.streaming:
		return v, nil // one turn at a time; /resume while running is a no-op
	}
	v.resumeOpen = true
	v.resumeLoading = true
	v.resumeList = nil
	v.resumeIdx = 0
	return v, v.listSessionsCmd()
}

// listSessionsCmd reads the saved-transcript listing off the update loop.
func (v AgentView) listSessionsCmd() tea.Cmd {
	dir := v.sessionDir
	return func() tea.Msg {
		list, err := session.ListSessions(dir)
		return agentEventMsg{msg: sessionListMsg{list: list, err: err}}
	}
}

// applySessionList lands the picker listing. A listing failure closes the
// picker and surfaces one notice — the same once-and-continue posture as
// the recorder (the chat never blocks on the disk).
func (v AgentView) applySessionList(m sessionListMsg) AgentView {
	v.resumeLoading = false
	if m.err != nil {
		v.resumeOpen = false
		v.notice = "resume: " + m.err.Error()
		return v
	}
	v.resumeList = m.list
	v.resumeIdx = 0
	if len(m.list) == 0 {
		v.resumeOpen = false
		v.notice = "no saved sessions"
	}
	return v
}

// resumeKey owns the picker's keys (same shape as the model selector: esc
// close, enter pick, j/k or arrows move). Picking with a live conversation
// asks first — resuming replaces the in-memory history.
func (v AgentView) resumeKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		v.resumeOpen = false
		v.resumeList = nil
		v.resumeIdx = 0
	case k.Code == tea.KeyEnter:
		if len(v.resumeList) > 0 && v.resumeIdx >= 0 && v.resumeIdx < len(v.resumeList) {
			path := v.resumeList[v.resumeIdx].Path
			if len(v.turns) > 0 || v.streamText != "" {
				v.resumePick = path
				v.resumeConfirm = true
				return v, nil
			}
			return v.importSession(path)
		}
	case k.Text == "j" || k.Code == tea.KeyDown:
		if v.resumeIdx < len(v.resumeList)-1 {
			v.resumeIdx++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if v.resumeIdx > 0 {
			v.resumeIdx--
		}
	}
	return v, nil
}

// resumeConfirmKey handles y/enter (proceed) vs n/esc (cancel) in the
// overwrite dialog, mirroring the /clear confirmation.
func (v AgentView) resumeConfirmKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Text == "y" || k.Code == tea.KeyEnter:
		v.resumeConfirm = false
		path := v.resumePick
		v.resumePick = ""
		return v.importSession(path)
	case k.Text == "n" || k.Code == tea.KeyEsc:
		// Cancel backs out of the whole flow — dialog and picker — so the
		// next keypress reaches the composer instead of a hidden list.
		v.resumeConfirm = false
		v.resumeOpen = false
		v.resumeList = nil
		v.resumeIdx = 0
		v.resumePick = ""
		v.notice = "resume cancelled"
	}
	return v, nil
}

// importSession starts the parse of one chosen transcript. Parsing runs in
// a command; applySessionLoaded performs the actual import once the turns
// arrive. resumePending blocks sends for the whole window, so the import
// can never race a turn the user sends mid-load into oblivion.
func (v AgentView) importSession(path string) (AgentView, tea.Cmd) {
	v.resumeOpen = false
	v.resumeList = nil
	v.resumeIdx = 0
	v.resumePending = true
	return v, func() tea.Msg {
		turns, err := session.Load(path)
		return agentEventMsg{msg: sessionLoadedMsg{path: path, turns: turns, err: err}}
	}
}

// applySessionLoaded imports parsed turns into the live conversation.
//
// Safe-import semantics (V2a): transcripts only ever record committed
// user/assistant turns — tool-call activity lives on the statusline and is
// never written — so an import can never fabricate tool state. The imported
// turns enter as plain history for the next runner request. The truncation
// flag recomputes here over system prompt + imported history (mirroring a
// normal commit), so an over-budget transcript shows the truncation marker
// immediately instead of only after the next send. Imported model/meta
// strings and the notice's file name are file-derived display text and are
// sanitized like any other remote-derived string before they enter state.
// The next send keeps the currently configured/selected model — the
// picker's rows show historical names only and never switch the active
// model. The import replaces the in-memory conversation only; the new
// run's transcript file stays append-only and records just the turns sent
// after the resume.
func (v AgentView) applySessionLoaded(m sessionLoadedMsg) AgentView {
	// The load window is over in every outcome: unblock sends whether the
	// import succeeded, failed, or was superseded.
	v.resumePending = false
	if m.err != nil {
		v.notice = "resume failed: " + m.err.Error()
		return v
	}
	v.turns = v.turns[:0]
	for _, t := range m.turns {
		role := ollama.RoleUser
		if t.Role == "assistant" {
			role = ollama.RoleAssistant
		}
		// Imported metadata is file-derived text: sanitize it before it
		// enters state or the render cache (same boundary as streamed
		// tokens and model names).
		model := sanitizeTerminalText(t.Model)
		meta := sanitizeTerminalText(t.Meta)
		header := v.userHeader()
		if role == ollama.RoleAssistant {
			header = v.assistantHeaderRow(model, meta)
		}
		v.turns = append(v.turns, turn{
			msg:    ollama.ChatMessage{Role: role, Content: t.Content},
			model:  model,
			meta:   meta,
			render: v.renderBlock(header, t.Content),
		})
	}
	// Budget recompute on import (mirrors a normal commit's
	// checkContextBudget): an over-budget transcript must surface the
	// truncation marker now, not only after the next send.
	v.truncated = false
	v.measuredPromptTokens = 0 // the conversation was replaced wholesale
	v.lastTokPerSec = 0        // its last turn's rate no longer describes this one
	v.checkContextBudget()
	v.scroll = 0
	v.follow = true
	name := m.path
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = sanitizeTerminalText(name)
	v.notice = fmt.Sprintf("resumed %d turns from %s", len(m.turns), name)
	return v
}
