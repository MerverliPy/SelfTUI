package agent

import (
	"context"
	"strings"
)

// N6 composer attachments (PLAN.md §12): the composer's @-file picker inserts
// `@path` references into the draft; the send path expands them into inline
// file blocks on the wire. Expansion is deliberately the same jailed read the
// model itself could request — ReadFile applies securePath (no workspace
// escape, no symlink escape), the maxReadBytes ceiling, and the ctx-cancel
// checks. There is no new read primitive here.

// FileRefTokens extracts the workspace-file candidates from a draft: every
// '@' begins a token that runs to the next whitespace (or end of text), with
// trailing prose punctuation ("@README.md.", "@src/app.go,") stripped so a
// sentence-final reference still resolves. The tokens are candidates only —
// ExpandFileRefs decides which ones name real, readable workspace files;
// anything else stays ordinary prose.
func FileRefTokens(text string) []string {
	var out []string
	for i := 0; i < len(text); {
		if text[i] != '@' {
			i++
			continue
		}
		j := i + 1
		for j < len(text) && text[j] != ' ' && text[j] != '\t' && text[j] != '\n' && text[j] != '\r' {
			j++
		}
		token := strings.TrimRight(text[i+1:j], ".,;:!?)\"']}")
		if token != "" {
			out = append(out, token)
		}
		i = j
	}
	return out
}

// ExpandFileRefs turns one draft into the wire content for an agent turn:
// every '@token' that resolves to a readable workspace file gets an inline
// `[file: token]` block appended (in first-mention order, deduplicated);
// references that name an existing but unreadable file (directory, over the
// size cap, or a workspace escape attempt) carry a visible "unavailable"
// note instead of content. Tokens that name no file at all are left as
// ordinary prose. The returned value is the full message content to send.
func ExpandFileRefs(ctx context.Context, root, text string) string {
	tokens := FileRefTokens(text)
	if len(tokens) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	seen := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		content, err := ReadFile(ctx, root, token)
		switch {
		case err == nil:
			b.WriteString("\n\n[file: " + token + "]\n" + content)
		case isMissingEntry(err):
			// Not a file in the workspace: the token is prose, not a
			// reference. Nothing is attached and nothing is announced.
		default:
			// The user referenced something the jail refuses (escape
			// attempt, directory, oversized): say so instead of attaching
			// silently or failing the whole send.
			note := firstLineOf(err.Error())
			b.WriteString("\n\n[file: " + token + " — unavailable: " + note + "]")
		}
	}
	return b.String()
}

// isMissingEntry reports whether err is ReadFile's "no such file or
// directory" shape — the signal that a token names no workspace entry and is
// therefore prose rather than a failed reference.
func isMissingEntry(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such file or directory")
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
