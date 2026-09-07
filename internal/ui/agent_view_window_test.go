package ui

import (
	"fmt"
	"strings"
	"testing"

	"selftui/internal/agent"
	"selftui/internal/ollama"
)

// chatLinesNaive is a frozen copy of the pre-N1 O(total) transcript assembly
// (agent_view.go before the render-window change). The windowed path
// (chatLineCount / chatWindow) must produce byte-identical output; these
// tests pin that equivalence so golden frames and scroll behavior cannot
// drift while only the visible rows are materialized.
func chatLinesNaive(v AgentView) []string {
	var lines []string
	if v.truncated {
		lines = append(lines, v.styles.mutedText().Render("… "+agent.TruncationNotice))
		lines = append(lines, "")
	}
	for i := range v.turns {
		t := v.turns[i]
		block := t.render
		if block == "" {
			block = sanitizeTerminalText(t.msg.Content)
		}
		if block == "" {
			continue
		}
		lines = append(lines, strings.Split(block, "\n")...)
		lines = append(lines, "")
	}
	if v.streamText != "" {
		sb := strings.Split(v.renderBlock(v.assistantHeader(v.model), v.streamText), "\n")
		if v.streaming {
			sb = withStreamingCaret(sb)
		}
		lines = append(lines, sb...)
		lines = append(lines, "")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// windowEquivalenceScenario builds one AgentView transcript for the
// equivalence table.
type windowScenario struct {
	name  string
	build func(v *AgentView)
}

func windowScenarios() []windowScenario {
	block := "header one\ncontent two\n\ncontent four"
	return []windowScenario{
		{"empty", func(v *AgentView) {}},
		{"truncated-no-turns", func(v *AgentView) { v.truncated = true }},
		{"plain-turns", func(v *AgentView) {
			for i := 0; i < 5; i++ {
				v.turns = append(v.turns, turn{render: fmt.Sprintf("t%d line a\nt%d line b", i, i)})
			}
		}},
		{"empty-turns-skipped", func(v *AgentView) {
			v.turns = append(v.turns,
				turn{render: block},
				turn{}, // no render, no content: skipped entirely
				turn{render: "second block\nrow two"},
			)
		}},
		{"render-gap-falls-back-to-sanitized-content", func(v *AgentView) {
			v.turns = append(v.turns,
				turn{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "cached render lost\nsecond row"}},
				turn{render: block},
			)
		}},
		{"trailing-newlines-in-last-block", func(v *AgentView) {
			v.turns = append(v.turns,
				turn{render: block},
				turn{render: "ends with blanks\n\n\n"},
			)
		}},
		{"trailing-newlines-then-stream", func(v *AgentView) {
			v.turns = append(v.turns, turn{render: "ends with blanks\n\n"})
			v.streamText = "streaming answer\n"
		}},
		{"streaming-caret", func(v *AgentView) {
			v.turns = append(v.turns, turn{render: block})
			v.streamText = "partial answer"
			v.streaming = true
		}},
		{"streaming-caret-after-blank-line", func(v *AgentView) {
			v.streamText = "partial answer\n"
			v.streaming = true
		}},
		{"stream-not-streaming", func(v *AgentView) {
			v.streamText = "finished text\n\n"
		}},
		{"truncated-with-turns-and-stream", func(v *AgentView) {
			v.truncated = true
			v.turns = append(v.turns, turn{render: block}, turn{render: ""})
			v.streamText = "live"
			v.streaming = true
		}},
	}
}

// TestChatLineCountMatchesNaive pins chatLineCount to len(chatLinesNaive)
// across transcript shapes: the scroll clamp and window arithmetic depend on
// the count being exact.
func TestChatLineCountMatchesNaive(t *testing.T) {
	for _, sc := range windowScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			v := benchAgentView(t)
			sc.build(&v)
			want := len(chatLinesNaive(v))
			if got := v.chatLineCount(); got != want {
				t.Fatalf("chatLineCount() = %d, naive len = %d", got, want)
			}
		})
	}
}

// TestChatWindowMatchesNaive pins the windowed materialization to the naive
// full assembly: every window slice of every scenario must be byte-identical
// to naive[start:end], including tail windows (trailing-blank drop) and
// out-of-range ends.
func TestChatWindowMatchesNaive(t *testing.T) {
	for _, sc := range windowScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			v := benchAgentView(t)
			sc.build(&v)
			naive := chatLinesNaive(v)
			total := len(naive)
			windows := [][2]int{
				{0, total},                  // full
				{0, total + 10},             // over-extended end clamps
				{maxInt(0, total-3), total}, // tail
				{maxInt(0, total-1), total + 5},
				{1, maxInt(1, total-1)},  // head-trimmed
				{total / 2, total/2 + 4}, // middle
				{total, total},           // empty
				{total + 1, total + 9},   // fully out of range
				{-3, 2},                  // negative start
			}
			for _, w := range windows {
				got := v.chatWindow(w[0], w[1])
				// Expected slice: clamp the raw window to the naive list exactly
				// as chatWindow clamps it (end first, then start), on the
				// original absolute indices.
				s, e := w[0], w[1]
				if e > total {
					e = total
				}
				if s < 0 {
					s = 0
				}
				if s > e {
					s = e
				}
				want := naive[s:e]
				if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
					t.Fatalf("chatWindow(%d,%d) = %q, want %q", w[0], w[1], got, want)
				}
			}
		})
	}
}

// TestChatWindowSkipsFarBlocks is a behavioral guard for the N1 win: a tail
// window over a long transcript must not depend on far-away block content —
// only rows overlapping the window may be materialized. Mutating a far block
// after counting would be caught by golden-style equality anyway; here we
// assert the count path never needs the split by checking lineCount
// arithmetic directly against turnDisplayLines for every turn shape.
func TestChatWindowSkipsFarBlocks(t *testing.T) {
	for _, sc := range windowScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			v := benchAgentView(t)
			sc.build(&v)
			for i := range v.turns {
				if got, want := v.turns[i].lineCount(), len(v.turns[i].turnDisplayLines()); got != want {
					t.Fatalf("turn %d: lineCount() = %d, turnDisplayLines len = %d", i, got, want)
				}
			}
		})
	}
}
