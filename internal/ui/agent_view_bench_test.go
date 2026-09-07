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

// BenchmarkStreamingFrame100x pins the N2 streaming repaint cost. Production
// cadence after N2: token deltas queue; each repaint tick (60 ms) flushes the
// batch, re-primes the active-block cache (one glamour render), and the frame
// reads that cache; token frames between ticks hit the warm cache with zero
// glamour work. Sub-benchmarks:
//
//   - tick-render-short: one tick on a ~200-char stream — directly comparable
//     to the pinned ChatPane100x/streaming baseline (which re-rendered the
//     block three times per token frame).
//   - tick-render-4k: one tick late in a long stream — shows the per-tick
//     O(stream) glamour cost the batch cadence bounds to ≤ ~17 Hz.
//   - token-frame-cached-4k: a per-token frame between ticks — cache warm.
//
// Gate: tick-render paths must beat the pinned naive ChatLines100x baseline
// and must not regress the pinned ChatPane100x/streaming number; the cached
// token frame must sit near the window-only ChatWindow100x/tail cost.
func BenchmarkStreamingFrame100x(b *testing.B) {
	run := func(b *testing.B, streamLen int, perTick bool) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000) // 2,000 turns total
		v.streaming = true
		v.streamText = strings.Repeat("streaming delta ", streamLen/16)
		h := benchChatH(v)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if perTick {
				// One tick's content change at fixed stream length: mutate the
				// tail in place so the doc size (and thus the O(doc) glamour
				// cost) stays exactly at streamLen across iterations.
				v.streamText = v.streamText[:len(v.streamText)-1] + string(rune('a'+i%26))
				v.primeStreamRender() // the tick's cache re-prime
			}
			_ = v.renderChatPane(h) // the frame (View cost)
		}
	}
	b.Run("tick-render-short", func(b *testing.B) { run(b, 192, true) })
	b.Run("tick-render-4k", func(b *testing.B) { run(b, 4096, true) })
	// Pre-N2 comparison: per-token frames with no cache (three full glamour
	// renders per frame — what ChatPane100x/streaming pins at 192 chars),
	// measured at 4k so the O(stream) growth the tick cadence removes stays
	// visible.
	b.Run("naive-frame-4k", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000)
		v.streaming = true
		v.streamText = strings.Repeat("streaming delta ", 4096/16)
		h := benchChatH(v)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.renderChatPane(h) // cache invalid: falls back to three fresh renders
		}
	})
	b.Run("token-frame-cached-4k", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000)
		v.streaming = true
		v.streamText = strings.Repeat("streaming delta ", 4096/16)
		v.primeStreamRender()
		h := benchChatH(v)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.renderChatPane(h) // content unchanged: cache hit, no glamour
		}
	})
}

// BenchmarkChatLines100x pins the pre-N1 O(total) rebuild for historical
// comparison (Pinned baseline 2026-09-07, i7-9700K/go1.27.1: ≈0.75 ms/op,
// 1.39 MB/op, 2 018 allocs/op). Production no longer runs this path per
// frame — chatLines is now the O(total) convenience view over the windowed
// machinery; keep the benchmark so regressions in the naive path stay
// visible and the N1 gate (window must beat this) stays measurable.
func BenchmarkChatLines100x(b *testing.B) {
	v := benchAgentView(b)
	seedBenchTranscript(b, &v, 1000) // 2,000 turns total
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.chatLines()
	}
}

// BenchmarkChatWindow100x isolates the N1 render window: count by newline
// arithmetic (O(turns), zero allocs) and materialize only the O(visible)
// rows renderChatPane actually shows — the per-frame production path. The
// N1 gate: this must beat the pinned ChatLines100x baseline above.
func BenchmarkChatWindow100x(b *testing.B) {
	b.Run("tail", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000) // 2,000 turns total
		total := v.chatLineCount()
		contentH := benchChatH(v) - 2
		start := maxInt(0, total-contentH)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.chatWindowTotal(start, total, total)
		}
	})
	b.Run("count-only", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.chatLineCount()
		}
	})
	b.Run("scrolled-up", func(b *testing.B) {
		v := benchAgentView(b)
		seedBenchTranscript(b, &v, 1000)
		total := v.chatLineCount()
		contentH := benchChatH(v) - 2
		end := total - 2000 // deep in the history
		start := maxInt(0, end-contentH)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = v.chatWindowTotal(start, end, total)
		}
	})
}
