package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/ollama"
)

// Small-terminal geometry (phase 7): below 40 columns or 12 rows the shell
// cannot lay out its chrome, so the App switches to a deterministic bounded
// "terminal too small" message naming the current dimensions and the 40x12
// minimum. At or above the boundary the normal shell renders. Every frame —
// message or shell — must stay inside the terminal: no row wider than the
// window, no view taller than the window, no panic.

func assertFrameBounded(t *testing.T, name string, w, h int, got string) {
	t.Helper()
	rows := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if w > 0 && len(rows) > maxInt(h, 1) {
		t.Errorf("%s %dx%d: view is %d rows tall (terminal %d) — vertical overflow",
			name, w, h, len(rows), h)
	}
	for i, l := range rows {
		if lw := lipgloss.Width(l); w > 0 && lw > w {
			t.Errorf("%s %dx%d: row %d is %d columns wide (terminal %d): %q",
				name, w, h, i+1, lw, w, stripANSI(l))
		}
	}
	if strings.TrimSpace(stripANSI(got)) == "" {
		t.Errorf("%s %dx%d: frame rendered empty", name, w, h)
	}
}

// TestSmallTerminalBoundaryState drives a fresh shell at each boundary
// geometry. Below the minimum the view must be the bounded small-terminal
// message; at and above it the normal shell must render.
func TestSmallTerminalBoundaryState(t *testing.T) {
	cases := []struct {
		name      string
		w, h      int
		wantSmall bool
	}{
		{"below-min-width", 39, 12, true},
		{"below-min-height", 40, 11, true},
		{"narrow-and-short", 39, 11, true},
		{"narrow-tall", 39, 100, true},
		{"short-wide", 200, 11, true},
		{"one-column", 1, 30, true},
		{"one-row", 80, 1, true},
		{"tiny", 1, 1, true},
		{"exact-minimum", 40, 12, false},
		{"min-plus-column", 41, 12, false},
		{"min-plus-row", 40, 13, false},
		{"canonical-compact", 72, 30, false},
		{"wide-pc", 120, 40, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestApp(t)
			m = updateTab(t, m, tea.WindowSizeMsg{Width: c.w, Height: c.h})
			if got := m.tooSmall(); got != c.wantSmall {
				t.Fatalf("tooSmall() = %v, want %v at %dx%d", got, c.wantSmall, c.w, c.h)
			}
			got := view(t, m)
			assertFrameBounded(t, c.name, c.w, c.h, got)
			if c.wantSmall {
				// Content assertions need room to render: at 1 column or 1 row
				// only the bounded placeholder survives (assertFrameBounded
				// above already proved it does not overflow).
				if c.w >= 20 && c.h >= 4 {
					for _, want := range []string{"terminal too small", "40x12"} {
						if !strings.Contains(got, want) {
							t.Errorf("small message missing %q:\n%s", want, got)
						}
					}
					if !strings.Contains(got, fmt.Sprintf("%dx%d", c.w, c.h)) {
						t.Errorf("small message missing the current dimensions %dx%d:\n%s", c.w, c.h, got)
					}
				}
				return
			}
			for _, label := range tabLabels {
				if !strings.Contains(got, label) {
					t.Errorf("normal shell missing tab %q:\n%s", label, got)
				}
			}
		})
	}
}

// TestZeroSizeFrameIsNotSmallTerminal: a 0x0 frame (no pty size negotiated
// yet, M0a edge note) is stored harmlessly and is not treated as too small.
func TestZeroSizeFrameIsNotSmallTerminal(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 0, Height: 0})
	if m.tooSmall() {
		t.Fatal("tooSmall() = true for a zero-size frame (no size negotiated yet)")
	}
}

// seedUnicodeAgentTurn loads the agent tab and a committed assistant turn
// whose rendered lines carry representative wide Unicode — CJK, box drawing,
// block shading, an emoji and a combining mark — pre-wrapped to the chat
// pane's inner width so the frame's width accounting is what is under test.
func seedUnicodeAgentTurn(t *testing.T, m App) App {
	t.Helper()
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"}) // Agent tab
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	inner := m.w - 2
	raw := []string{
		"你好世界 ▍▓▒░ ✓ café 🚀",               // CJK + shading + emoji
		"┌─┐ box ─── │ 终端 │ ✓",             // box drawing + CJK
		"combining e\u0301 and 漢字 text",    // combining accent + CJK
		strings.Repeat("x", inner-2) + "你", // exactly inner cells + one wide rune
	}
	// Width-check every seeded line against the pane's inner width: the test
	// content itself must be a valid render (glamour normally wraps here).
	for _, l := range raw {
		if lw := lipgloss.Width(l); lw > inner {
			t.Fatalf("seed line %q is %d cells, inner pane %d", l, lw, inner)
		}
	}
	m.agent.turns = []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "render unicode"},
			render: m.agent.renderBlock(m.agent.userHeader(), "render unicode")},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: strings.Join(raw, "\n")},
			model: "qwen3:8b", meta: "0.4s · stop",
			render: m.agent.renderBlock(m.agent.assistantHeaderRow("qwen3:8b", "0.4s · stop"),
				strings.Join(raw, "\n"))},
	}
	return m
}

// seedZeroSizeTranscript loads a committed transcript (pairs of user and
// assistant turns, render cache filled exactly as the commit path leaves it)
// into the agent tab. Used by the 0×0 boundedness test to prove a large
// history cannot leak into a zero-size frame.
func seedZeroSizeTranscript(t *testing.T, v *AgentView, turnPairs int) {
	t.Helper()
	for i := 0; i < turnPairs; i++ {
		body := fmt.Sprintf("assistant line one\nassistant line two (%d)", i)
		v.turns = append(v.turns,
			turn{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "user turn"},
				render: v.renderBlock(v.userHeader(), "user turn")},
			turn{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: body},
				model:  "qwen3:8b",
				render: v.renderBlock(v.assistantHeaderRow("qwen3:8b", ""), body)},
		)
	}
}

// TestZeroSizeAgentFrameStaysBounded pins the conclave finding that 0×0
// renders chrome by design: with a substantial transcript and a live stream
// armed, the zero-size frame must stay a small, deterministic chrome shape —
// the chat pane contributes nothing (renderChatPane refuses h<2) and the
// transcript content cannot leak into the frame regardless of history size.
func TestZeroSizeAgentFrameStaysBounded(t *testing.T) {
	frame := func(t *testing.T, turnPairs int, streaming bool) string {
		t.Helper()
		m := newTestApp(t)
		m = updateTab(t, m, tea.WindowSizeMsg{Width: 0, Height: 0})
		m = updateTab(t, m, tea.KeyPressMsg{Text: "2"}) // Agent tab
		m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
		seedZeroSizeTranscript(t, &m.agent, turnPairs)
		if streaming {
			m.agent.streamText = "streaming now"
			m.agent.streaming = true
		}
		return stripANSI(m.View().Content)
	}

	got := frame(t, 200, true) // 400 turns + live stream at 0×0
	rows := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(rows) > 10 {
		t.Errorf("0×0 agent frame is %d rows — transcript broke frame boundedness:\n%s", len(rows), got)
	}
	for i, r := range rows {
		if w := lipgloss.Width(r); w > 40 {
			t.Errorf("0×0 agent frame row %d is %d columns wide (chrome budget 40): %q", i+1, w, r)
		}
	}
	// Model chip strings (e.g. "qwen3:8b") legitimately appear in the
	// composer chrome, so only transcript content is asserted absent here.
	for _, leak := range []string{"user turn", "assistant line", "streaming now", "❯ you"} {
		if strings.Contains(got, leak) {
			t.Errorf("0×0 agent frame leaked transcript content %q:\n%s", leak, got)
		}
	}

	// History size must not affect the frame: same state, empty transcript.
	if empty := frame(t, 0, true); empty != got {
		t.Errorf("0×0 frame differs between empty and 400-turn transcript (history leaked into layout):\nempty:\n%s\nseeded:\n%s", empty, got)
	}

	// The pane guard itself: renderChatPane refuses sub-2-row heights, while
	// the underlying chatLines computation stays safe for the full history.
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 0, Height: 0})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	seedZeroSizeTranscript(t, &m.agent, 200)
	for _, h := range []int{0, 1} {
		if out := m.agent.renderChatPane(h); out != "" {
			t.Errorf("renderChatPane(%d) = %d bytes, want \"\" (h<2 guard)", h, len(out))
		}
	}
	if lines := m.agent.chatLines(); len(lines) == 0 {
		t.Error("chatLines returned no lines for a 400-turn transcript (computation must stay safe even when the pane hides it)")
	}
}

// TestSmallTerminalUnicodeContent renders the populated Agent tab with wide
// Unicode content at the boundary geometries. Above the minimum the shell
// must render the content inside the frame (no row overflow); below it the
// bounded message replaces the shell entirely.
func TestSmallTerminalUnicodeContent(t *testing.T) {
	cases := []struct {
		name  string
		w, h  int
		small bool
	}{
		{"min-exact-40x12", 40, 12, false},
		{"min-plus-41x12", 41, 12, false},
		{"min-plus-40x13", 40, 13, false},
		{"canonical-compact-72x30", 72, 30, false},
		{"below-min-39x12", 39, 12, true},
		{"below-min-40x11", 40, 11, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestApp(t)
			m = updateTab(t, m, tea.WindowSizeMsg{Width: c.w, Height: c.h})
			m = seedUnicodeAgentTurn(t, m)
			if c.small {
				got := view(t, m)
				assertFrameBounded(t, c.name, c.w, c.h, got)
				if !strings.Contains(got, "terminal too small") || !strings.Contains(got, "40x12") {
					t.Errorf("expected the small-terminal message, got:\n%s", got)
				}
				return
			}
			got := view(t, m)
			assertFrameBounded(t, c.name, c.w, c.h, got)
			if !strings.Contains(stripANSI(got), "你") {
				t.Errorf("unicode content missing from the frame:\n%s", got)
			}
			rows := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
			if len(rows) > c.h {
				t.Errorf("view is %d rows tall (terminal %d)", len(rows), c.h)
			}
		})
	}
}
