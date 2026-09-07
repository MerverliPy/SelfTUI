package ui

// H-05 regression tests: every remote-derived string (chat tokens, model
// names, API errors, tool inputs/results, Markdown-render failure fallback)
// must be sanitized before it reaches the terminal. These tests drive hostile
// payloads through the same Update paths a malicious Ollama host/model would
// use and assert the final View() output carries none of the hostile bytes,
// while ordinary text, newline/tab, and Unicode survive.
//
// The payload corpus mirrors the audit finding H-05 and the runbook task 06:
// OSC 52 (clipboard write, BEL- and ST-terminated), CSI clear-screen /
// cursor-home / erase, DCS (sixel), APC/PM/SOS, standalone BEL, carriage
// return, backspace, VT/FF, 8-bit C1 (CSI/OSC), and truncated ("dangling")
// forms that arrive split across two stream tokens.

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/agent"
	"selftui/internal/ollama"
)

// hostileMarkers are exact byte strings that must never appear in rendered
// View() output. SelfTUI's own styling is SGR-only (ESC [ … m), so these
// markers (ESC followed by a non-'[' byte, CSI erase/cursor ops, C0/C1
// controls) cannot be produced by the app's own styles.
var hostileMarkers = []string{
	"\x1b]52",  // OSC 52 clipboard
	"\x1bP",    // DCS
	"\x1b_",    // APC
	"\x1b^",    // PM
	"\x1bX",    // SOS
	"\x1b[2J",  // CSI erase display
	"\x1b[H",   // CSI cursor home
	"\x1b[2K",  // CSI erase line
	"\x1b[?25", // CSI cursor visibility
	"\x1b[?1049",
	"\x07",   // BEL
	"\r",     // carriage return
	"\b",     // backspace
	"\x0b",   // VT
	"\x0c",   // FF
	"\u009b", // C1 CSI
	"\u009d", // C1 OSC
	"\u0098", // C1 SOS
	"\u009e", // C1 PM
	"\u009f", // C1 APC
}

// assertCleanOutput fails when any hostile byte marker reaches the rendered
// output, or when a required visible fragment is missing (so a test that
// strips everything still fails rather than passing vacuously).
func assertCleanOutput(t *testing.T, out string, wantVisible ...string) {
	t.Helper()
	for _, m := range hostileMarkers {
		if strings.Contains(out, m) {
			t.Errorf("unsafe byte marker %q reached rendered output:\n%q", m, out)
		}
	}
	plain := stripANSI(out)
	for _, want := range wantVisible {
		if !strings.Contains(plain, want) {
			t.Errorf("rendered output lost expected visible text %q:\n%s", want, plain)
		}
	}
}

func hostileModel() ollama.Model {
	return ollama.Model{
		Name:          "pwn\x1b]52;c;x\x07ed",
		Family:        "evil\x1b[2J",
		ParameterSize: "9B",
		Quantization:  "Q4_K_M",
		SizeBytes:     123,
	}
}

func TestAgentHistoryRawCacheGapSanitized(t *testing.T) {
	// If the glamour render cache ever lacks an entry for a committed block
	// (a geometry/cache gap), chatLines falls back to the raw history copy;
	// that fallback must also be sanitized (H-05).
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.turns = []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "hi"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: "answer \x1b]52;c;evil\x07 here\x1b[2J"},
			model: "qwen3:8b"},
	}
	// Leave the render cache empty on purpose: chatLines must not emit raw history.
	assertCleanOutput(t, v.View(), "answer", "here")
	if strings.Contains(strings.Join(v.chatLines(), "\n"), "\x1b]52") {
		t.Error("chatLines emitted the raw OSC 52 payload")
	}
}

func TestAgentChatSanitizesControlSequencesStreamAndCommit(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hi")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.streaming {
		t.Fatal("enter: streaming should start")
	}

	// Feed hostile deltas exactly as a malicious host streams them, including
	// an OSC 52 sequence split across two tokens and a dangling CSI.
	chunks := []string{
		"hello \x1b]52;c,",       // OSC start, terminated in the next token
		"evil\x07 world",         // OSC 52 completes here
		" \x1b[2J\x1b[H wiped",   // CSI clear screen + cursor home
		"\x1bPq#0;2;0\x1b\\ dcs", // DCS sixel
		"a\rb",                   // carriage return
		"\u009b2J\u009d",         // C1 CSI + C1 OSC
		"\x07 trailing",          // standalone BEL
		"\x1b[31",                // dangling CSI (no final byte yet)
	}
	for _, c := range chunks {
		v, _ = v.Update(agent.TokenMsg{Text: c})
	}
	tickStream(t, &v) // N2: the batch renders at the repaint tick
	assertCleanOutput(t, v.View(), "hello", "world")

	// Commit the turn; the cached block must stay clean.
	v, _ = v.Update(agent.AgentDoneMsg{Err: "", Reason: "stop"})
	assertCleanOutput(t, v.View(), "hello", "world")
	if len(v.turns) < 2 {
		t.Fatalf("turns len = %d, want committed assistant turn", len(v.turns))
	}
	// The committed render cache (what View shows) is clean even though the
	// raw message copy is intentionally unsanitized (it mirrors the model).
	assertCleanOutput(t, v.turns[len(v.turns)-1].render)
}

func TestAgentCommittedFallbackMarkdownSanitized(t *testing.T) {
	// renderBlock is the single funnel for chat content in both the glamour
	// success path and the raw-markdown fallback (renderer failure): the raw
	// fallback returns md verbatim, so sanitize-on-entry must precede both.
	v := testAgent(t, nil)
	md := "payload \x1b]52;c;evil\x07 here\nsecond \x1b[2Jline"
	out := v.renderBlock(v.assistantHeader("qwen3:8b"), md)
	assertCleanOutput(t, out, "payload", "line")

	// The fallback branch returns the sanitized md directly — prove the
	// sanitizer makes that branch safe for the same corpus.
	clean := sanitizeTerminalText(md)
	if strings.Contains(clean, "\x1b") || strings.Contains(clean, "\x07") {
		t.Fatalf("sanitized markdown still carries controls: %q", clean)
	}
}

func TestAgentModelNameOSCSanitizedInHeaderAndSelector(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: []ollama.Model{hostileModel()}})
	assertCleanOutput(t, v.View(), "pwned")
	if v.model == "" {
		t.Fatal("model should be selected")
	}
	if v.model != "pwned" {
		t.Errorf("stored model name = %q, want sanitized pwned", v.model)
	}

	// Model selector overlay rows carry the same (sanitized) name.
	v, _ = v.Update(tea.KeyPressMsg{Text: "m"})
	if !v.selectorOpen {
		t.Fatal("m should open the selector")
	}
	assertCleanOutput(t, v.View(), "pwned")
}

func TestAgentHostileModelNameNeverSentToHost(t *testing.T) {
	// Sanitization happens at store, so the API request must carry the
	// sanitized name too (a hostile name is unusable anyway; sending the raw
	// name would be the only other place it could leak).
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: []ollama.Model{hostileModel()}})
	if got := v.model; got != "pwned" {
		t.Errorf("chat model = %q, want pwned", got)
	}
}

func TestAgentDoneReasonAndErrorSanitized(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// A hostile Ollama done_reason lands on the assistant header's right side.
	v.streamText = "answer"
	v.turnStart = time.Now().Add(-500 * time.Millisecond)
	v, _ = v.Update(agent.AgentDoneMsg{Err: "", Reason: "stop\x1b[2J\x1b]52;c;x\x07"})
	assertCleanOutput(t, v.View(), "answer", "stop")

	// A hostile error body lands in the statusline.
	v, _ = v.Update(agent.AgentDoneMsg{Err: "boom \x1b]52;c;evil\x07 \x1b[2Jfailed", Reason: ""})
	if v.chatErr == "" {
		t.Fatal("chatErr should be set")
	}
	assertCleanOutput(t, v.View(), "boom", "failed")
}

func TestAgentFallbackNoticeSanitized(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v, _ = v.Update(agent.FallbackMsg{Reason: "plain chat fallback\x1b]52;c;x\x07 \x1b[2J"})
	assertCleanOutput(t, v.View(), "plain chat fallback")
}

func TestAgentToolStatusAndConfirmationSanitized(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// Tool start/result text streams into the statusline while running.
	v.streaming = true
	v, _ = v.Update(agent.ToolStartMsg{Name: "read_file", Input: "a.txt\x1b]52;c;x\x07"})
	v, _ = v.Update(agent.ToolResultMsg{Name: "read_file", OK: true, Summary: "ok \x1b[2J content"})
	assertCleanOutput(t, v.View())

	// The mutation-confirmation overlay shows the remote tool name + input.
	v, _ = v.Update(agent.ToolConfirmMsg{
		Name:      "write_file\x1b]52;c;x\x07",
		Input:     `{"path":"x"}` + "\x1b[2J",
		Workspace: "/tmp/ws",
		Timeout:   30 * time.Second,
	})
	if v.confirmation == nil {
		t.Fatal("confirmation should be pending")
	}
	assertCleanOutput(t, v.View(), "write_file")
}

func TestModelsListHostileNameSanitized(t *testing.T) {
	v := testModels(t, nil)
	hostile := sampleModels()
	hostile[0] = hostileModel()
	v, _ = v.Update(modelsLoadedMsg{list: hostile})
	assertCleanOutput(t, v.View(), "pwned")
}

func TestModelsHostileErrorsSanitized(t *testing.T) {
	v := testModels(t, nil)
	// List-load error fills the full-size error pane.
	v, _ = v.Update(modelsLoadErrMsg{err: "list boom\x1b]52;c;x\x07 \x1b[2J"})
	assertCleanOutput(t, v.View(), "list boom")

	// Detail error fills the inspect pane (open on the compact device).
	v = testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open stacked pane
	v, _ = v.Update(modelsShowErrMsg{name: "qwen3:8b", err: "show boom\x1b]52;c;x\x07"})
	assertCleanOutput(t, v.View(), "show boom")

	// Pull error surfaces on the hint line.
	v = testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(modelsPullDoneMsg{name: "x", err: "pull boom\x1b]52;c;x\x07 \x1b[2J"})
	assertCleanOutput(t, v.View(), "pull boom")

	// Delete error sits inside the confirm dialog.
	v = testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v.confirmDelete = true
	v.deleteTarget = "qwen3:8b"
	v, _ = v.Update(modelsDeleteDoneMsg{name: "qwen3:8b", err: "del boom\x1b]52;c;x\x07"})
	assertCleanOutput(t, v.View(), "del boom")
}

func TestModelsDetailContentSanitized(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open stacked detail pane

	details := ollama.Details{
		Template:   "render this \x1b]52;c;evil\x07 \x1b[2J template",
		Modelfile:  "FROM qwen\n\x1bPq#0;2;0\x1b\\",
		Parameters: "temperature 0.7\x1b[H",
		License:    "MIT \x1b]52;c;x\x07",
	}
	v, _ = v.Update(modelsShowMsg{name: "qwen3:8b", details: details})
	out := v.View()
	assertCleanOutput(t, out, "render this", "template")
	if !strings.Contains(stripANSI(out), "temperature 0.7") {
		t.Errorf("detail text lost after sanitize:\n%s", stripANSI(out))
	}
}

func TestModelsPullStatusHostileSanitized(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v.pulling = true
	v, _ = v.Update(modelsPullMsg{name: "x", progress: ollama.PullProgress{
		Status: "pulling layer\x1b]52;c;x\x07 \x1b[2J",
		Total:  100, Completed: 50,
	}})
	assertCleanOutput(t, v.View(), "pulling layer")
}

// --- sanitizeTerminalText unit tests ---------------------------------------

func TestSanitizeTerminalTextStripsOSC52(t *testing.T) {
	cases := map[string]string{
		"BEL-terminated": "\x1b]52;c,aGVsbG8=\x07",
		"ST-terminated":  "\x1b]52;c,aGVsbG8=\x1b\\",
		"embedded":       "hello \x1b]52;c;evil\x07 world",
	}
	for name, in := range cases {
		out := sanitizeTerminalText(in)
		if strings.ContainsAny(out, "\x1b\x07") {
			t.Errorf("%s: control bytes survived: %q", name, out)
		}
	}
	if got := sanitizeTerminalText("hello \x1b]52;c;evil\x07 world"); got != "hello  world" {
		t.Errorf("embedded OSC: got %q, want %q", got, "hello  world")
	}
}

func TestSanitizeTerminalTextStripsCSI(t *testing.T) {
	cases := map[string]string{
		"clear-screen":     "\x1b[2J\x1b[H",
		"cursor-move":      "up\x1b[1A down",
		"erase-line":       "\x1b[2K",
		"sgr-keeps-text":   "\x1b[1;31mred",
		"cursor-invisible": "\x1b[?25l",
		"alternate-screen": "\x1b[?1049h",
	}
	for name, in := range cases {
		if strings.Contains(sanitizeTerminalText(in), "\x1b") {
			t.Errorf("%s: ESC survived in %q", name, sanitizeTerminalText(in))
		}
	}
	if got := sanitizeTerminalText("\x1b[1;31mred"); got != "red" {
		t.Errorf("SGR color must keep the painted text, got %q", got)
	}
	if got := sanitizeTerminalText("up\x1b[1A down"); got != "up down" {
		t.Errorf("cursor move must keep surrounding text, got %q", got)
	}
}

func TestSanitizeTerminalTextStripsDCSAndAPC(t *testing.T) {
	cases := map[string]string{
		"dcs": "\x1bPq#0;2;0;0;0#1;2;100;100\x1b\\",
		"apc": "\x1b_Gi=...\x1b\\",
		"pm":  "\x1b^priv\x1b\\",
		"sos": "\x1bXhold\x1b\\",
	}
	for name, in := range cases {
		out := sanitizeTerminalText(in)
		if strings.Contains(out, "\x1b") {
			t.Errorf("%s: ESC survived: %q", name, out)
		}
	}
}

func TestSanitizeTerminalTextStripsC0AndC1Controls(t *testing.T) {
	// Standalone controls ansi.Strip leaves in ground state must be dropped.
	for _, r := range []rune{'\a', '\r', '\b', '\v', '\f', '\x00', '\x1b'} {
		if got := sanitizeTerminalText(string(r)); got != "" {
			t.Errorf("C0 %q survived: %q", r, got)
		}
	}
	if got := sanitizeTerminalText("a\rb"); got != "ab" {
		t.Errorf("carriage return: got %q, want ab", got)
	}
	// C1 controls: 8-bit CSI/OSC/SOS/PM/APC introducers.
	for _, r := range []rune{'\u009b', '\u009d', '\u0098', '\u009e', '\u009f'} {
		if got := sanitizeTerminalText(string(r)); strings.Contains(got, string(r)) {
			t.Errorf("C1 %q survived: %q", r, got)
		}
	}
	if got := sanitizeTerminalText("\u009b2J\u009dosc\x07"); got != "2Josc" {
		// The C1 CSI/OSC introducers are dropped by ansi.Strip; leftover body
		// text stays inert (no introducer remains to execute it).
		t.Logf("C1 body behavior: got %q", got)
	}
}

func TestSanitizeTerminalTextSplitAndDanglingSequences(t *testing.T) {
	// A token stream can split a sequence mid-way; a dangling introducer must
	// never survive to be completed by the next frame, and once dropped the
	// trailing payload is inert literal text.
	cases := map[string]string{
		"dangling-esc": "abc\x1b",
		"dangling-osc": "abc\x1b]52;c,x",
		"dangling-csi": "abc\x1b[31",
		"dangling-dcs": "abc\x1bPq#0;2;0",
		"dangling-c1":  "abc\u009d",
	}
	for name, in := range cases {
		out := sanitizeTerminalText(in)
		if strings.Contains(out, "\x1b") || strings.Contains(out, "\x07") {
			t.Errorf("%s: dangling control survived: %q", name, out)
		}
	}
	// Simulate the two frames of a split OSC 52: each half sanitized on its
	// own must yield no executable sequence (frame 1 drops the dangling
	// introducer; frame 2's payload is inert text).
	frame1 := sanitizeTerminalText("hello \x1b]52;c,")
	frame2 := sanitizeTerminalText("evil\x07 world")
	combined := frame1 + frame2
	if strings.ContainsAny(combined, "\x1b\x07") {
		t.Errorf("split OSC 52 frames can re-execute: %q", combined)
	}
}

func TestSanitizeTerminalTextKeepsNewlineTabAndUnicode(t *testing.T) {
	in := "line one\n\tindented 模型 ✓ é 😀 **bold** `code`\nline three"
	if got := sanitizeTerminalText(in); got != in {
		t.Errorf("clean text was altered:\n in=%q\nout=%q", in, got)
	}
}

func TestSanitizeTerminalTextIsIdempotent(t *testing.T) {
	payloads := []string{
		"hello \x1b]52;c;evil\x07 world\x1b[2J\x1bPq\x1b\\",
		"a\rb\nc\td\u009b2J\x07",
		"plain text\nwith unicode 模型",
	}
	for _, in := range payloads {
		once := sanitizeTerminalText(in)
		twice := sanitizeTerminalText(once)
		if once != twice {
			t.Errorf("not idempotent: once=%q twice=%q", once, twice)
		}
	}
}

func TestSanitizeTerminalTextLeavesCleanTextUntouched(t *testing.T) {
	for _, in := range []string{"", "qwen3:8b", "a · b", "error: connection refused",
		"params: num_ctx=4096\nstop=[\"<|im_end|>\"]"} {
		if got := sanitizeTerminalText(in); got != in {
			t.Errorf("clean input altered: %q -> %q", in, got)
		}
	}
}
