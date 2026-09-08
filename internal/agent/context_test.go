package agent

// Context-budget edge tests (M6 — context-truncation edges). BudgetMessages
// must never send the model more than ~3/4 of num_ctx regardless of turn
// shape: a brand-new chat whose first message is huge, conversations without
// older turns to drop, tool-call argument bloat, and repeated calls (the
// runner re-budgets the same history every iteration).

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
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

// --- M-02 protocol-safe context budgeting (external audit M-02) ------------
//
// BudgetMessages must evict conversation in ATOMIC exchange units, not one
// message at a time: a tool turn is an assistant tool_calls message plus all
// of its correlated role=tool results, and splitting that group (or leaving a
// result behind when its call is dropped) sends Ollama a protocol-invalid
// history. It must also never silently exceed the budget: an oversized
// retained tool call or an oversized system prompt has no shrinkable Content,
// so one-message eviction + truncateLatest cannot bound it. Regression
// helpers here drive the six table scenarios in TestBudgetM02ProtocolSafe.

// budgetLimitFor returns the smallest numCtx whose three-quarter budget is
// exactly wantLimit, so table cases can express limits precisely.
func budgetLimitFor(wantLimit int) int {
	return (4*wantLimit + 2) / 3
}

// callToolMsg is an assistant tool_calls message for the named tool.
func callToolMsg(name, args string) ollama.ChatMessage {
	return ollama.ChatMessage{
		Role: ollama.RoleAssistant,
		ToolCalls: []ollama.ToolCall{{
			Function: ollama.ToolCallFunction{Name: name, Arguments: jsonRaw(args)},
		}},
	}
}

func jsonRaw(s string) []byte { return []byte(s) }

// toolResultMsg is one role=tool result correlated with a preceding call.
func toolResultMsg(name, content string) ollama.ChatMessage {
	return ollama.ChatMessage{Role: ollama.RoleTool, ToolName: name, Content: content}
}

// checkBudgetInvariants asserts the M-02 structural postconditions on any
// BudgetMessages result: at most one truncation marker, no retained role=tool
// message whose immediately preceding retained message is not an assistant
// tool call, and — when a bound is achievable — ApproxTokens <= limit. When
// idempotent is true the result must also be a fixed point of BudgetMessages
// (the runner re-budgets the same history every iteration).
func checkBudgetInvariants(t *testing.T, got []ollama.ChatMessage, limit int, idempotent bool) {
	t.Helper()
	markers := 0
	for _, m := range got {
		if m.Role == ollama.RoleSystem && m.Content == TruncationNotice {
			markers++
		}
	}
	if markers > 1 {
		t.Errorf("marker count = %d, want at most 1", markers)
	}
	for i, m := range got {
		if m.Role != ollama.RoleTool {
			continue
		}
		if i == 0 {
			t.Errorf("retained history begins with an orphan role=tool message")
			continue
		}
		if prev := got[i-1]; prev.Role != ollama.RoleAssistant || len(prev.ToolCalls) == 0 {
			t.Errorf("role=tool message at index %d lacks an immediately preceding assistant tool call", i)
		}
	}
	if limit >= 0 && approximateTokens(got) > limit {
		t.Errorf("output exceeds budget: approximateTokens = %d, limit = %d", approximateTokens(got), limit)
	}
	if idempotent {
		if again := BudgetMessages(got, budgetLimitFor(limit)); !reflect.DeepEqual(again, got) {
			t.Errorf("result is not a fixed point of BudgetMessages:\n first:  %+v\n second: %+v", got, again)
		}
	}
}

// rolesOf extracts the role sequence for compact structural assertions.
func rolesOf(msgs []ollama.ChatMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = string(m.Role)
	}
	return out
}

func wantRoles(t *testing.T, got []ollama.ChatMessage, want ...string) {
	t.Helper()
	if gotRoles := rolesOf(got); !reflect.DeepEqual(gotRoles, want) {
		t.Fatalf("roles = %v, want %v\nmessages: %+v", gotRoles, want, got)
	}
}

// contentPresent reports whether any retained message's content equals the
// marker string (used to prove a whole exchange was dropped with it).
func contentPresent(got []ollama.ChatMessage, marker string) bool {
	for _, m := range got {
		if m.Content == marker {
			return true
		}
	}
	return false
}

// TestBudgetM02ProtocolSafe is the M-02 table: atomic exchange eviction,
// orphan prevention, newest-exchange preference, and deterministic bounded
// representations for the two unshrinkable oversized shapes (a giant retained
// tool call and a giant system prompt), plus a tiny num_ctx.
func TestBudgetM02ProtocolSafe(t *testing.T) {
	sys := sysMsg("s")
	big := func(n int) string { return strings.Repeat("x", n) }
	bigR := func(n int) string { return strings.Repeat("r", n) }

	cases := []struct {
		name   string
		in     []ollama.ChatMessage
		numCtx int
		limit  int
		check  func(t *testing.T, got []ollama.ChatMessage)
	}{
		{
			// One assistant tool call + one result inside an OLDER exchange
			// that must be dropped: the call and its result must vanish
			// together. One-message eviction used to stop between them and
			// start retained history with a role=tool orphan.
			name:   "assistant call plus one result evicted atomically",
			in:     []ollama.ChatMessage{sys, userMsg(big(400)), callToolMsg("read_file", `{"path":"/old"}`), toolResultMsg("read_file", bigR(400)), userMsg(bigN("n", 60)), asstMsg(bigN("m", 60))},
			numCtx: budgetLimitFor(150),
			limit:  150,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				wantRoles(t, got, "system", "system", "user", "assistant")
				if !strings.Contains(got[len(got)-2].Content, "n") {
					t.Errorf("newest user turn lost: %+v", got)
				}
				if contentPresent(got, big(400)) || contentPresent(got, bigR(400)) {
					t.Errorf("dropped exchange left a message behind: %+v", got)
				}
			},
		},
		{
			// One assistant message carrying a parallel batch of calls plus
			// ALL of their results: the whole group is one atomic exchange.
			// No result may survive its calls, and no call may survive
			// without every correlated result.
			name: "parallel calls plus all results evicted atomically",
			in: []ollama.ChatMessage{
				sys, userMsg(big(400)),
				assistantParallelCalls(`{"path":"/a"}`, `{"path":"/b"}`),
				toolResultMsg("read_file", bigR(300)), toolResultMsg("list_dir", bigR(300)),
				userMsg(bigN("n", 60)), asstMsg(bigN("m", 60)),
			},
			numCtx: budgetLimitFor(225),
			limit:  225,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				wantRoles(t, got, "system", "system", "user", "assistant")
				if contentPresent(got, bigR(300)) || contentPresent(got, big(400)) {
					t.Errorf("parallel batch left a call or result behind: %+v", got)
				}
			},
		},
		{
			// Two older exchanges (one plain, one a full tool exchange) plus
			// the newest exchange. The newest complete user-led exchange is
			// preferred: both older exchanges must disappear whole, in order.
			name: "two older exchanges dropped before the newest",
			in: []ollama.ChatMessage{
				sys,
				userMsg(big(300)), asstMsg(bigN("y", 300)), // oldest plain exchange
				userMsg(big(500)), callToolMsg("read_file", `{"path":"/p"}`), toolResultMsg("read_file", bigR(500)), // middle tool exchange
				userMsg(bigN("w", 40)), asstMsg(bigN("z", 40)), // newest exchange
			},
			numCtx: budgetLimitFor(170),
			limit:  170,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				wantRoles(t, got, "system", "system", "user", "assistant")
				if !strings.Contains(got[len(got)-1].Content, "z") || !strings.Contains(got[len(got)-2].Content, "w") {
					t.Errorf("newest exchange not retained whole: %+v", got)
				}
				for _, lost := range []string{big(300), big(500), bigR(500)} {
					if contentPresent(got, lost) {
						t.Errorf("older exchange content survived: %+v", got)
					}
				}
			},
		},
		{
			// A retained tool exchange whose call arguments alone blow the
			// budget. Content truncation cannot shrink arguments; the call
			// must get a deterministic bounded representation and the whole
			// exchange (call + result + user turn) must stay intact and under
			// the limit — never silently sent raw.
			name:   "latest oversized tool call bounded deterministically",
			in:     []ollama.ChatMessage{sys, userMsg(bigN("q", 80)), callToolMsg("write_file", `{"content":"`+big(4000)+`"}`), toolResultMsg("write_file", "wrote /x")},
			numCtx: budgetLimitFor(150),
			limit:  150,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				wantRoles(t, got, "system", "user", "assistant", "tool")
				asst := got[2]
				if len(asst.ToolCalls) != 1 || asst.ToolCalls[0].Function.Name != "write_file" {
					t.Fatalf("tool call not retained: %+v", got)
				}
				if args := string(asst.ToolCalls[0].Function.Arguments); args != "{}" {
					t.Errorf("arguments not compacted to the deterministic placeholder: %q", args)
				}
				if !strings.HasPrefix(got[1].Content, "q") || got[3].Content != "wrote /x" {
					t.Errorf("user turn or result corrupted: %+v", got)
				}
			},
		},
		{
			// An oversized system prompt cannot be evicted and has no
			// conversational older turn to drop; the output must still be
			// bounded via a deterministic representation of the pinned system
			// message, in its intended position, with no marker added.
			name:   "oversized system prompt bounded",
			in:     []ollama.ChatMessage{sysMsg(big(20000)), userMsg(bigN("h", 40))},
			numCtx: budgetLimitFor(48),
			limit:  48,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				if len(got) != 2 {
					t.Fatalf("len = %d, want 2 (system + user): %+v", len(got), got)
				}
				if got[0].Role != ollama.RoleSystem || !strings.HasPrefix(got[0].Content, "[truncated] ") {
					t.Errorf("system message not bounded deterministically: %+v", got[0])
				}
				if got[1].Role != ollama.RoleUser {
					t.Errorf("second message is not the user turn: %+v", got[1])
				}
			},
		},
		{
			// num_ctx so small that the whole budget is a handful of tokens:
			// a lone huge user turn (and a system + huge user) must still be
			// represented within the limit when that is possible.
			name:   "tiny num_ctx still bounded",
			in:     []ollama.ChatMessage{userMsg(big(1000))},
			numCtx: budgetLimitFor(6),
			limit:  6,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				if len(got) != 1 || !strings.HasPrefix(got[0].Content, "[truncated] ") {
					t.Fatalf("lone huge turn not bounded: %+v", got)
				}
			},
		},
		{
			name:   "tiny num_ctx with system pinned",
			in:     []ollama.ChatMessage{sys, userMsg(big(1000))},
			numCtx: budgetLimitFor(6),
			limit:  6,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				wantRoles(t, got, "system", "user")
				if got[0].Content != "s" || !strings.HasPrefix(got[1].Content, "[truncated] ") {
					t.Fatalf("system or user not as expected: %+v", got)
				}
			},
		},
		{
			// A newest retained tool exchange whose RESULT is enormous (not
			// its args): the call and its result must stay together and the
			// result content is what gets the deterministic bound.
			name:   "oversized newest result keeps its call",
			in:     []ollama.ChatMessage{sys, userMsg(bigN("q", 80)), callToolMsg("read_file", `{"path":"/x"}`), toolResultMsg("read_file", bigR(2000))},
			numCtx: budgetLimitFor(300),
			limit:  300,
			check: func(t *testing.T, got []ollama.ChatMessage) {
				wantRoles(t, got, "system", "user", "assistant", "tool")
				if len(got[2].ToolCalls) != 1 || got[2].ToolCalls[0].Function.Name != "read_file" {
					t.Fatalf("tool call lost while its result was trimmed: %+v", got)
				}
				if got[3].Role != ollama.RoleTool || !strings.HasPrefix(got[3].Content, "[truncated] ") {
					t.Errorf("oversized result not bounded in place: %+v", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BudgetMessages(tc.in, tc.numCtx)
			checkBudgetInvariants(t, got, tc.limit, true)
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// bigN is like big but with a distinct filler byte so exchanges are easy to
// tell apart in assertions.
func bigN(b string, n int) string { return strings.Repeat(b, n) }

// assistantParallelCalls is one assistant message carrying two tool calls.
func assistantParallelCalls(argsA, argsB string) ollama.ChatMessage {
	return ollama.ChatMessage{
		Role: ollama.RoleAssistant,
		ToolCalls: []ollama.ToolCall{
			{Function: ollama.ToolCallFunction{Name: "read_file", Arguments: jsonRaw(argsA)}},
			{Function: ollama.ToolCallFunction{Name: "list_dir", Arguments: jsonRaw(argsB)}},
		},
	}
}

// P1-7 regression: content truncation must never split a multibyte UTF-8
// rune. The budget math is byte-based, so a byte-exact cut can land inside a
// CJK/emoji sequence and hand the model invalid UTF-8 (which json.Marshal
// would silently replace). BudgetMessages must produce only valid UTF-8
// output regardless of how a cut lands.
func TestBudgetTruncationKeepsValidUTF8(t *testing.T) {
	// Build one oversized user turn from multibyte content so any shortening
	// path must cut inside rune-dense text. 你好 is 3 bytes per rune.
	filler := strings.Repeat("你", 3000) // 9000 bytes
	in := []ollama.ChatMessage{
		sysMsg("be brief"),
		userMsg("prefix " + filler + " 世界"),
		asstMsg("ok"),
	}
	got := BudgetMessages(in, 2000) // tiny budget: forces truncation

	for i, m := range got {
		if !utf8.ValidString(m.Content) {
			t.Errorf("message %d has invalid UTF-8 after budgeting: %q", i, m.Content)
		}
	}
	if len(got) != len(in) {
		t.Errorf("len = %d, want %d (in-place bounding, not eviction)", len(got), len(in))
	}
	// The oversized user turn (index 1) is shortened in place with the tail
	// convention: a clean, valid suffix (prefix + rune-aligned tail).
	user := got[1]
	if user.Role != ollama.RoleUser || !strings.HasPrefix(user.Content, "[truncated] ") {
		t.Fatalf("user turn not truncated: role=%v content=%q", user.Role, user.Content)
	}
	if !strings.HasSuffix(user.Content, "世界") {
		t.Errorf("user turn lost its (valid) tail end: %q", user.Content)
	}
}

// trim helpers unit-level: the byte-keep math must align to a rune boundary
// even when the natural cut point is mid-sequence.
func TestTrimTailAlignsToRuneBoundary(t *testing.T) {
	// 9 runes x 3 bytes = 27 bytes; cutting to 10 bytes lands at byte 10,
	// which is inside rune 4. The tail must start at a rune boundary.
	s := "你你你你你你你你你" // 27 bytes
	got := contentTailWithin(s, 10)
	if !utf8.ValidString(got) {
		t.Errorf("tail %q is invalid UTF-8", got)
	}
	if len(got) > 10 {
		t.Errorf("tail len = %d > budget 10", len(got))
	}
	if got != s[len(s)-9:] {
		t.Errorf("tail = %q, want the last 3 full runes %q", got, s[len(s)-9:])
	}
}

// --- perf rewrite regression: incremental accounting vs whole-slice rescans
// (audit concept c1). BudgetMessages used to rescan the whole remaining
// message slice with approximateTokens for every eviction candidate, and
// boundToLimit rescan-summed the whole slice on every trimming pass, so
// re-budgeting a long tool-heavy history was quadratic. The rewrite keeps
// running totals; the oracle below reimplements the pre-rewrite semantics with
// full rescans so any behavioral drift in the incremental accounting is
// caught by direct comparison.

// referenceBudgetMessages reimplements the pre-incremental BudgetMessages
// semantics exactly: every eviction candidate rescan-sums its whole remaining
// suffix and the kept suffix is re-summed before the in-place bound. It is
// intentionally quadratic and exists only as a test oracle.
func referenceBudgetMessages(messages []ollama.ChatMessage, numCtx int) []ollama.ChatMessage {
	out := append([]ollama.ChatMessage(nil), messages...)
	if numCtx <= 0 || len(out) == 0 {
		return out
	}
	limit := numCtx * 3 / 4
	if approximateTokens(out) <= limit {
		return out
	}

	prefix := 0
	for prefix < len(out) && out[prefix].Role == ollama.RoleSystem {
		prefix++
	}
	marked := prefix > 0 && out[prefix-1].Content == TruncationNotice

	var bounds []int
	if prefix < len(out) {
		bounds = append(bounds, prefix)
	}
	for i := prefix + 1; i < len(out); i++ {
		if out[i].Role == ollama.RoleUser {
			bounds = append(bounds, i)
		}
	}

	if len(bounds) <= 1 {
		return referenceBoundToLimit(out, limit, prefix)
	}

	marker := ollama.ChatMessage{Role: ollama.RoleSystem, Content: TruncationNotice}
	base := approximateTokens(out[:prefix])
	k := 0
	for ; k < len(bounds); k++ {
		total := base + approximateTokens(out[bounds[k]:])
		if k > 0 && !marked {
			total += tokenOfContent(TruncationNotice)
		}
		if total <= limit {
			break
		}
	}
	if k >= len(bounds) {
		k = len(bounds) - 1
	}
	kept := append([]ollama.ChatMessage(nil), out[:prefix]...)
	convStart := prefix
	if k > 0 && !marked {
		kept = append(kept, marker)
		convStart++
	}
	kept = append(kept, out[bounds[k]:]...)
	if approximateTokens(kept) <= limit {
		return kept
	}
	return referenceBoundToLimit(kept, limit, convStart)
}

// referenceBoundToLimit is the pre-incremental boundToLimit: it rescan-sums
// the whole work slice before every decision and hands each trimming helper a
// fresh full-slice total (the old helpers recomputed that same value per
// candidate, since the slice is unchanged during one pass). Trimming
// decisions and mutations are therefore exactly the old semantics.
func referenceBoundToLimit(messages []ollama.ChatMessage, limit, convStart int) []ollama.ChatMessage {
	if len(messages) == 0 || approximateTokens(messages) <= limit {
		return append([]ollama.ChatMessage(nil), messages...)
	}
	work := append([]ollama.ChatMessage(nil), messages...)

	probe := compactToolCallArguments(work)
	if approximateTokens(probe) <= limit {
		return probe
	}

	for approximateTokens(work) > limit {
		if ok, _ := trimNewestContent(work, limit, convStart, len(work), approximateTokens(work)); ok {
			continue
		}
		if ok, _ := compactNewestCallArguments(work, convStart, approximateTokens(work)); ok {
			continue
		}
		if ok, _ := trimSystemContent(work, limit, convStart, approximateTokens(work)); ok {
			continue
		}
		break
	}
	return work
}

// toolHistoryMessages builds a large tool-heavy conversation: a pinned system
// message, exchanges whole tool turns (user prompt, assistant read_file call,
// and its correlated result), then a fresh newest user turn with assistant
// text. Filler bytes are distinct per exchange and for the newest turn, so
// dropped content is distinguishable from what the budget must retain.
func toolHistoryMessages(exchanges, turnBytes int) []ollama.ChatMessage {
	msgs := []ollama.ChatMessage{sysMsg("pinned system prompt")}
	for i := 0; i < exchanges; i++ {
		msgs = append(msgs, toolExchangeMessages(byte('a'+i%26), turnBytes)...)
	}
	msgs = append(msgs,
		userMsg(strings.Repeat("Q", turnBytes/2)),
		asstMsg(strings.Repeat("Y", turnBytes/4)),
	)
	return msgs
}

// toolExchangeMessages is one user-led tool turn: the user prompt, the
// assistant tool call, and its correlated role=tool result.
func toolExchangeMessages(seed byte, turnBytes int) []ollama.ChatMessage {
	fill := func(b byte, n int) string { return strings.Repeat(string(b), n) }
	return []ollama.ChatMessage{
		userMsg(fill(seed, turnBytes)),
		callToolMsg("read_file", `{"path":"`+fill(seed+1, turnBytes/4)+`"}`),
		toolResultMsg("read_file", fill(seed+2, turnBytes/2)),
	}
}

// checkBudgetMatchesReference fails the test unless BudgetMessages produces
// exactly what the rescanning reference produces for the same input.
func checkBudgetMatchesReference(t *testing.T, in []ollama.ChatMessage, numCtx int) []ollama.ChatMessage {
	t.Helper()
	got := BudgetMessages(in, numCtx)
	if want := referenceBudgetMessages(in, numCtx); !reflect.DeepEqual(got, want) {
		t.Fatalf("BudgetMessages diverged from the rescanning reference:\n got: %+v\nwant: %+v", got, want)
	}
	return got
}

// TestBudgetMessagesMatchesReference locks the incremental-accounting rewrite
// to the exact pre-rewrite semantics across every phase: under-budget
// passthrough, zero/negative budgets, whole-exchange eviction (with and
// without an already-present marker), in-place bounding of a lone oversized
// exchange, argument compaction and content trimming inside boundToLimit, and
// large tool-heavy histories.
func TestBudgetMessagesMatchesReference(t *testing.T) {
	sys := sysMsg("s")
	big := func(n int) string { return strings.Repeat("x", n) }
	bigR := func(n int) string { return strings.Repeat("r", n) }

	hugeNewest := append(toolHistoryMessages(20, 300),
		userMsg(strings.Repeat("N", 20000)), asstMsg(strings.Repeat("M", 20000)))
	markedHistory := BudgetMessages(toolHistoryMessages(40, 400), budgetLimitFor(3000))

	cases := []struct {
		name   string
		in     []ollama.ChatMessage
		numCtx int
	}{
		{name: "under budget", in: []ollama.ChatMessage{sys, userMsg("hi"), asstMsg("yo")}, numCtx: 4096},
		{name: "empty history", in: nil, numCtx: 4096},
		{name: "no budget", in: []ollama.ChatMessage{sys, userMsg(big(10000))}, numCtx: 0},
		{name: "negative budget", in: []ollama.ChatMessage{sys, userMsg(big(10000))}, numCtx: -8},
		{name: "single huge user turn", in: []ollama.ChatMessage{sysMsg("short system"), userMsg(big(20000))}, numCtx: 64},
		{name: "lone huge user turn", in: []ollama.ChatMessage{userMsg(big(20000))}, numCtx: 64},
		{name: "huge system prompt bounded", in: []ollama.ChatMessage{sysMsg(big(20000)), userMsg("go")}, numCtx: budgetLimitFor(48)},
		{name: "tiny num_ctx", in: []ollama.ChatMessage{userMsg(big(1000))}, numCtx: budgetLimitFor(6)},
		{name: "older tool exchange evicted atomically", in: []ollama.ChatMessage{sys, userMsg(big(400)), callToolMsg("read_file", `{"path":"/old"}`), toolResultMsg("read_file", bigR(400)), userMsg("next"), asstMsg("ok")}, numCtx: budgetLimitFor(150)},
		{name: "oversized retained tool args compacted", in: []ollama.ChatMessage{sys, userMsg(big(80)), callToolMsg("write_file", `{"content":"`+big(4000)+`"}`), toolResultMsg("write_file", "wrote /x")}, numCtx: budgetLimitFor(150)},
		{name: "oversized newest result trimmed", in: []ollama.ChatMessage{sys, userMsg(big(80)), callToolMsg("read_file", `{"path":"/x"}`), toolResultMsg("read_file", bigR(2000))}, numCtx: budgetLimitFor(300)},
		{name: "large tool history heavy eviction", in: toolHistoryMessages(60, 400), numCtx: budgetLimitFor(3000)},
		{name: "all exchanges dropped, newest oversized", in: hugeNewest, numCtx: budgetLimitFor(4000)},
		{name: "re-budget an already-marked history", in: markedHistory, numCtx: budgetLimitFor(3000)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkBudgetMatchesReference(t, tc.in, tc.numCtx)
		})
	}
}

// TestBudgetMessagesLargeToolHistory exercises re-budgeting of a large
// tool-heavy conversation — the exact case that used to rescan the whole
// message slice per eviction candidate. The result must stay inside the 3/4
// budget, stay a fixed point of BudgetMessages, and keep agreeing with the
// rescanning reference on every pass while the runner appends new tool turns.
func TestBudgetMessagesLargeToolHistory(t *testing.T) {
	hist := toolHistoryMessages(60, 400) // ~10.9k approximate tokens
	numCtx := budgetLimitFor(3000)
	limit := 3000

	got := checkBudgetMatchesReference(t, hist, numCtx)
	checkBudgetInvariants(t, got, limit, true)
	if n := ApproxTokens(got); n > limit {
		t.Errorf("output over the 3/4 budget: ApproxTokens = %d > limit %d", n, limit)
	}
	if !contentPresent(got, TruncationNotice) {
		t.Errorf("no truncation marker after evicting a large tool history")
	}
	if last := got[len(got)-1]; last.Content != strings.Repeat("Y", 100) {
		t.Errorf("newest assistant text not retained whole: %q", last.Content)
	}

	// The runner re-budgets the same (growing) history every iteration: each
	// appended tool turn must re-budget identically to the rescanning oracle.
	cur := hist
	for i := 0; i < 5; i++ {
		cur = append(cur, toolExchangeMessages(byte('A'+i), 400)...)
		cur = checkBudgetMatchesReference(t, cur, numCtx)
		checkBudgetInvariants(t, cur, limit, false)
	}
	if again := BudgetMessages(cur, numCtx); !reflect.DeepEqual(again, cur) {
		t.Errorf("result is not a fixed point of BudgetMessages after re-budgeting")
	}
}

// TestBudgetMessagesPreservesAtomicToolExchange: with the over-budget region
// spanning many tool turns, eviction must drop whole user-led exchanges — no
// lone assistant call, no orphan role=tool result, no retained history that
// begins mid-exchange — and must match the rescanning reference.
func TestBudgetMessagesPreservesAtomicToolExchange(t *testing.T) {
	hist := toolHistoryMessages(90, 500)
	numCtx := budgetLimitFor(2500)
	limit := 2500

	got := checkBudgetMatchesReference(t, hist, numCtx)
	checkBudgetInvariants(t, got, limit, true)

	// Retained history must start at an exchange boundary: right after the
	// pinned system prefix (the truncation marker included) the first
	// conversational message is a user turn, never an assistant call or an
	// orphan tool result.
	conv := 0
	for conv < len(got) && got[conv].Role == ollama.RoleSystem {
		conv++
	}
	if conv >= len(got) {
		t.Fatalf("nothing retained after the pinned system prefix")
	}
	if got[conv].Role != ollama.RoleUser {
		t.Fatalf("retained history starts mid-exchange at %d: %+v", conv, got[conv])
	}
	if !contentPresent(got, TruncationNotice) {
		t.Errorf("no marker although older tool exchanges were evicted")
	}
	// Every retained assistant tool call still has its correlated result, and
	// no result survives its call: eviction never splits a tool turn.
	calls, results := 0, 0
	for _, m := range got {
		if m.Role == ollama.RoleAssistant && len(m.ToolCalls) > 0 {
			calls++
		}
		if m.Role == ollama.RoleTool {
			results++
		}
	}
	if calls != results {
		t.Errorf("tool turn split by eviction: %d calls retained but %d results", calls, results)
	}
	if got[len(got)-1].Content != strings.Repeat("Y", 125) {
		t.Errorf("newest exchange not retained whole: %q", got[len(got)-1].Content)
	}
}

// benchBudgetSink keeps benchmark outputs reachable so the compiler cannot
// eliminate the measured call.
var benchBudgetSink []ollama.ChatMessage

// BenchmarkBudgetMessagesLargeToolHistory measures re-budgeting a ~1.8k
// message tool-heavy history that forces eviction of most older exchanges
// (the case that used to rescan the whole remaining slice per candidate).
func BenchmarkBudgetMessagesLargeToolHistory(b *testing.B) {
	history := toolHistoryMessages(600, 500)
	numCtx := budgetLimitFor(30000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchBudgetSink = BudgetMessages(history, numCtx)
	}
}

// BenchmarkBudgetMessagesLargeToolHistoryReference times the pre-rewrite
// semantics (whole-slice rescans) on the same history, so the perf claim is
// measurable against the oracle.
func BenchmarkBudgetMessagesLargeToolHistoryReference(b *testing.B) {
	history := toolHistoryMessages(600, 500)
	numCtx := budgetLimitFor(30000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchBudgetSink = referenceBudgetMessages(history, numCtx)
	}
}
