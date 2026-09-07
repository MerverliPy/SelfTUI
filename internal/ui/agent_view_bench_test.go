package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

// D4 (PLAN §11 owner decision): measure the per-frame cost of the chat
// transcript at ~100× a typical session size. chatLines rebuilds the full
// line list from every cached turn on each frame (agent_view.go), while
// renderChatPane only windows O(visible) rows out of it — these benchmarks
// pin that per-frame cost so future windowing work has a baseline to beat.
//
// Baseline: a "typical" v1 session is taken as 20 turns (10 user + 10
// assistant, each assistant block a dozen-plus rendered rows). 100× is
// therefore 2,000 turns. Turn bodies cycle a small set of pre-rendered
// markdown documents (the commit path caches glamour output per turn, so
// per-frame cost depends on line counts, not on body uniqueness); setup
// renders each distinct body once and copies the cached strings, mirroring
// production's render cache.

// benchAgentView builds a committed AgentView at the canonical test geometry
// (88×40) with models loaded, ready for transcript seeding.
func benchAgentView(tb testing.TB) AgentView {
	tb.Helper()
	cfg := config.Default()
	v := NewAgentView(ollama.New("http://localhost:1", ""), NewStyles("dark"), "dark", "", cfg.Agent)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	return v
}

// benchBodies returns the distinct markdown documents cycled into seeded
// assistant turns (realistic mix: prose, list, fenced code).
func benchBodies() []string {
	return []string{
		fmt.Sprintf("Here is the summary of the change:\n\n- point one with some prose\n- point two, slightly longer\n- point three\n\nAnd a closing paragraph with `%s` inline code.\n", "make check"),
		"```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\nThe snippet above shows the entry point.\n",
		strings.Repeat("A paragraph of ordinary prose for width wrapping. ", 8) + "\n\n" + strings.Repeat("Second wrapped paragraph. ", 6),
		"Short reply.\n",
	}
}

// seedBenchTranscript appends turnPairs user/assistant pairs whose render
// cache is pre-filled exactly as the commit path would leave it.
func seedBenchTranscript(tb testing.TB, v *AgentView, turnPairs int) {
	tb.Helper()
	bodies := benchBodies()
	rendered := make([]string, len(bodies))
	for i, body := range bodies {
		rendered[i] = v.renderBlock(v.assistantHeaderRow("qwen3:8b", "0.4s · stop"), body)
	}
	for i := 0; i < turnPairs; i++ {
		body := bodies[i%len(bodies)]
		v.turns = append(v.turns,
			turn{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: fmt.Sprintf("user question %d", i)},
				render: v.renderBlock(v.userHeader(), fmt.Sprintf("user question %d", i))},
			turn{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: body},
				model: "qwen3:8b", meta: "0.4s · stop", render: rendered[i%len(rendered)]},
		)
	}
}

// benchChatH mirrors AgentView.View's chat-pane height arithmetic at the
// current geometry so the benchmark windows the same row count production
// renders per frame.
func benchChatH(v AgentView) int {
	return maxInt(v.h-2-v.composerRows()-3-1, 1)
}

// BenchmarkChatPane100x measures renderChatPane per frame with a 2,000-turn
// transcript (100× a 20-turn session) at the 88×40 test geometry.
func BenchmarkChatPane100x(b *testing.B) {
	b.Run("tail", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000) // 2,000 turns total
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.renderChatPane(benchChatH(v))
		}
	})
	b.Run("scrolled-up", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000)
		v.follow = false
		v.scroll = 2000 // deep in the history, window arithmetic still O(visible)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.renderChatPane(benchChatH(v))
		}
	})
	b.Run("streaming", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000)
		v.streamText = strings.Repeat("streaming delta ", 12)
		v.streaming = true
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.renderChatPane(benchChatH(v))
		}
	})
}

// BenchmarkChatLines100x isolates the O(total cached lines) rebuild that
// chatLines performs every frame — the specific hotspot named in the D4
// owner decision.
func BenchmarkChatLines100x(b *testing.B) {
	v := benchAgentView(b)
	seedBenchTranscript(b, &v, 1000) // 2,000 turns total
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.chatLines()
	}
}
