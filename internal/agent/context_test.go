package agent

// Context-budget edge tests (M6 — context-truncation edges). BudgetMessages
// must never send the model more than ~3/4 of num_ctx regardless of turn
// shape: a brand-new chat whose first message is huge, conversations without
// older turns to drop, tool-call argument bloat, and repeated calls (the
// runner re-budgets the same history every iteration).

import (
	"strings"
	"testing"

	"selftui/internal/ollama"
)

func sysMsg(content string) ollama.ChatMessage {
	return ollama.ChatMessage{Role: ollama.RoleSystem, Content: content}
}
func userMsg(content string) ollama.ChatMessage {
	return ollama.ChatMessage{Role: ollama.RoleUser, Content: content}
}
func asstMsg(content string, args ...string) ollama.ChatMessage {
	var calls []ollama.ToolCall
	for _, a := range args {
		calls = append(calls, ollama.ToolCall{
			Function: ollama.ToolCallFunction{Name: "read_file", Arguments: []byte(a)},
		})
	}
	return ollama.ChatMessage{Role: ollama.RoleAssistant, Content: content, ToolCalls: calls}
}

func TestBudgetUnderLimitUnchanged(t *testing.T) {
	in := []ollama.ChatMessage{sysMsg("be brief"), userMsg("hello"), asstMsg("hi")}
	got := BudgetMessages(in, 4096)
	if len(got) != len(in) {
		t.Fatalf("len = %d, want %d", len(got), len(in))
	}
	for i, m := range got {
		if m.Role != in[i].Role || m.Content != in[i].Content {
			t.Errorf("message %d changed under budget: %+v", i, m)
		}
	}
	if contents := strings.Join(messageContents(got), " "); strings.Contains(contents, "Earlier conversation omitted") {
		t.Errorf("marker inserted although under budget")
	}
}

func TestBudgetZeroOrNegativeNumCtxUnchanged(t *testing.T) {
	in := []ollama.ChatMessage{sysMsg("s"), userMsg(strings.Repeat("x", 10000))}
	for _, n := range []int{0, -1} {
		got := BudgetMessages(in, n)
		if len(got) != 2 || got[1].Content != in[1].Content {
			t.Errorf("numCtx=%d should leave the conversation untouched", n)
		}
	}
}

// TestBudgetSingleHugeTurnBounded: the very first message of a chat can be a
// giant paste; with no older turn to drop the list must still be bounded by
// truncating the overflowing latest message — never sent raw past the budget.
func TestBudgetSingleHugeTurnBounded(t *testing.T) {
	in := []ollama.ChatMessage{sysMsg("short system"), userMsg(strings.Repeat("x", 20000))}
	got := BudgetMessages(in, 64) // limit = 48 tokens ≈ 192 chars
	if got[0].Role != in[0].Role || got[0].Content != in[0].Content {
		t.Errorf("system prompt mutated: %+v", got[0])
	}
	if last := got[len(got)-1]; !strings.HasPrefix(last.Content, "[truncated] ") {
		t.Fatalf("last message not truncated: len=%d prefix=%q", len(last.Content), last.Content[:20])
	}
	if n := approximateTokens(got); n > 64 {
		t.Errorf("output still over numCtx: %d tokens", n)
	}
}

func TestBudgetSingleTurnWithoutSystem(t *testing.T) {
	in := []ollama.ChatMessage{userMsg(strings.Repeat("y", 20000))}
	got := BudgetMessages(in, 64)
	if len(got) != 1 || !strings.HasPrefix(got[0].Content, "[truncated] ") {
		t.Fatalf("lone huge user turn not truncated: len=%d", len(got))
	}
}

// TestBudgetMarkerOncePerCall: repeated budgeting of the same history (the
// runner re-budgets every iteration as tool results accumulate) must not
// stack markers — one marker, monotonically dropping the oldest turns.
func TestBudgetMarkerOncePerCall(t *testing.T) {
	in := []ollama.ChatMessage{
		sysMsg("system"),
		userMsg(strings.Repeat("a", 400)),
		asstMsg(strings.Repeat("b", 400)),
		userMsg(strings.Repeat("c", 400)),
		userMsg(strings.Repeat("d", 400)),
		userMsg(strings.Repeat("e", 400)),
	}
	got := BudgetMessages(in, 64)
	markers := 0
	for _, m := range got {
		if strings.Contains(m.Content, "Earlier conversation omitted") {
			markers++
		}
	}
	if markers != 1 {
		t.Fatalf("marker count = %d, want 1: %+v", markers, got)
	}
	// A second pass over the result is stable (no new marker, no growth).
	again := BudgetMessages(got, 64)
	markers = 0
	for _, m := range again {
		if strings.Contains(m.Content, "Earlier conversation omitted") {
			markers++
		}
	}
	if markers != 1 {
		t.Errorf("second pass marker count = %d, want 1", markers)
	}
}

// TestToolCallArgumentsCountTowardBudget: model-requested tool calls carry
// JSON arguments that inflate the input; they must count toward the budget
// (an assistant tool-call turn is what drives the next model request).
func TestToolCallArgumentsCountTowardBudget(t *testing.T) {
	msg := asstMsg("", strings.Repeat("A", 800))
	// content floor (1) + name+args tokens: 1 + (9+800+3)/4 = 204
	want := 1 + (len("read_file")+800+3)/4
	if got := approximateTokens([]ollama.ChatMessage{msg}); got != want {
		t.Errorf("approximateTokens = %d, want %d (tool args must count)", got, want)
	}

	// A history whose overflow comes only from an older tool call must still
	// evict the older turns rather than send the args raw.
	in := []ollama.ChatMessage{
		sysMsg("s"),
		asstMsg("", strings.Repeat("A", 1200)), // older tool call, huge args
		userMsg("what next"),
	}
	got := BudgetMessages(in, 48)
	if n := approximateTokens(got); n > 48 {
		t.Errorf("history with a big tool call not bounded: %d tokens", n)
	}
	if contents := strings.Join(messageContents(got), " "); !strings.Contains(contents, "Earlier conversation omitted") {
		t.Errorf("no truncation marker after evicting tool-call history")
	}
}

// messageContents extracts the content of each message (shared helper for
// budget assertions; it lived in command_test.go before the v0.1 executor
// removal deleted that file).
func messageContents(in []ollama.ChatMessage) []string {
	out := make([]string, len(in))
	for i, msg := range in {
		out[i] = msg.Content
	}
	return out
}
