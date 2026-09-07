package ui

// N4 (PLAN §12) — status bar as observability row: the shell's bottom row
// gains the model chip, a compact context meter, the last completed turn's
// measured tok/s, and a background-job pill for streaming pulls; the context
// meter gains an amber tier (~80%) before the red-100% tier, shared by the
// composer header and the status row via ctxTierFor.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

// testAgentCtx builds an Agent view with numCtx = 1000 (input budget
// 750 tokens) and a draft sized to land the ctx meter on an exact percentage.
// With an empty system prompt and no turns, tokens = (len(draft)+3)/4.
// pct = tokens*100/750 (integer division).
func testAgentCtx(t *testing.T, draftLen int) AgentView {
	t.Helper()
	cfg := config.Default()
	cfg.Agent.NumCtx = 1000
	v := NewAgentView(ollama.New("http://localhost:1", ""), NewStyles("dark"), "dark", "", cfg.Agent)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if draftLen > 0 {
		v.input.SetValue(strings.Repeat("a", draftLen))
	}
	return v
}

func TestCtxTierBoundaries(t *testing.T) {
	// 79/80/100 on the meter's displayed scale: amber starts at 80, red at 100.
	for _, tc := range []struct {
		pct  int
		want ctxTier
	}{
		{0, ctxOK}, {79, ctxOK}, {80, ctxAmber}, {99, ctxAmber}, {100, ctxRed},
	} {
		if got := ctxTierFor(tc.pct); got != tc.want {
			t.Errorf("ctxTierFor(%d) = %d, want %d", tc.pct, got, tc.want)
		}
	}
}

func TestCtxMeterAmberTier(t *testing.T) {
	// 78% (draft 2365 chars → 592 tok → 78%): plain, byte-identical to the
	// pre-N4 muted rendering. 80% (2400 → 600 tok) and 81% (2429 → 608 tok):
	// amber. 100% (3000 → 750 tok): the red "ctx full" message, unchanged.
	// The composer header is left + pad + right, so the tiered segment is
	// asserted as the row's suffix.
	below := testAgentCtx(t, 2365)
	if below.ctxPct() != 78 {
		t.Fatalf("boundary fixture drifted: pct = %d, want 78", below.ctxPct())
	}
	if got := below.composerHeader(); !strings.HasSuffix(got, below.styles.mutedText().Render(below.meterPlainWithUsage())) {
		t.Errorf("78%% composer header is not the muted pre-N4 rendering:\n%q", stripANSI(got))
	}
	if got := below.ctxMeterSegment(); got != below.ctxMeterPlain() {
		t.Errorf("78%% status-row meter changed: %q", got)
	}

	amber := testAgentCtx(t, 2400)
	if amber.ctxPct() != 80 {
		t.Fatalf("boundary fixture drifted: pct = %d, want 80", amber.ctxPct())
	}
	if got := amber.composerHeader(); !strings.HasSuffix(got, amber.styles.warnText().Render(amber.meterPlainWithUsage())) {
		t.Errorf("80%% composer header is not amber-styled:\n%q", stripANSI(got))
	}
	if got := amber.ctxMeterSegment(); got != amber.styles.warnText().Render(amber.ctxMeterPlain()) {
		t.Errorf("80%% status-row meter is not amber-styled: %q", got)
	}

	justInside := testAgentCtx(t, 2429)
	if justInside.ctxPct() != 81 {
		t.Fatalf("boundary fixture drifted: pct = %d, want 81", justInside.ctxPct())
	}
	if got := justInside.composerHeader(); !strings.HasSuffix(got, justInside.styles.warnText().Render(justInside.meterPlainWithUsage())) {
		t.Errorf("81%% composer header is not amber-styled:\n%q", stripANSI(got))
	}

	full := testAgentCtx(t, 3000)
	if full.ctxPct() != 100 {
		t.Fatalf("boundary fixture drifted: pct = %d, want 100", full.ctxPct())
	}
	if out := stripANSI(full.composerHeader()); !strings.Contains(out, "ctx full — /clear") {
		t.Errorf("100%% composer header lost the red full message:\n%s", out)
	}
	if got := full.ctxMeterSegment(); got != full.styles.Error.Render(full.ctxMeterPlain()) {
		t.Errorf("100%% status-row meter is not red-styled: %q", got)
	}
}

// meterPlainWithUsage recomposes the composer header's right-hand plain text
// (meter + usage) the way composerHeader builds it.
func (v AgentView) meterPlainWithUsage() string {
	plain := v.ctxMeterPlain()
	if usage := v.ctxUsage(); usage != "" {
		plain += " · " + usage
	}
	return plain
}

func TestCtxMeterSegmentOmittedWithoutData(t *testing.T) {
	// No num_ctx: no meter at all.
	cfg := config.Default()
	cfg.Agent.NumCtx = 0
	v := NewAgentView(ollama.New("http://localhost:1", ""), NewStyles("dark"), "dark", "", cfg.Agent)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if got := v.ctxMeterSegment(); got != "" {
		t.Errorf("meter segment rendered without num_ctx: %q", got)
	}
	// num_ctx set but a zero-token conversation: omitted so a no-data frame
	// stays byte-identical to the pre-N4 status row.
	v2 := testAgentCtx(t, 0)
	if got := v2.ctxMeterSegment(); got != "" {
		t.Errorf("meter segment rendered for a zero-token conversation: %q", got)
	}
}

func TestStatusBarObservabilitySegments(t *testing.T) {
	m := newTestApp(t)
	// A populated agent: model selected, draft in the meter, measured tok/s
	// from the last completed turn.
	m.agent = testAgentCtx(t, 2400) // 80% → amber meter in the row
	m.agent.model = "qwen3:8b"
	m.agent.lastTokPerSec = 41
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	out := view(t, m)
	for _, want := range []string{"qwen3:8b", "41 tok/s", "ctx"} {
		if !strings.Contains(out, want) {
			t.Errorf("status row missing observability segment %q:\n%s", want, out)
		}
	}

	// Fresh app (no model selected, no measured tok/s, no pull): the new
	// segments with absent data stay absent — pre-N4 identity plus, at most,
	// the 0% meter the composer header already shows for the default system
	// prompt payload.
	fresh := updateTab(t, newTestApp(t), tea.WindowSizeMsg{Width: 120, Height: 40})
	freshOut := view(t, fresh)
	for _, banned := range []string{"tok/s", "qwen3:8b", "⇣"} {
		if strings.Contains(freshOut, banned) {
			t.Errorf("no-data status row shows %q:\n%s", banned, freshOut)
		}
	}
	want := strings.Join([]string{
		"⏻ " + fresh.cfg.Host, toolsChip(false), canonicalWorkspaceLabel(fresh.cfg.WorkspaceRoot),
	}, " · ")
	if !strings.Contains(freshOut, want) {
		t.Errorf("no-data status row drifted from the pre-N4 identity:\nwant %q in\n%s", want, freshOut)
	}
}

func TestStatusBarPullPill(t *testing.T) {
	m := newTestApp(t)
	m.models.pulling = true
	m.models.pullName = "qwen3:8b"
	m.models.pullTotal, m.models.pullDone = 1000, 450
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if out := view(t, m); !strings.Contains(out, "⇣ qwen3:8b 45%") {
		t.Errorf("pull pill missing from the status row:\n%s", out)
	}

	// No server-reported sizes yet: pill degrades to name + ellipsis.
	m2 := newTestApp(t)
	m2.models.pulling = true
	m2.models.pullName = "qwen3:8b"
	m2 = updateTab(t, m2, tea.WindowSizeMsg{Width: 120, Height: 40})
	if out := view(t, m2); !strings.Contains(out, "⇣ qwen3:8b…") {
		t.Errorf("sizeless pull pill missing:\n%s", out)
	}

	// Pill gone once the pull completes.
	m2.models.pulling = false
	if m2.models.pullPill() != "" {
		t.Error("pull pill survived pull completion")
	}
}

func TestStatusBarWidthPressure(t *testing.T) {
	// 72-col phone with every segment present: the row sheds workspace, then
	// the observability tail (tok/s → meter → pill → model chip), and keeps
	// the identity floor host · tools fully legible (no mid-word truncation).
	m := newTestApp(t)
	m.agent = testAgentCtx(t, 2400)
	m.agent.model = "qwen3:8b"
	m.agent.lastTokPerSec = 41
	m.models.pulling = true
	m.models.pullName = "qwen3:8b"
	m.models.pullTotal, m.models.pullDone = 1000, 450
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 72, Height: 30})
	out := view(t, m)
	for _, want := range []string{"⏻ http://localhost:11434", "tools off"} {
		if !strings.Contains(out, want) {
			t.Errorf("72-col status row lost identity segment %q:\n%s", want, out)
		}
	}
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	row := rows[len(rows)-1]
	for _, banned := range []string{"tok/s", "⇣", "…"} {
		if strings.Contains(row, banned) {
			t.Errorf("72-col status row kept/truncated segment %q:\n%s", banned, row)
		}
	}
	for _, want := range []string{"⏻ http://localhost:11434", "tools off"} {
		if !strings.Contains(row, want) {
			t.Errorf("72-col status row lost identity segment %q:\n%s", want, row)
		}
	}
}

func TestLastTokPerSecClearedWithConversation(t *testing.T) {
	v := testAgent(t, nil)
	v.lastTokPerSec = 41

	cleared, _ := v.clearConfirmKey(tea.Key{Text: "y", Code: tea.KeyEnter})
	if cleared.lastTokPerSec != 0 {
		t.Errorf("/clear kept the last turn's tok/s: %d", cleared.lastTokPerSec)
	}
	kept, _ := v.clearConfirmKey(tea.Key{Text: "n", Code: tea.KeyEsc})
	if kept.lastTokPerSec != 41 {
		t.Errorf("cancelled /clear dropped the last turn's tok/s: %d", kept.lastTokPerSec)
	}
}
