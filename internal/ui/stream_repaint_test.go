package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/agent"
	"selftui/internal/config"
	"selftui/internal/ollama"
)

// N2 — streaming repaint discipline (PLAN §12): token deltas batch to a
// repaint tick, the active block's rendered output is cached per content
// change, committed neighbors stay frozen, and the final chunk flushes
// immediately. These tests pin each of those behaviors.

// tickStream drives one N2 repaint tick through the view exactly as the tea
// runtime would: it executes streamTickCmd and delivers the enveloped tick.
func tickStream(t *testing.T, v *AgentView) {
	t.Helper()
	next, _ := v.Update(streamTickMsg{})
	*v = next
}

// streamingView builds a models-loaded view with an in-flight stream (no
// server: tokens are fed directly, as the M7 tests do).
func streamingView(t *testing.T) AgentView {
	t.Helper()
	cfg := config.Default()
	v := NewAgentView(ollama.New("http://localhost:1", ""), NewStyles("dark"), "dark", "", cfg.Agent)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v.streaming = true
	return v
}

func TestStreamDeltasQueueUntilRepaintTick(t *testing.T) {
	v := streamingView(t)

	for _, seq := range []string{"Hel", "lo ", "wor", "ld"} {
		v, _ = v.Update(agent.TokenMsg{Text: seq})
	}
	if v.streamText != "" {
		t.Errorf("streamText = %q before the tick, want deltas still queued", v.streamText)
	}
	if v.pendingStream != "Hello world" {
		t.Errorf("pendingStream = %q, want the full batch queued", v.pendingStream)
	}
	// The frame itself must not show the queued text: unchanged rendered
	// content means the repaint is a no-op until the tick flushes.
	if out := stripANSI(v.View()); strings.Contains(out, "Hello world") {
		t.Errorf("queued deltas leaked onto the frame before the tick:\n%s", out)
	}

	tickStream(t, &v)
	if v.streamText != "Hello world" || v.pendingStream != "" {
		t.Errorf("after tick: streamText=%q pendingStream=%q, want batch flushed", v.streamText, v.pendingStream)
	}
	if out := stripANSI(v.View()); !strings.Contains(out, "Hello world") {
		t.Errorf("flushed text missing from the frame:\n%s", out)
	}
	if !v.follow {
		t.Error("follow must re-arm when a batch flushes (M7-B)")
	}
}

func TestStreamTickReArmsWhileStreamingOnly(t *testing.T) {
	v := streamingView(t)

	cmd := v.streamTickCmd()
	if cmd == nil {
		t.Fatal("streamTickCmd returned nil")
	}
	// While streaming, a tick re-arms itself.
	v.pendingStream = "x"
	next, cmd := v.Update(streamTickMsg{})
	v = next
	if cmd == nil {
		t.Error("tick while streaming must re-arm the repaint tick")
	}
	// A tick with nothing queued is a no-op merge but still re-arms.
	next, cmd = v.Update(streamTickMsg{})
	v = next
	if cmd == nil {
		t.Error("empty tick while streaming must still re-arm")
	}
	// After the stream ends the tick neither flushes nor re-arms.
	v.streaming = false
	next, cmd = v.Update(streamTickMsg{})
	v = next
	if cmd != nil {
		t.Error("tick after the stream ended must not re-arm")
	}
}

func TestStreamDoneFlushesPendingImmediately(t *testing.T) {
	v := streamingView(t)
	v.turnStart = v.turnStart.Add(-time.Second)

	// Deltas arrive, then the final chunk (done) — no repaint tick in
	// between: the commit must not lose the tail.
	v, _ = v.Update(agent.TokenMsg{Text: "partial "})
	v, _ = v.Update(agent.TokenMsg{Text: "answer"})
	if v.pendingStream == "" {
		t.Fatal("deltas should be queued before done arrives")
	}
	v, _ = v.Update(agent.AgentDoneMsg{Reason: "stop"})

	if v.streaming {
		t.Error("done should end the stream")
	}
	if len(v.turns) == 0 {
		t.Fatal("done should commit the assistant turn")
	}
	last := v.turns[len(v.turns)-1]
	if last.msg.Content != "partial answer" {
		t.Errorf("committed content = %q, want the flushed tail included", last.msg.Content)
	}
	if v.pendingStream != "" || v.streamText != "" {
		t.Errorf("post-commit buffers = stream %q pending %q, want both empty", v.streamText, v.pendingStream)
	}
	if !strings.Contains(last.meta, "stop") {
		t.Errorf("footer meta = %q, want the N3 stop reason riding the commit", last.meta)
	}
}

func TestStreamFrozenNeighborsNotRebuilt(t *testing.T) {
	v := streamingView(t)
	seedBenchTranscript(t, &v, 3)

	// Sentinel-mark every committed render: any rebuild of a frozen turn
	// would overwrite the sentinel and fail. Deltas must never touch them.
	for i := range v.turns {
		v.turns[i].render = "frozen-sentinel-" + strconv.Itoa(i)
	}

	for _, seq := range []string{"one ", "two ", "three ", "four ", "five "} {
		v, _ = v.Update(agent.TokenMsg{Text: seq})
	}
	tickStream(t, &v)
	tickStream(t, &v) // a second batch: neighbors must stay frozen across ticks
	for i := range v.turns {
		if got := v.turns[i].render; !strings.HasPrefix(got, "frozen-sentinel-") {
			t.Errorf("turns[%d].render = %q, want the frozen cached render untouched", i, got)
		}
	}
}

// TestStreamCacheMatchesFreshRender pins the N2 cache to the fresh render
// path: whatever a frame reads (cache hit or miss) must be byte-identical to
// renderBlock, so the N1 equivalence pins and the caret shape are unaffected
// by which path a frame hits.
func TestStreamCacheMatchesFreshRender(t *testing.T) {
	v := streamingView(t)

	steps := []string{"# Title\n\n", "first paragraph ", "with wrap words.\n\n", "- list one\n", "- list two\n\n", "```go\n", "fmt.Println()\n", "```\n", "tail prose"}
	want := ""
	for _, s := range steps {
		v, _ = v.Update(agent.TokenMsg{Text: s})
		tickStream(t, &v) // flush + prime exactly as production would
		want = renderBlockForTest(t, &v)
		if got := v.streamBlockRender(); got != want {
			t.Fatalf("cache render drifted from fresh render after %q", s)
		}
		// Count/display must agree with the frozen naive assembly.
		if got, wantN := v.chatLineCount(), len(chatLinesNaive(v)); got != wantN {
			t.Fatalf("chatLineCount = %d, naive = %d after %q", got, wantN, s)
		}
	}
	_ = want
}

// renderBlockForTest renders the streaming block fresh (the miss path) —
// the reference the cache must match.
func renderBlockForTest(t *testing.T, v *AgentView) string {
	t.Helper()
	return v.renderBlock(v.assistantHeader(v.model), v.streamText)
}

// TestStreamCacheInvalidatesOnGeometryAndTheme covers the cache key: width
// and theme changes must re-prime (or miss) so output follows the renderer.
func TestStreamCacheInvalidatesOnGeometryAndTheme(t *testing.T) {
	v := streamingView(t)

	v, _ = v.Update(agent.TokenMsg{Text: "some streamed prose that will wrap differently at other widths"})
	tickStream(t, &v)

	// Resize: the frame must show the re-rendered block at the new width.
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if got, want := v.chatLineCount(), len(chatLinesNaive(v)); got != want {
		t.Fatalf("after resize: chatLineCount = %d, naive = %d (cache stale)", got, want)
	}
	if got, want := v.streamBlockRender(), v.renderBlock(v.assistantHeader(v.model), v.streamText); got != want {
		t.Fatal("after resize: cached render differs from fresh render")
	}

	// Theme change re-tints the active block too.
	v = v.applyTheme(false, NewStyles("light"))
	if got, want := v.streamBlockRender(), v.renderBlock(v.assistantHeader(v.model), v.streamText); got != want {
		t.Fatal("after theme change: cached render differs from fresh render")
	}
}

// TestStreamPayloadSeesPendingDeltas pins the logic-side read: the ctx
// budget (payloadMessages) must observe deltas still awaiting the repaint
// tick, since the meter renders during a live stream. (The /clear and resume
// guards are unreachable mid-stream — slashMenu gates on !streaming and open
// Resume is slash-only — so their streamText reads stay as-is.)
func TestStreamPayloadSeesPendingDeltas(t *testing.T) {
	v := streamingView(t)
	v.streamText = "committed-so-far"
	v.pendingStream = " plus queued"

	if got := v.liveStreamText(); got != "committed-so-far plus queued" {
		t.Errorf("liveStreamText = %q, want flushed text plus pending", got)
	}

	msgs := v.payloadMessages()
	var streamMsg *ollama.ChatMessage
	for i := range msgs {
		if msgs[i].Role == ollama.RoleAssistant {
			streamMsg = &msgs[i]
		}
	}
	if streamMsg == nil || streamMsg.Content != "committed-so-far plus queued" {
		t.Errorf("payloadMessages = %+v, want the assistant payload to include the queued tail", msgs)
	}
}

// TestStreamCaretShapeThroughBatching pins the M7-B caret UX through the new
// cadence: caret rides the flushed streaming block and vanishes at commit.
func TestStreamCaretShapeThroughBatching(t *testing.T) {
	v := streamingView(t)

	v, _ = v.Update(agent.TokenMsg{Text: "streaming text\n"})
	tickStream(t, &v)
	lines := v.streamDisplayLines()
	if len(lines) == 0 || !strings.HasSuffix(lines[len(lines)-2], "▍") {
		t.Errorf("caret missing from the flushed streaming block: %q", lines)
	}
	if lines[len(lines)-1] != "" {
		t.Errorf("streaming block must end with its separator row: %q", lines)
	}

	// Commit: caret disappears, block freezes into the turn cache.
	v, _ = v.Update(agent.AgentDoneMsg{Reason: "stop"})
	if out := stripANSI(v.View()); strings.Contains(out, "▍") {
		t.Errorf("caret must disappear at commit:\n%s", out)
	}
}
