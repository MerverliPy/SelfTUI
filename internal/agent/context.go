package agent

import (
	"unicode/utf8"

	"selftui/internal/ollama"
)

// TruncationNotice is the deterministic marker BudgetMessages inserts when it
// drops earlier conversation (PLAN.md §10 M7-C: the UI shows the same marker
// in the transcript once its context meter passes the budget, so a silent
// wire-only truncation becomes visible).
const TruncationNotice = "Earlier conversation omitted to fit the context window."

// truncatedContentPrefix is the deterministic prefix BudgetMessages prepends
// to content it had to shorten (established by the pre-M-02 truncateLatest;
// tests and the transcript both match on it).
const truncatedContentPrefix = "[truncated] "

// compactedToolCallArguments is the deterministic, protocol-valid stand-in for
// tool-call arguments that cannot fit the context budget. It is valid JSON
// (so the assistant tool_calls message stays well-formed on the wire), keeps
// every call name so the correlated role=tool results remain positionally
// matched to their calls, and is far smaller than any real argument payload.
const compactedToolCallArguments = "{}"

// BudgetMessages keeps the pinned system prompt and the newest atomic
// exchanges while dropping the oldest conversation when the approximate input
// budget is full. Three quarters of numCtx are reserved for input so the
// model has room to reply.
//
// Eviction is atomic per user-led exchange (M-02): a tool turn is stored as
// an assistant tool_calls message plus all of its correlated role=tool
// results, and that group is never split — dropping a single message could
// otherwise send the model an orphan tool result, misassociate a result, or
// begin retained history with a role=tool message. The newest complete
// user-led exchange is always retained.
//
// Two shapes cannot be shrunk by eviction or content truncation and are
// bounded deterministically instead of being sent raw past the budget: an
// oversized latest tool call (its arguments become the placeholder above) and
// an oversized pinned system prompt (its content is shortened with the same
// "[truncated] " convention as a last resort). ApproxTokens(out) <= 3/4 of
// numCtx whenever such a bounded representation is possible.
func BudgetMessages(messages []ollama.ChatMessage, numCtx int) []ollama.ChatMessage {
	out := append([]ollama.ChatMessage(nil), messages...)
	// A zero/negative numCtx means "no budget" (leave the conversation
	// alone); the runner always sends system + at least one turn, so any
	// other shape is safe to budget. A brand-new chat whose first message is
	// huge must be bounded too — that is what the in-place bounding fallback
	// below handles when there is no older exchange to drop.
	if numCtx <= 0 || len(out) == 0 {
		return out
	}
	limit := numCtx * 3 / 4
	if approximateTokens(out) <= limit {
		return out
	}

	// Leading system messages are pinned in their intended order. marked
	// detects a marker left by an earlier BudgetMessages pass (the runner
	// re-budgets the same history every iteration): it is part of the pinned
	// prefix and must never be duplicated.
	prefix := 0
	for prefix < len(out) && out[prefix].Role == ollama.RoleSystem {
		prefix++
	}
	marked := prefix > 0 && out[prefix-1].Content == TruncationNotice

	// Split the conversational region into atomic exchanges at each user
	// message: everything after a user message up to the next one — assistant
	// text, assistant tool calls, and all their correlated role=tool results
	// — is one unit and is evicted or retained whole. A leading run of
	// non-user messages (only possible in hand-built fragments) rides in the
	// first exchange so nothing begins a retained history with role=tool.
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
		// One exchange (or the pinned system alone): there is no older
		// conversation to omit, so no marker — bound the retained messages in
		// place instead of ever exceeding the budget.
		return boundToLimit(out, limit, prefix)
	}

	// Drop whole oldest exchanges until the newest suffix fits; the newest
	// complete exchange is always retained. k is the number of exchanges
	// dropped; whenever k > 0 the marker is inserted exactly once.
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
	return boundToLimit(kept, limit, convStart)
}

// boundToLimit deterministically reduces messages that still exceed limit
// after atomic eviction. messages[:convStart] is the pinned system prefix
// (the real prompt plus the truncation marker, when one was inserted);
// messages[convStart:] is the retained newest exchange (or nothing). The
// caller's slice is never mutated.
func boundToLimit(messages []ollama.ChatMessage, limit, convStart int) []ollama.ChatMessage {
	if len(messages) == 0 || approximateTokens(messages) <= limit {
		return append([]ollama.ChatMessage(nil), messages...)
	}
	work := append([]ollama.ChatMessage(nil), messages...)

	// Least-damage first: if replacing the oversized tool-call arguments with
	// the deterministic placeholder already fits, do only that — the user
	// turn and every tool RESULT (the data the model needs to continue) stay
	// intact and only the already-executed argument detail gives way.
	probe := compactToolCallArguments(work)
	if approximateTokens(probe) <= limit {
		return probe
	}

	for approximateTokens(work) > limit {
		// (1) Shorten the newest shrinkable conversational Content (user
		// turns, assistant text, tool results). Newest first, so fresher
		// content keeps its tail as long as possible.
		if trimNewestContent(work, limit, convStart, len(work)) {
			continue
		}
		// (2) Compact the newest assistant tool-call message whose arguments
		// exceed the placeholder. Only messages with no Content to trim ever
		// reach this point (a pure tool_calls message has empty Content).
		if compactNewestCallArguments(work, convStart) {
			continue
		}
		// (3) Last resort: shorten the pinned system prompt itself. Its order
		// is preserved and only an oversized tail gives way; the marker is
		// never shortened, so the truncation notice stays visible.
		if trimSystemContent(work, limit, convStart) {
			continue
		}
		break // genuinely unboundedable at these per-message floors
	}
	return work
}

// contentTailWithin returns the longest tail (suffix) of content that fits in
// maxBytes and starts on a UTF-8 rune boundary (P1-7). The budget math is
// byte-based, so a byte-exact cut can split a multibyte rune and hand the
// model invalid UTF-8 (json.Marshal would silently replace the broken bytes).
// Advancing the cut to the next rune start keeps the output valid and only
// ever makes it shorter, so the byte budget stays respected. On a tail that
// is already aligned (or an all-ASCII cut) the rune start is unchanged.
func contentTailWithin(content string, maxBytes int) string {
	start := len(content) - maxBytes
	if start < 0 {
		return content
	}
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++ // step past a continuation byte onto the next rune's lead
	}
	return content[start:]
}

// trimNewestContent shortens the newest message in [lo, hi) whose Content can
// actually shrink the estimate, using the deterministic "[truncated] " tail
// convention. It reports whether any message was shortened.
func trimNewestContent(messages []ollama.ChatMessage, limit, lo, hi int) bool {
	for i := hi - 1; i >= lo; i-- {
		content := messages[i].Content
		if content == "" {
			continue
		}
		others := approximateTokens(messages) - tokenOfContent(content)
		room := maxInt(1, limit-others) * 4
		if len(content) <= room {
			continue // this message is not the overflow; leave it alone
		}
		keep := maxInt(0, room-len(truncatedContentPrefix))
		next := truncatedContentPrefix + contentTailWithin(content, keep)
		if tokenOfContent(next) >= tokenOfContent(content) {
			continue // no strict decrease at these sizes; try an older message
		}
		messages[i].Content = next
		return true
	}
	return false
}

// compactNewestCallArguments replaces the arguments of the newest assistant
// tool-call message whose arguments exceed the placeholder, newest first. It
// reports whether any message was compacted. Call names are preserved so the
// following role=tool results stay positionally correlated with their calls.
func compactNewestCallArguments(messages []ollama.ChatMessage, lo int) bool {
	for i := len(messages) - 1; i >= lo; i-- {
		msg := messages[i]
		if len(msg.ToolCalls) == 0 {
			continue
		}
		compacted := compactToolCallArguments([]ollama.ChatMessage{msg})[0]
		if tokenOfMessage(compacted) >= tokenOfMessage(msg) {
			continue // no oversized arguments; nothing to gain
		}
		messages[i] = compacted
		return true
	}
	return false
}

// trimSystemContent shortens the newest pinned system message (skipping the
// truncation marker) whose Content can actually shrink the estimate. It
// reports whether any message was shortened.
func trimSystemContent(messages []ollama.ChatMessage, limit, convStart int) bool {
	for i := convStart - 1; i >= 0; i-- {
		content := messages[i].Content
		if content == "" || content == TruncationNotice {
			continue
		}
		others := approximateTokens(messages) - tokenOfContent(content)
		room := maxInt(1, limit-others) * 4
		if len(content) <= room {
			continue
		}
		keep := maxInt(0, room-len(truncatedContentPrefix))
		next := truncatedContentPrefix + contentTailWithin(content, keep)
		if tokenOfContent(next) >= tokenOfContent(content) {
			continue
		}
		messages[i].Content = next
		return true
	}
	return false
}

// compactToolCallArguments returns a copy of messages in which every tool-call
// argument has been replaced by the deterministic placeholder (calls whose
// arguments already equal the placeholder are left untouched).
func compactToolCallArguments(messages []ollama.ChatMessage) []ollama.ChatMessage {
	out := append([]ollama.ChatMessage(nil), messages...)
	for i := range out {
		if len(out[i].ToolCalls) == 0 {
			continue
		}
		calls := append([]ollama.ToolCall(nil), out[i].ToolCalls...)
		changed := false
		for j := range calls {
			if string(calls[j].Function.Arguments) == compactedToolCallArguments {
				continue
			}
			calls[j].Function.Arguments = []byte(compactedToolCallArguments)
			changed = true
		}
		if changed {
			out[i].ToolCalls = calls
		}
	}
	return out
}

// ApproxTokens is the exported approximate-token estimator (4 chars per
// token, tool-call arguments counted) that drives both BudgetMessages and the
// UI context meter (M7-C). The meter reuses exactly this math so its
// percentage is the number the runner will actually budget against.
func ApproxTokens(messages []ollama.ChatMessage) int { return approximateTokens(messages) }

func approximateTokens(messages []ollama.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += tokenOfMessage(msg)
	}
	return total
}

func tokenOfMessage(msg ollama.ChatMessage) int {
	total := tokenOfContent(msg.Content)
	for _, call := range msg.ToolCalls {
		total += tokenOfCall(call)
	}
	return total
}

func tokenOfContent(content string) int { return maxInt(1, (len(content)+3)/4) }

func tokenOfCall(call ollama.ToolCall) int {
	return maxInt(1, (len(call.Function.Name)+len(call.Function.Arguments)+3)/4)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
