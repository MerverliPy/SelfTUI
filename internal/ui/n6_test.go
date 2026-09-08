package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
)

// drainExpansion runs the send path's deferred attachment-expansion command
// and feeds its landing message back through Update (the Bubble Tea runtime
// executes commands automatically; tests drive it by hand). A send without
// @-references carries no expansion command.
func drainExpansion(t *testing.T, v *AgentView, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("send returned no command")
	}
	*v, _ = v.Update(cmd())
}

// N6 (PLAN.md §12): /details + /thinking display toggles (default off =
// byte-identical frames), the @-file picker over the jailed workspace, and
// @-reference expansion through the jailed read on send. No leader key was
// added — the slash menu and palette stay the discoverable paths.

func TestAgentViewSlashToggles(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "/details")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.showDetails {
		t.Error("/details should turn the tool-output blocks on")
	}
	if !strings.Contains(v.notice, "tool-output blocks on") {
		t.Errorf("notice = %q", v.notice)
	}
	// Toggling twice returns to off.
	v.notice = ""
	typeText(t, &v, "/details")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.showDetails {
		t.Error("a second /details must toggle the blocks off again")
	}
	if !strings.Contains(v.notice, "tool-output blocks off") {
		t.Errorf("notice = %q", v.notice)
	}

	typeText(t, &v, "/thinking")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.showThinking {
		t.Error("/thinking should turn the reasoning blocks on")
	}
	typeText(t, &v, "/thinking")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.showThinking {
		t.Error("a second /thinking must toggle the blocks off again")
	}

	// Both commands are reachable through the menu and the help reference.
	typeText(t, &v, "/")
	out := stripANSI(v.View())
	for _, want := range []string{"/details", "/thinking"} {
		if !strings.Contains(out, want) {
			t.Errorf("menu missing %q:\n%s", want, out)
		}
	}
	typeText(t, &v, "help")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	out = stripANSI(v.View())
	if !strings.Contains(out, "/details") || !strings.Contains(out, "/thinking") {
		t.Errorf("help overlay missing the N6 toggles:\n%s", out)
	}
}

func TestAgentViewToggleRenderingGating(t *testing.T) {
	build := func(extras bool) AgentView {
		v := testAgent(t, nil)
		v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
		v.turns = append(v.turns, turn{render: "answer text"}, turn{
			render:   "final answer",
			thinking: "step one\nstep two",
			tools:    []string{"⚙ read_file a.txt", "✓ read_file: alpha"},
		})
		return v
	}
	// Byte-identical default: stored extras with the toggles off render
	// exactly like turns without extras.
	without := build(false)
	with := build(true)
	if got, want := stripANSI(with.View()), stripANSI(without.View()); got != want {
		t.Errorf("toggles off must render byte-identically to the pre-N6 shape:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	with.showThinking = true
	out := stripANSI(with.View())
	if !strings.Contains(out, "· reasoning") || !strings.Contains(out, "step two") {
		t.Errorf("/thinking on must surface the reasoning block:\n%s", out)
	}
	if strings.Contains(out, "⚙ read_file a.txt") {
		t.Errorf("/details still off must keep tool lines hidden:\n%s", out)
	}

	with.showDetails = true
	out = stripANSI(with.View())
	if !strings.Contains(out, "✓ read_file: alpha") {
		t.Errorf("/details on must surface the tool lines:\n%s", out)
	}

	// Toggling back off restores the default frame byte-for-byte.
	with.showThinking = false
	with.showDetails = false
	if got, want := stripANSI(with.View()), stripANSI(without.View()); got != want {
		t.Errorf("toggles off again must restore the default frame exactly")
	}
}

// TestAgentViewThinkingAndToolEventFlow drives the live update path: the
// runner's ThinkingMsg deltas batch into the repaint tick (never touching
// streamText), tool events collect their /details lines, and both commit
// with the turn.
func TestAgentViewThinkingAndToolEventFlow(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.model = "qwen3:8b"

	v.streaming = true
	v, _ = v.Update(agent.ThinkingMsg{Text: "think "})
	v, _ = v.Update(agent.ThinkingMsg{Text: "hard"})
	if v.pendingThinking != "think hard" || v.thinkingText != "" {
		t.Fatalf("deltas must batch: pending=%q thinking=%q", v.pendingThinking, v.thinkingText)
	}
	v, _ = v.Update(agentEventMsg{msg: streamTickMsg{}})
	if v.thinkingText != "think hard" || v.pendingThinking != "" {
		t.Fatalf("tick must merge thinking: %q / %q", v.thinkingText, v.pendingThinking)
	}
	if v.streamText != "" {
		t.Errorf("thinking must never enter streamText, got %q", v.streamText)
	}

	v, _ = v.Update(agentEventMsg{msg: agent.TokenMsg{Text: "answer"}})
	v, _ = v.Update(agentEventMsg{msg: streamTickMsg{}})
	if v.streamText != "answer" {
		t.Fatalf("content stream polluted: %q", v.streamText)
	}

	v, _ = v.Update(agentEventMsg{msg: agent.ToolStartMsg{Name: "read_file", Input: `{"path":"a.txt"}`}})
	v, _ = v.Update(agentEventMsg{msg: agent.ToolResultMsg{Name: "read_file", OK: true, Summary: "alpha"}})
	if len(v.streamTools) != 2 {
		t.Fatalf("tool lines = %v, want one per event", v.streamTools)
	}

	// Both toggles off: the live frame shows neither block.
	out := stripANSI(v.View())
	if strings.Contains(out, "· reasoning") || strings.Contains(out, "⚙ read_file") {
		t.Errorf("toggles off must keep the live frame clean:\n%s", out)
	}
	v.showThinking = true
	v.showDetails = true
	out = stripANSI(v.View())
	if !strings.Contains(out, "· reasoning") || !strings.Contains(out, "⚙ read_file") {
		t.Errorf("toggles on must surface the live blocks:\n%s", out)
	}

	// The done event commits the extras with the turn.
	v, _ = v.Update(agent.AgentDoneMsg{Reason: "stop"})
	if v.streaming {
		t.Fatal("turn should have committed")
	}
	if len(v.turns) == 0 || v.turns[len(v.turns)-1].thinking != "think hard" {
		t.Errorf("committed turn lost its thinking: %+v", v.turns)
	}
	if len(v.turns) == 0 || len(v.turns[len(v.turns)-1].tools) != 2 {
		t.Errorf("committed turn lost its tool lines: %+v", v.turns)
	}
}

func TestAgentViewFilePicker(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	root := t.TempDir()
	for _, rel := range []string{"a.txt", "b.md", "src/app.go"} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	v.runner = agent.NewRunner(v.client, root, "", 4)

	// A fresh '@' arms the picker and starts the off-loop walk.
	typeText(t, &v, "@")
	if !v.fileOpen || !v.fileLoading {
		t.Fatal("'@' should arm the picker and start the listing")
	}
	v, _ = v.Update(fileListMsg{files: agent.WorkspaceFiles(v.ctx, root)})
	if v.fileLoading || len(v.fileList) != 3 {
		t.Fatalf("listing = %v (loading=%v)", v.fileList, v.fileLoading)
	}
	out := stripANSI(v.View())
	for _, want := range []string{"a.txt", "b.md", "src/app.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("picker missing %q:\n%s", want, out)
		}
	}

	// Typing narrows the list (the draft is the filter).
	typeText(t, &v, "app")
	if m := v.fileMatches(); len(m) != 1 || m[0] != "src/app.go" {
		t.Fatalf("filter matches = %v", m)
	}
	// enter attaches in place and disarms.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.fileOpen {
		t.Error("picking must disarm the picker")
	}
	if got := v.input.Value(); got != "@src/app.go " {
		t.Fatalf("input after pick = %q, want %q", got, "@src/app.go ")
	}

	// esc disarms and keeps the typed query in the draft.
	typeText(t, &v, "@b")
	if !v.fileMenuActive() {
		t.Fatal("'@b' should re-arm the picker")
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.fileOpen {
		t.Error("esc must disarm the picker")
	}
	if got := v.input.Value(); !strings.HasSuffix(got, "@b") {
		t.Errorf("esc must keep the draft, got %q", got)
	}

	// A word end (space) closes the trigger; the picker stays closed.
	typeText(t, &v, "@a.txt")
	if !v.fileMenuActive() {
		t.Fatal("'@a.txt' should re-arm the picker")
	}
	typeText(t, &v, " ")
	if v.fileOpen {
		t.Error("a space after the query must close the picker")
	}

	// Deleting the '@' closes it too.
	typeText(t, &v, "@x")
	if !v.fileMenuActive() {
		t.Fatal("'@x' should re-arm the picker")
	}
	for i := 0; i < 2; i++ {
		v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	if v.fileOpen {
		t.Error("deleting past the '@' must close the picker")
	}
}

// TestAgentViewAttachSend pins the send-path contract: the transcript shows
// the draft, the wire content carries the jailed inline attachment, and the
// context meter budgets the expanded payload.
func TestAgentViewAttachSend(t *testing.T) {
	v := testAgent(t, nil)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	v.runner = agent.NewRunner(v.client, root, "", 4)
	v.model = "qwen3:8b"

	typeText(t, &v, "see @a.txt")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainExpansion(t, &v, cmd)
	drainChat(t, &v)

	if len(v.turns) == 0 {
		t.Fatal("no turn committed")
	}
	turn := v.turns[0]
	if turn.msg.Content != "see @a.txt" {
		t.Errorf("display content = %q, want the draft", turn.msg.Content)
	}
	want := "see @a.txt\n\n[file: a.txt]\nalpha"
	if turn.wire != want {
		t.Errorf("wire = %q, want %q", turn.wire, want)
	}
	// The meter mirrors the expanded payload.
	msgs := v.payloadMessages()
	if len(msgs) != 1 || msgs[0].Content != want {
		t.Errorf("payload = %+v, want the expanded content", msgs)
	}
}

// TestAgentViewAttachSendJailed pins the jail on the send path: a draft
// referencing a path outside the workspace attaches a visible refusal note,
// never content, and the displayed draft is unchanged.
func TestAgentViewAttachSendJailed(t *testing.T) {
	v := testAgent(t, nil)
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside-secret")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("classified"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	v.runner = agent.NewRunner(v.client, root, "", 4)
	v.model = "qwen3:8b"

	typeText(t, &v, "open @../outside-secret/secret")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainExpansion(t, &v, cmd)
	drainChat(t, &v)

	if len(v.turns) == 0 {
		t.Fatal("no turn committed")
	}
	if got := v.turns[0].msg.Content; got != "open @../outside-secret/secret" {
		t.Errorf("display content = %q, want the draft", got)
	}
	wire := v.turns[0].wire
	if !strings.Contains(wire, "unavailable") || !strings.Contains(wire, "escapes workspace") {
		t.Errorf("wire = %q, want a visible jail refusal", wire)
	}
	if strings.Contains(wire, "classified") {
		t.Error("content outside the workspace must never be attached")
	}
}

// TestAgentViewAttachMeterWarnsOnRemoteHost pins the N6 safety surface: when
// the host is remote and @-references expanded, the user learns the workspace
// content went off-machine (tools being off does not make it local).
func TestAgentViewAttachMeterWarnsOnRemoteHost(t *testing.T) {
	v := testAgent(t, nil)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	v.runner = agent.NewRunner(v.client, root, "", 4)
	v.model = "qwen3:8b"
	v.host = "http://192.168.1.50:11434"

	typeText(t, &v, "see @a.txt")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainExpansion(t, &v, cmd)
	drainChat(t, &v)
	if !strings.Contains(v.notice, "attached workspace content sent to") {
		t.Errorf("notice = %q, want the remote-attachment warning", v.notice)
	}
}

// TestAgentViewThinkingFlushAndSanitize pins the reasoning-path fixes: a
// ThinkingMsg that races the done event (no repaint tick in between) still
// flushes into the committed turn, and remote reasoning is sanitized at the
// merge so no control bytes are ever stored or rendered.
func TestAgentViewThinkingFlushAndSanitize(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.model = "qwen3:8b"
	v.streaming = true
	v.showThinking = true

	v, _ = v.Update(agent.ThinkingMsg{Text: "answering\x1b]52;c;evil\x07 now"})
	v, _ = v.Update(agent.TokenMsg{Text: "answer"})
	// Done arrives inside the same repaint interval: no tick ran.
	v, _ = v.Update(agent.AgentDoneMsg{Reason: "stop"})
	if v.streaming {
		t.Fatal("turn should have committed")
	}
	if len(v.turns) == 0 {
		t.Fatal("no turn committed")
	}
	th := v.turns[len(v.turns)-1].thinking
	if !strings.Contains(th, "answering") || !strings.Contains(th, "now") {
		t.Errorf("pending reasoning lost on the done path: %q", th)
	}
	if strings.ContainsAny(th, "\x1b\x07") {
		t.Errorf("reasoning stored with control bytes: %q", th)
	}
}

// TestAgentViewToolRowsMatchCount pins the /details window contract: each
// stored element is one display row, so a multi-line tool summary must be
// stored as one row per line or the live window undercounts.
func TestAgentViewToolRowsMatchCount(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.showDetails = true
	v.streaming = true
	v, _ = v.Update(agent.ToolResultMsg{Name: "read_file", OK: true, Summary: "line one\nline two\nline three"})
	if len(v.streamTools) != 3 {
		t.Fatalf("streamTools = %v, want one row per summary line", v.streamTools)
	}
	if got, want := v.streamLineCount(), len(v.streamDisplayLines()); got != want {
		t.Errorf("streamLineCount = %d, display lines = %d — the live window would overflow", got, want)
	}
}

// TestAgentViewPickerHighlightAndSpaces pins the picker fixes: enter picks
// the highlighted row (not the first match), sensitive paths are never
// offered, and a path with a space is inserted in its escaped "\\ " form —
// which the send path then expands into the real file.
func TestAgentViewPickerHighlightAndSpaces(t *testing.T) {
	v := testAgent(t, nil)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"a.txt", "b.md", "docs/design notes.md"} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	v.runner = agent.NewRunner(v.client, root, "", 4)
	v.model = "qwen3:8b"

	typeText(t, &v, "@")
	v, _ = v.Update(fileListMsg{files: agent.WorkspaceFiles(v.ctx, root)})
	if len(v.fileList) != 3 {
		t.Fatalf("listing = %v, want the sensitive path filtered out", v.fileList)
	}

	// Down once highlights the second ranked row; enter must pick that row
	// (the pre-fix bug always inserted matches[0], i.e. "@b.md ").
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := v.input.Value(); got != "@a.txt " {
		t.Fatalf("input after pick = %q, want the highlighted row inserted", got)
	}

	// The space path is offered and picked in its escaped form. Typing the
	// new "@" re-arms the picker and re-walks the workspace, so the test
	// feeds the fresh listing just like the runtime would.
	typeText(t, &v, "see @design")
	v, _ = v.Update(fileListMsg{files: agent.WorkspaceFiles(v.ctx, root)})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := v.input.Value(); got != "@a.txt see @docs/design\\ notes.md " {
		t.Fatalf("input after pick = %q, want the escaped space path", got)
	}

	// Sending expands the escaped reference into the real file.
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainExpansion(t, &v, cmd)
	drainChat(t, &v)
	if len(v.turns) == 0 {
		t.Fatal("no turn committed")
	}
	wire := v.turns[0].wire
	if !strings.Contains(wire, "[file: docs/design notes.md]") {
		t.Errorf("wire = %q, want the space path attached", wire)
	}
}

// TestAgentViewAttachSensitiveRefused pins the send-side policy gate: a
// hand-typed reference to a credential file expands to a visible policy
// note, never to the file's content.
func TestAgentViewAttachSensitiveRefused(t *testing.T) {
	v := testAgent(t, nil)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	v.runner = agent.NewRunner(v.client, root, "", 4)
	v.model = "qwen3:8b"

	typeText(t, &v, "load @.env")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainExpansion(t, &v, cmd)
	drainChat(t, &v)

	if len(v.turns) == 0 {
		t.Fatal("no turn committed")
	}
	wire := v.turns[0].wire
	if !strings.Contains(wire, "unavailable") || !strings.Contains(wire, "tool policy") {
		t.Errorf("wire = %q, want a policy refusal note", wire)
	}
	if strings.Contains(wire, "SECRET=1") {
		t.Error("credential content must never be attached")
	}
}
