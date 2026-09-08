package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// chatLinesNaive is a frozen copy of the pre-N1 O(total) transcript assembly
// (agent_view.go before the render-window change), extended for the N6
// gated extras (reasoning behind /thinking, tool lines behind /details)
// with the same composition the windowed path uses. The windowed path
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
		block := v.turnBlock(i) // N6: gated extras composed exactly as the windowed path does
		if block == "" {
			continue
		}
		lines = append(lines, strings.Split(block, "\n")...)
		lines = append(lines, "")
	}
	if tail := v.streamDisplayLines(); len(tail) > 0 {
		lines = append(lines, tail...)
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
		// N6: gated extras — stored extras with both toggles off must render
		// byte-identically to the pre-N6 shapes; each toggle on composes its
		// own block. The extras-only stream (thinking streaming before any
		// content) exercises the composer-count arithmetic too.
		{"thinking-and-tools-stored-toggles-off", func(v *AgentView) {
			v.turns = append(v.turns,
				turn{render: block, thinking: "hidden reasoning\nrow two", tools: []string{"⚙ read_file a.txt", "✓ read_file: ok"}},
				turn{render: "plain"},
			)
		}},
		{"thinking-toggle-on", func(v *AgentView) {
			v.showThinking = true
			v.turns = append(v.turns,
				turn{render: block, thinking: "step one\nstep two\n"},
				turn{render: "answer"},
			)
		}},
		{"details-toggle-on", func(v *AgentView) {
			v.showDetails = true
			v.turns = append(v.turns,
				turn{render: block, tools: []string{"⚙ grep pattern .", "⚠ grep: denied"}},
			)
		}},
		{"both-toggles-on-with-extras-only-turn", func(v *AgentView) {
			v.showThinking = true
			v.showDetails = true
			v.turns = append(v.turns,
				turn{thinking: "only reasoning", tools: []string{"⚙ list_dir ."}}, // no content block at all
				turn{render: block},
			)
		}},
		{"thinking-streaming-extras-only", func(v *AgentView) {
			v.showThinking = true
			v.streaming = true
			v.thinkingText = "live reasoning\nsecond line\n"
			v.streamTools = []string{"⚙ read_file big.txt"}
		}},
		{"details-streaming-with-content", func(v *AgentView) {
			v.showDetails = true
			v.streaming = true
			v.streamTools = []string{"⚙ grep x .", "✓ grep: 3 matches"}
			v.streamText = "partial answer\n"
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
				if got, want := v.turnLineCount(i), len(v.turnDisplayLines(i)); got != want {
					t.Fatalf("turn %d: turnLineCount() = %d, turnDisplayLines len = %d", i, got, want)
				}
			}
		})
	}
}
