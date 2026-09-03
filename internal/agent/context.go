package agent

import (
	"selftui/internal/ollama"
)

const truncationNotice = "Earlier conversation omitted to fit the context window."

// BudgetMessages keeps the system prompt and newest turn while dropping the
// oldest conversation when the approximate input budget is full. Three
// quarters of numCtx are reserved for input so the model has room to reply.
func BudgetMessages(messages []ollama.ChatMessage, numCtx int) []ollama.ChatMessage {
	out := append([]ollama.ChatMessage(nil), messages...)
	// A zero/negative numCtx means "no budget" (leave the conversation
	// alone); the runner always sends system + at least one turn, so any
	// other shape is safe to budget. A brand-new chat whose first message is
	// huge must be bounded too — that is what the truncateLatest fallback
	// below handles when there is no older turn to drop.
	if numCtx <= 0 || len(out) == 0 {
		return out
	}
	limit := numCtx * 3 / 4
	if approximateTokens(out) <= limit {
		return out
	}
	firstTurn := 0
	for firstTurn < len(out) && out[firstTurn].Role == ollama.RoleSystem {
		firstTurn++
	}
	if firstTurn >= len(out)-1 {
		return truncateLatest(out, limit)
	}
	// A deterministic marker is the bounded stand-in for a future model-driven
	// summary; never silently pretend the earlier messages are still present.
	marker := ollama.ChatMessage{Role: ollama.RoleSystem, Content: truncationNotice}
	out = append(out[:firstTurn], append([]ollama.ChatMessage{marker}, out[firstTurn:]...)...)
	firstTurn++
	for approximateTokens(out) > limit && firstTurn < len(out)-1 {
		out = append(out[:firstTurn], out[firstTurn+1:]...)
	}
	return truncateLatest(out, limit)
}

func truncateLatest(messages []ollama.ChatMessage, limit int) []ollama.ChatMessage {
	if len(messages) == 0 || approximateTokens(messages) <= limit {
		return messages
	}
	last := len(messages) - 1
	withoutLatest := append([]ollama.ChatMessage(nil), messages[:last]...)
	available := maxInt(1, limit-approximateTokens(withoutLatest)) * 4
	content := messages[last].Content
	if len(content) > available {
		prefix := "[truncated] "
		keep := maxInt(0, available-len(prefix))
		messages[last].Content = prefix + content[len(content)-keep:]
	}
	return messages
}

func approximateTokens(messages []ollama.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += maxInt(1, (len(msg.Content)+3)/4)
		for _, call := range msg.ToolCalls {
			total += maxInt(1, (len(call.Function.Name)+len(call.Function.Arguments)+3)/4)
		}
	}
	return total
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
