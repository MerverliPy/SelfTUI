package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MerverliPy/SelfTUI/internal/ollama"
)

// sanitizeTerminalText is the single pre-style sanitization boundary for
// remote-derived text (H-05): model names/details, API error bodies, streamed
// chat tokens, tool inputs/results, and the raw-markdown fallback all pass
// through it before they are stored for display or styled by SelfTUI.
//
// A malicious Ollama host/model can otherwise embed terminal control
// sequences — OSC 52 (clipboard writes), CSI (screen clear, cursor movement),
// DCS (sixel), and bare C0/C1 control bytes — that a real terminal would
// execute as they stream out of the render cache. The JSON layer decodes
// "\u001b" to a real ESC byte, so this is reachable without any terminal
// trickery.
//
// The rule set is intentionally lossless for real text and lossy only for
// terminal control:
//
//  1. Parse the whole string as an ECMA-48 stream and drop every escape
//     sequence (CSI/OSC/DCS/APC/PM/SOS and their 8-bit C1 forms), including
//     one truncated at the end of the string — a token stream can split a
//     sequence mid-way, so a dangling introducer must never survive to be
//     completed by the next frame.
//  2. Drop any remaining unsafe control bytes: every C0 control except
//     newline (\n) and tab (\t), plus DEL and the C1 controls (standalone
//     BEL, CR, BS, VT, FF, and the C1 range) that the parser may leave in
//     ground state.
//
// It must be applied BEFORE any SelfTUI styling (lipgloss SGR, glamour
// markdown, pane borders): once the app's own styles are in the string, ESC
// sequences are legitimate and must never be stripped. It is idempotent, so
// double application (a render path that re-checks already-clean state) is
// harmless.
func sanitizeTerminalText(s string) string {
	// ansi.Strip implements a state machine over ground/escape/CSI/OSC/DCS
	// states and emits only printable characters, dropping complete AND
	// dangling escape-introduced or C1-introduced sequences wholesale. It is
	// pinned transitively by the Charm v2 set (charmbracelet/x/ansi), so
	// reusing it adds no new dependency. On its own it preserves standalone
	// C0 controls (BEL, CR, BS, VT, FF), so pass two filters those out.
	out := ansi.Strip(s)
	var b strings.Builder
	b.Grow(len(out))
	for _, r := range out {
		switch {
		case r == '\n' || r == '\t':
			// Newlines and tabs carry real layout semantics in the chat
			// transcript, the detail pane, and raw model file text; keep them.
			b.WriteRune(r)
		case r < 0x20:
			// Remaining C0 controls (BEL, CR, BS, VT, FF, NUL, …).
		case r >= 0x7f && r <= 0x9f:
			// DEL + C1 controls (8-bit CSI/OSC/DCS/… that were not part of a
			// parsed sequence). Runes above U+00A0 are ordinary text and kept.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeModelsNames strips control sequences from every /api/tags name as
// the list is stored, so list rows, detail lookups, delete targets, chat
// headers, and the model picker all carry one clean representation. A hostile
// name with control bytes is unusable against Ollama anyway (a matching model
// cannot exist), so the sanitized name is also the one used for API calls:
// the request fails cleanly instead of the raw bytes leaking into a styled
// row or an error echo.
func sanitizeModelNames(models []ollama.Model) []ollama.Model {
	for i := range models {
		models[i].Name = sanitizeTerminalText(models[i].Name)
	}
	return models
}

// sanitizeDetails returns a copy of a /api/show payload with every
// remote-derived string field sanitized. It is applied when the show result
// is stored, so the inspect pane (facts, caps, parameters, template, model
// file, license, model-info key/value lines) renders only clean text.
func sanitizeDetails(d ollama.Details) ollama.Details {
	d.License = sanitizeTerminalText(d.License)
	d.Modelfile = sanitizeTerminalText(d.Modelfile)
	d.Parameters = sanitizeTerminalText(d.Parameters)
	d.Template = sanitizeTerminalText(d.Template)
	for i, c := range d.Capabilities {
		d.Capabilities[i] = sanitizeTerminalText(c)
	}
	d.Details.ParentModel = sanitizeTerminalText(d.Details.ParentModel)
	d.Details.Format = sanitizeTerminalText(d.Details.Format)
	d.Details.Family = sanitizeTerminalText(d.Details.Family)
	d.Details.ParameterSize = sanitizeTerminalText(d.Details.ParameterSize)
	d.Details.QuantizationLevel = sanitizeTerminalText(d.Details.QuantizationLevel)
	for i, f := range d.Details.Families {
		d.Details.Families[i] = sanitizeTerminalText(f)
	}
	d.ModelInfo = sanitizeInfoMap(d.ModelInfo)
	d.ProjectorInfo = sanitizeInfoMap(d.ProjectorInfo)
	return d
}

// sanitizeInfoMap rebuilds a model-info map with sanitized keys and string
// leaf values (model_info renders as "key = value" lines, so keys are
// terminal-reachable too).
func sanitizeInfoMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return m
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[sanitizeTerminalText(k)] = sanitizeInfoValue(v)
	}
	return out
}

func sanitizeInfoValue(v any) any {
	switch t := v.(type) {
	case string:
		return sanitizeTerminalText(t)
	case map[string]any:
		return sanitizeInfoMap(t)
	case []any:
		for i := range t {
			t[i] = sanitizeInfoValue(t[i])
		}
	case []string:
		for i := range t {
			t[i] = sanitizeTerminalText(t[i])
		}
	}
	return v
}
