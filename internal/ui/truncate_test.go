package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"selftui/internal/agent"
)

// truncate helpers: cut text is visibly truncated ("…") and styled text is
// never split mid-ANSI.

func TestTruncateToWidthAddsEllipsis(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := truncateToWidth(long, 30)
	if w := lipgloss.Width(got); w > 30 {
		t.Errorf("width = %d, want ≤ 30", w)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("cut text must show an ellipsis, got %q", got)
	}
	if !strings.HasPrefix(got, "aaa") {
		t.Errorf("content head lost: %q", got)
	}
}

func TestTruncateToWidthFitsPassThrough(t *testing.T) {
	s := "exactly-thirty-columns-0"
	if lipgloss.Width(s) != 24 {
		t.Fatalf("fixture width = %d, want 24", lipgloss.Width(s))
	}
	if got := truncateToWidth(s, 24); got != s {
		t.Errorf("fitting text changed: %q", got)
	}
	if got := truncateToWidth(s, 80); got != s {
		t.Errorf("text under the cap changed: %q", got)
	}
}

func TestTruncateToWidthStyledTextKeepsAnsi(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("abcdefghij")
	got := truncateToWidth(styled, 5)
	if w := lipgloss.Width(got); w > 5 {
		t.Errorf("width = %d, want ≤ 5", w)
	}
	if !strings.HasSuffix(stripANSI(got), "…") {
		t.Errorf("styled cut text must show an ellipsis: %q", stripANSI(got))
	}
	// The retained prefix must still carry its style (an ANSI open sequence
	// survived the cut) and render cleanly.
	if !strings.HasPrefix(stripANSI(got), "abc") {
		t.Errorf("styled prefix lost: %q", stripANSI(got))
	}
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("style codes were stripped by truncation: %q", got)
	}
}

func TestTruncateToWidthWideRunes(t *testing.T) {
	s := "中中中中中中中中" // 8 × 2-col cells
	got := truncateToWidth(s, 7)
	if w := lipgloss.Width(got); w > 7 {
		t.Errorf("width = %d, want ≤ 7", w)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("wide-rune cut should still show an ellipsis: %q", got)
	}
	if got := truncateToWidth("hello", 1); got != "…" {
		t.Errorf("maxW=1 should collapse to the ellipsis, got %q", got)
	}
}

// Tool/thinking phases stream no text, so the transcript must not flash an
// empty assistant header with a caret (the statusline carries that state).
func TestNoCaretDuringToolOnlyStreaming(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	v.streaming = true
	v.toolStatus = "⚙ write_file {\"path\":\"timer.sh\"}"
	v.streamText = ""
	if out := stripANSI(v.View()); strings.Contains(out, "▍") {
		t.Errorf("caret must not appear before any text streams:\n%s", out)
	}

	// Once real text arrives the caret rides it; gone again at rest.
	v, _ = v.Update(agent.TokenMsg{Text: "#!/bin/sh"})
	tickStream(t, &v) // N2: the delta renders at the repaint tick
	if out := stripANSI(v.View()); !strings.Contains(out, "▍") || !strings.Contains(out, "#!/bin/sh") {
		t.Errorf("caret missing while text streams:\n%s", out)
	}
	v.streaming = false
	if out := stripANSI(v.View()); strings.Contains(out, "▍") {
		t.Errorf("caret must disappear at rest:\n%s", out)
	}
}
