package ui

// Golden render tests for M5: the full shell is rendered at the two
// canonical geometries — the measured Moshi portrait device (72x30, see
// docs/m0a-gate-evidence.md) and a wide PC window (120x40) — and the
// deterministic text of each scenario is compared against a checked-in
// fixture under testdata/golden. Regenerate fixtures with
//
//	go test ./internal/ui -run TestGoldenRender -update
//
// Fixtures store ANSI-stripped output: layout, geometry, and content drift
// fail the compare; palette/theme drift is covered by the dedicated theme
// tests below. Run with -count=1 (or after any source change) so a stale
// test cache can never mask a fixture edit.
//
// Fixtures pin a fixed workspace root (goldenApp uses "/tmp") because the
// shell renders the canonical workspace in its status rows: the row layout
// depends on the label's length, so the root must be a constant absolute
// path of stable length on every machine (never the checkout cwd).
// normalizeWorkspace additionally strips any accidental cwd text before a
// frame is stored or compared, so the fixtures can never pin the machine's
// checkout path.

import (
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/config"
	"github.com/MerverliPy/SelfTUI/internal/logsink"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
	"github.com/MerverliPy/SelfTUI/internal/session"
)

var updateGolden = flag.Bool("update", false, "regenerate golden render fixtures")

// goldenFrame names one canonical render. build returns the App in a
// deterministic, populated state (loaded lists, open detail, editing
// forms — never mid-spinner or mid-stream).
type goldenFrame struct {
	name  string
	w, h  int
	build func(t *testing.T, w, h int) App
}

// sampleDetails is a deterministic POST /api/show payload for the inspect
// panes of the golden models scenarios.
func sampleDetails() ollama.Details {
	return ollama.Details{
		License:      "Apache-2.0\n\nLicensed under the Apache License, Version 2.0 (the \"License\");\nyou may not use this file except in compliance with the License.",
		Parameters:   "num_ctx 4096\nnum_predict 2048\ntemperature 0.6",
		Modelfile:    "FROM qwen3:8b\n# generated for golden tests",
		Template:     "{{ if .System }}<system>{{ .System }}</system>{{ end }}{{ .Prompt }}",
		Capabilities: []string{"tools", "vision"},
		ModelInfo: map[string]any{
			"general.architecture":   "qwen3",
			"general.file_type":      float64(15),
			"general.parameter_size": "8.2B",
			"vision.enabled":         true,
		},
	}
}

// goldenApp builds the App for one golden frame. It differs from newTestApp
// only in the workspace root: the status rows render the canonical workspace
// (empty workspace_root → the process cwd), and the row layout depends on the
// label's length, so byte-exact frames must never render a machine-dependent
// checkout path. "/tmp" exists everywhere v0.1 runs (Linux/WSL), passes
// config validation when set, and has a stable length, keeping every frame
// identical on any machine or CI runner. No frame reads or writes through it.
func goldenApp(t *testing.T) App {
	t.Helper()
	cfg := config.Default()
	cfg.WorkspaceRoot = "/tmp"
	return New(&cfg, NewStyles(cfg.Theme), ollama.New(cfg.Host, cfg.AuthToken))
}

func bootApp(t *testing.T, w, h int) App {
	t.Helper()
	return updateTab(t, goldenApp(t), tea.WindowSizeMsg{Width: w, Height: h})
}

// lightApp builds the App for one light-theme golden frame. It
// mirrors goldenApp but forces the "light" theme so the same
// geometry/content is verified under both palettes.
func lightApp(t *testing.T) App {
	t.Helper()
	cfg := config.Default()
	cfg.WorkspaceRoot = "/tmp"
	cfg.Theme = "light"
	return New(&cfg, NewStyles("light"), ollama.New(cfg.Host, cfg.AuthToken))
}

// bootLightApp boots a light-theme App at the given geometry.
func bootLightApp(t *testing.T, w, h int) App {
	t.Helper()
	return updateTab(t, lightApp(t), tea.WindowSizeMsg{Width: w, Height: h})
}

// buildLightFrame maps every goldenFrames name to an explicit
// scenario builder for the light theme. The test fails if a name
// falls through to a default screen — every frame must have an
// explicit builder here.
func buildLightFrame(t *testing.T, name string, w, h int) App {
	switch name {
	case "models-compact":
		return buildModelsCompact(t, w, h)
	case "models-compact-inspect":
		return buildModelsCompactInspect(t, w, h)
	case "models-wide-inspect":
		return buildModelsWideInspect(t, w, h)
	case "agent-compact", "agent-wide":
		return buildAgent(t, w, h)
	case "agent-turn-compact", "agent-turn-wide":
		return buildAgentTurn(t, w, h)
	case "agent-picker-compact", "agent-picker-wide":
		return buildAgentModal(t, w, h, openModelPicker)
	case "agent-slash-compact", "agent-slash-wide":
		return buildAgentModal(t, w, h, openSlashMenu)
	case "agent-help-compact", "agent-help-wide":
		return buildAgentModal(t, w, h, openHelp)
	case "agent-clear-confirm-compact", "agent-clear-confirm-wide":
		return buildAgentModal(t, w, h, openClearConfirm)
	case "agent-batch-compact", "agent-batch-wide":
		return buildAgentBatchReview(t, w, h)
	case "agent-resume-picker-compact", "agent-resume-picker-wide":
		return buildAgentResumePicker(t, w, h)
	case "agent-resumed-compact", "agent-resumed-wide":
		return buildAgentResumed(t, w, h)
	case "palette-compact", "palette-wide":
		return buildAgentModal(t, w, h, openPalette)
	case "agent-logs-drawer-compact", "agent-logs-drawer-wide":
		m := bootLightApp(t, w, h)
		m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
		m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
		return seedLogsDrawer(t, m)
	case "settings-compact", "settings-wide":
		return buildSettingsEditing(t, w, h)
	default:
		t.Fatalf("buildLightFrame: no explicit builder for frame %q — add it before this test can pass", name)
		return App{}
	}
}

// seedLogsDrawer wires a deterministic canned ring into m and opens the
// logs drawer (ctrl+o): the fixture pins the drawer's render at each
// geometry. Lines are canned strings shaped like the real entries the N5
// sink produces (charmbracelet/log text format), written through the sink
// so they arrive exactly as live entries would.
func seedLogsDrawer(t *testing.T, m App) App {
	sink := logsink.New(io.Discard, logsink.NewRing(100), "")
	for _, l := range []string{
		"2026-09-07 10:00:00 INF selftui starting version=dev host=http://localhost:11434 theme=dark",
		"2026-09-07 10:00:01 DEBU ollama request method=GET path=/api/tags status=200 bytes=412 duration_ms=1",
		"2026-09-07 10:00:02 DEBU agent tool call tool=grep args=\"{pattern:needle,path:x.txt}\"",
		"2026-09-07 10:00:03 DEBU agent context budget messages_before=6 messages_after=4 truncated=true",
		"2026-09-07 10:00:04 WARN ollama request failed method=POST path=/api/chat err=\"connection refused\"",
	} {
		sink.Write([]byte(l + "\n"))
	}
	m = m.WithLog(nil, sink)
	return updateTab(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
}

// buildAgentLogsDrawer is the drawer fixture builder: the Agent tab (the
// every-day surface whose bottom rows the drawer covers) at each geometry.
func buildAgentLogsDrawer(t *testing.T, w, h int) App {
	m := bootApp(t, w, h)
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	return seedLogsDrawer(t, m)
}

func buildModelsCompact(t *testing.T, w, h int) App {
	m := bootApp(t, w, h)
	return updateTab(t, m, modelsEventMsg{msg: modelsLoadedMsg{list: sampleModels()}})
}

func buildModelsCompactInspect(t *testing.T, w, h int) App {
	m := buildModelsCompact(t, w, h)
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	return updateTab(t, m, modelsEventMsg{msg: modelsShowMsg{name: "qwen3:8b", details: sampleDetails()}})
}

func buildModelsWideInspect(t *testing.T, w, h int) App {
	m := buildModelsCompact(t, w, h)
	return updateTab(t, m, modelsEventMsg{msg: modelsShowMsg{name: "qwen3:8b", details: sampleDetails()}})
}

func buildAgent(t *testing.T, w, h int) App {
	m := bootApp(t, w, h)
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	return updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
}

// buildAgentTurn seeds one committed turn (user + assistant with a
// right-aligned meta header) so the transcript layout is pinned too.
func buildAgentTurn(t *testing.T, w, h int) App {
	m := buildAgent(t, w, h)
	m.agent.turns = []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "explain this repo"},
			render: m.agent.renderBlock(m.agent.userHeader(), "explain this repo")},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant,
			Content: "It is a mobile-first terminal UI for Ollama, styled like the opencode.ai TUI."},
			model: "qwen3:8b", meta: "0.4s · stop",
			render: m.agent.renderBlock(m.agent.assistantHeaderRow("qwen3:8b", "0.4s · stop"),
				"It is a mobile-first terminal UI for Ollama, styled like the opencode.ai TUI.")},
	}
	return m
}

// buildAgentModal frames the Agent tab with the given overlay open: each M7
// overlay must render inside the terminal at both canonical geometries.
func buildAgentModal(t *testing.T, w, h int, open func(t *testing.T, m App) App) App {
	m := buildAgent(t, w, h)
	return open(t, m)
}

func openModelPicker(t *testing.T, m App) App {
	return updateTab(t, m, tea.KeyPressMsg{Text: "m"})
}

func openSlashMenu(t *testing.T, m App) App {
	for _, r := range "/" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	return m
}

func openHelp(t *testing.T, m App) App {
	for _, r := range "/help" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	return updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
}

func openClearConfirm(t *testing.T, m App) App {
	// Seed a conversation so /clear has something to confirm, then type it.
	m.agent.turns = []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "explain this repo"}},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: "It is a terminal UI."},
			model: "qwen3:8b", meta: "0.4s · stop"},
	}
	for _, r := range "/clear" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	return updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
}

// buildAgentBatchReview frames the write_files review overlay (V2e): a
// deterministic multi-file batch is parked as the pending review so the new
// overlay's geometry is pinned at both canonical sizes. Diffs are canned
// rows (the runner computes real ones from the live tree); the fixture pins
// the overlay layout, not the diff algorithm.
func buildAgentBatchReview(t *testing.T, w, h int) App {
	m := buildAgent(t, w, h)
	m.agent.batchReview = &agent.BatchReviewMsg{
		Name:      "write_files",
		Workspace: "/tmp",
		Timeout:   120 * time.Second,
		Files: []agent.BatchFileReview{
			{
				Path: "internal/agent/runner.go", Kind: "edit",
				Summary: "M internal/agent/runner.go  +2 −1",
				Rows: []string{
					"  // M-06 gate",
					"-if err := ctx.Err(); err != nil {",
					"+if err := ctx.Err(); err != nil { return err }",
				},
			},
			{
				Path: "docs/v2e.md", Kind: "create",
				Summary: "A docs/v2e.md  +3",
				Rows: []string{
					"+# Multi-file batches",
					"+",
					"+All-or-nothing.",
				},
			},
		},
	}
	return m
}

func openPalette(t *testing.T, m App) App {
	return updateTab(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
}

// buildAgentResumePicker (V2a) opens the /resume picker over a seeded
// transcript directory. The transcript's mtime is pinned so the picker's
// "date · size" rows are byte-stable across machines.
func buildAgentResumePicker(t *testing.T, w, h int) App {
	m := buildAgent(t, w, h)
	dir := t.TempDir()
	l, err := session.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append("user", "qwen3:8b", "explain this repo", "", time.Date(2026, 9, 7, 21, 31, 2, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	path := l.Path()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 9, 7, 21, 45, 3, 0, time.UTC)
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	list, err := session.ListSessions(dir)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListSessions = (%v, %v)", list, err)
	}
	m.agent.sessionDir = dir
	m.agent.resumeOpen = true
	m.agent.resumeList = list
	m.agent.resumeIdx = 0
	return m
}

// buildAgentResumed (V2a) imports a saved transcript into the live Agent
// conversation: the transcript renders with its historical model chips and
// meta, and the statusline carries the resume notice.
func buildAgentResumed(t *testing.T, w, h int) App {
	m := buildAgent(t, w, h)
	m.agent.numCtx = 2048
	m.agent = m.agent.applySessionLoaded(sessionLoadedMsg{
		path: "/state/selftui/sessions/chat-20260907-214503.123-4321.md",
		turns: []session.Turn{
			{Role: "user", Model: "qwen3:8b", Content: "explain this repo"},
			{Role: "assistant", Model: "qwen3:8b", Meta: "0.4s · stop",
				Content: "It is a mobile-first terminal UI for Ollama, styled like the opencode.ai TUI."},
		},
	})
	return m
}

func buildSettingsEditing(t *testing.T, w, h int) App {
	m := bootApp(t, w, h)
	return updateTab(t, m, tea.KeyPressMsg{Text: "3"})
}

var goldenFrames = []goldenFrame{
	// Compact: the measured portrait phone (72x30).
	{"models-compact", 72, 30, buildModelsCompact},
	{"models-compact-inspect", 72, 30, buildModelsCompactInspect},
	{"agent-compact", 72, 30, buildAgent},
	{"agent-turn-compact", 72, 30, buildAgentTurn},
	{"agent-picker-compact", 72, 30, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openModelPicker) }},
	{"agent-resume-picker-compact", 72, 30, buildAgentResumePicker},
	{"agent-resumed-compact", 72, 30, buildAgentResumed},
	{"agent-slash-compact", 72, 30, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openSlashMenu) }},
	{"agent-help-compact", 72, 30, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openHelp) }},
	{"agent-clear-confirm-compact", 72, 30, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openClearConfirm) }},
	{"agent-batch-compact", 72, 30, buildAgentBatchReview},
	{"palette-compact", 72, 30, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openPalette) }},
	{"agent-logs-drawer-compact", 72, 30, buildAgentLogsDrawer},
	{"settings-compact", 72, 30, buildSettingsEditing},
	// Wide: PC window.
	{"models-wide-inspect", 120, 40, buildModelsWideInspect},
	{"agent-wide", 120, 40, buildAgent},
	{"agent-turn-wide", 120, 40, buildAgentTurn},
	{"agent-picker-wide", 120, 40, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openModelPicker) }},
	{"agent-resume-picker-wide", 120, 40, buildAgentResumePicker},
	{"agent-resumed-wide", 120, 40, buildAgentResumed},
	{"agent-slash-wide", 120, 40, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openSlashMenu) }},
	{"agent-help-wide", 120, 40, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openHelp) }},
	{"agent-clear-confirm-wide", 120, 40, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openClearConfirm) }},
	{"agent-batch-wide", 120, 40, buildAgentBatchReview},
	{"palette-wide", 120, 40, func(t *testing.T, w, h int) App { return buildAgentModal(t, w, h, openPalette) }},
	{"agent-logs-drawer-wide", 120, 40, buildAgentLogsDrawer},
	{"settings-wide", 120, 40, buildSettingsEditing},
}

// viewRows splits the raw rendered full-shell view into terminal rows.
func viewRows(m App) []string {
	return strings.Split(strings.TrimSuffix(m.View().Content, "\n"), "\n")
}

// TestGoldenFramesFitTerminal is the geometric guard behind the fixtures:
// no scenario may overflow its terminal — every rendered row must fit the
// width, and the shell must never exceed the height (a taller view scrolls
// the screen mid-render on the phone). Frame-filling views (models, agent,
// compact settings) must land on exactly h rows so tab bar + body + status
// bar stack without drift.
func TestGoldenFramesFitTerminal(t *testing.T) {
	for _, f := range goldenFrames {
		m := f.build(t, f.w, f.h)
		rows := viewRows(m)
		if len(rows) > f.h {
			t.Errorf("%s %dx%d: view is %d rows tall (terminal %d) — vertical overflow",
				f.name, f.w, f.h, len(rows), f.h)
		}
		if f.name != "settings-wide" && len(rows) != f.h {
			t.Errorf("%s %dx%d: view is %d rows tall, want exactly %d (frame must fill the terminal)",
				f.name, f.w, f.h, len(rows), f.h)
		}
		for i, l := range rows {
			if w := lipgloss.Width(l); w > f.w {
				t.Errorf("%s %dx%d: row %d is %d columns wide (terminal %d): %q",
					f.name, f.w, f.h, i+1, w, f.w, truncate(stripANSI(l), 48))
			}
		}
		if strings.TrimSpace(stripANSI(strings.Join(rows, "\n"))) == "" {
			t.Errorf("%s: frame rendered empty", f.name)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// workspaceToken is the deterministic placeholder golden fixtures use in
// place of the test process's working directory. The shell renders the
// canonical workspace — and with the default empty workspace_root that is
// the process cwd (real path, symlinks resolved) — so a fixture that stored
// the raw path byte-exactly would only pass when the suite runs from that
// exact checkout location. normalizeWorkspace maps whatever cwd the suite
// runs under to the same token, making fixtures portable (CI, a different
// checkout, another machine) and regeneration churn-free on any machine.
const workspaceToken = "<workspace>"

func normalizeWorkspace(s string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return s
	}
	s = strings.ReplaceAll(s, cwd, workspaceToken)
	if real, err := filepath.EvalSymlinks(cwd); err == nil && real != cwd {
		s = strings.ReplaceAll(s, real, workspaceToken)
	}
	return s
}

// TestGoldenRender compares each scenario's deterministic text against its
// checked-in fixture; -update rewrites the fixtures. The cwd is normalized
// away (see normalizeWorkspace) on both the write and the compare.
func TestGoldenRender(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	for _, f := range goldenFrames {
		m := f.build(t, f.w, f.h)
		got := normalizeWorkspace(stripANSI(strings.Join(viewRows(m), "\n")))
		path := filepath.Join(dir, f.name+".txt")
		if *updateGolden {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			t.Logf("updated %s", path)
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: missing fixture (run with -update): %v", f.name, err)
			continue
		}
		wantText := strings.TrimSuffix(string(want), "\n")
		if got != wantText {
			t.Errorf("%s: rendered output drifted from %s (regenerate with -update)\n%s",
				f.name, path, firstDiff(wantText, got))
		}
	}
}

// firstDiff renders a short labelled window around the first differing line.
func firstDiff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	at := 0
	for at < len(wl) && at < len(gl) && wl[at] == gl[at] {
		at++
	}
	from := at - 2
	if from < 0 {
		from = 0
	}
	var b strings.Builder
	b.WriteString("--- fixture (want) ---\n")
	for i := from; i < len(wl) && i < at+3; i++ {
		if i == at {
			b.WriteString("» ")
		}
		b.WriteString(wl[i] + "\n")
	}
	b.WriteString("--- rendered (got) ---\n")
	for i := from; i < len(gl) && i < at+3; i++ {
		if i == at {
			b.WriteString("» ")
		}
		b.WriteString(gl[i] + "\n")
	}
	return b.String()
}

// TestFitContent pins the overlay body-fitting helper used so approval and
// status dialogs can never overflow the terminal height.
func TestFitContent(t *testing.T) {
	long := make([]string, 40)
	for i := range long {
		long[i] = "payload line"
	}
	out := fitContent(long, 10)
	if len(out) != 10 {
		t.Fatalf("fitContent(40, 10) = %d rows, want 10", len(out))
	}
	if !strings.Contains(out[0], "payload line") {
		t.Errorf("head dropped: %q", out[0])
	}
	if tail := out[len(out)-1]; tail != "payload line" {
		t.Errorf("tail (action legend) not kept last: %q", tail)
	}
	marker := false
	for _, l := range out {
		if strings.Contains(l, "more lines") {
			marker = true
		}
	}
	if !marker {
		t.Errorf("missing drop marker in %q", out)
	}
	if got := fitContent([]string{"a", "b"}, 10); len(got) != 2 || got[1] != "b" {
		t.Errorf("short content must pass through untouched: %v", got)
	}
}

// TestApprovalOverlayFitsDevice drives a real write_file approval with a
// multi-KB payload and asserts the confirmation dialog stays inside the
// terminal at the measured phone geometry and the wide geometry — the
// decision legend ("y / enter approve") must be on screen.
func TestApprovalOverlayFitsDevice(t *testing.T) {
	for _, geom := range [][2]int{{72, 30}, {120, 40}} {
		w, h := geom[0], geom[1]
		root := t.TempDir()
		calls := 0
		payload := strings.Repeat("line of file content 0123456789 ", 80) // >3 KiB
		args, _ := json.Marshal(map[string]any{"path": "big.txt", "content": payload})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/tags":
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, uiTagsBody)
			case "/api/chat":
				calls++
				w.Header().Set("Content-Type", "application/x-ndjson")
				if calls == 1 {
					io.WriteString(w, `{"message":{"role":"assistant","tool_calls":[{"function":{"name":"write_file","arguments":`+string(args)+`}}]},"done":true}`+"\n")
				} else {
					io.WriteString(w, chatEvent("written", true)+"\n")
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()
		v := newAgentTools(t, ollama.New(srv.URL, ""), srv.URL, root)
		v, _ = v.Update(tea.WindowSizeMsg{Width: w, Height: h})
		v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
		typeText(t, &v, "write big")
		v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		deadline := time.After(3 * time.Second)
		for !v.ModalOpen() {
			select {
			case msg := <-v.chatCh:
				v, _ = v.Update(msg)
			case <-deadline:
				t.Fatalf("%dx%d: confirmation never arrived", w, h)
			}
		}
		body := v.View()
		rows := frameRows(body)
		if len(rows) > h-2 {
			t.Errorf("%dx%d: approval overlay body is %d rows (body height %d)", w, h, len(rows), h-2)
		}
		// The decision legend is the last content row inside the dialog
		// (the box's bottom border is the very last row) and must be on
		// screen at both geometries.
		found := false
		for _, r := range rows {
			if strings.Contains(stripANSI(r), "y / enter approve") {
				found = true
			}
		}
		if !found {
			t.Errorf("%dx%d: decision legend off screen:\n%s", w, h, stripANSI(body))
		}
		// At the phone width the payload must be truncated in the middle
		// with a marker; at the wide width the same payload wraps onto
		// fewer lines and may fit whole.
		if w == 72 {
			if out := stripANSI(body); !strings.Contains(out, "… (") || !strings.Contains(out, "more lines") {
				t.Errorf("%dx%d: expected a truncation marker for the long payload:\n%s", w, h, out)
			}
		}
	}
}

// frameRows splits a view body (no tab/status chrome) into terminal rows.
func frameRows(content string) []string {
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// TestThemePalettesDiffer: dark and light are genuinely different palettes —
// fg/bg/accent/error swap so the phone stays readable in both themes.
func TestThemePalettesDiffer(t *testing.T) {
	dark, light := NewStyles("dark"), NewStyles("light")
	pairs := []struct {
		name string
		a, b any
	}{
		{"fg", dark.fg, light.fg},
		{"bg", dark.bg, light.bg},
		{"muted", dark.muted, light.muted},
		{"accent", dark.accent, light.accent},
		{"error", dark.Error, light.Error},
	}
	for _, p := range pairs {
		if reflect.DeepEqual(p.a, p.b) {
			t.Errorf("%s palette identical between dark and light", p.name)
		}
	}
}

// TestLightThemeRendersEveryTab renders the populated shell in the light
// palette at both canonical geometries: no panics, every frame inside the
// terminal, and the tab chrome present. Each frame is built by an explicit
// builder (buildLightFrame), never a default: the test fails if a frame name
// has no builder.
func TestLightThemeRendersEveryTab(t *testing.T) {
	for _, f := range goldenFrames {
		m := buildLightFrame(t, f.name, f.w, f.h)
		// Verify the intended active tab, modal/state marker, and
		// representative content before checking geometry.
		allRows := strings.Join(viewRows(m), "\n")
		stripped := stripANSI(allRows)
		switch f.name {
		case "models-compact", "models-compact-inspect", "models-wide-inspect":
			if !strings.Contains(stripped, "Models") {
				t.Errorf("light %s: tab bar missing Models tab", f.name)
			}
			if strings.Contains(f.name, "inspect") && !strings.Contains(stripped, "qwen3:8b") {
				t.Errorf("light %s: inspect detail missing model name", f.name)
			}
		case "agent-compact", "agent-wide", "agent-turn-compact", "agent-turn-wide":
			if !strings.Contains(stripped, "Agent") {
				t.Errorf("light %s: tab bar missing Agent tab", f.name)
			}
			if strings.Contains(f.name, "agent-turn") && !strings.Contains(stripped, "It is a mobile-first") {
				t.Errorf("light %s: turn content missing", f.name)
			}
		case "agent-picker-compact", "agent-picker-wide":
			if !strings.Contains(stripped, "qwen3:8b") {
				t.Errorf("light %s: picker modal missing model list", f.name)
			}
		case "agent-slash-compact", "agent-slash-wide":
			if !strings.Contains(stripped, "/") {
				t.Errorf("light %s: slash menu missing command hint", f.name)
			}
		case "agent-help-compact", "agent-help-wide":
			if !strings.Contains(stripped, "help") {
				t.Errorf("light %s: help modal missing help text", f.name)
			}
		case "agent-clear-confirm-compact", "agent-clear-confirm-wide":
			if !strings.Contains(stripped, "clear") {
				t.Errorf("light %s: confirm modal missing confirm action", f.name)
			}
		case "agent-batch-compact", "agent-batch-wide":
			if !strings.Contains(stripped, "write_files") || !strings.Contains(stripped, "apply all") {
				t.Errorf("light %s: batch review overlay missing its content", f.name)
			}
		case "agent-resume-picker-compact", "agent-resume-picker-wide":
			if !strings.Contains(stripped, "saved chats") {
				t.Errorf("light %s: resume picker missing the picker title", f.name)
			}
		case "agent-resumed-compact", "agent-resumed-wide":
			if !strings.Contains(stripped, "It is a mobile-first") {
				t.Errorf("light %s: resumed transcript content missing", f.name)
			}
		case "palette-compact", "palette-wide":
			if !strings.Contains(stripped, "command") && !strings.Contains(stripped, "palette") {
				t.Errorf("light %s: palette missing command palette content", f.name)
			}
		case "agent-logs-drawer-compact", "agent-logs-drawer-wide":
			if !strings.Contains(stripped, "logs — debug") {
				t.Errorf("light %s: logs drawer missing the drawer title", f.name)
			}
		case "settings-compact", "settings-wide":
			if !strings.Contains(stripped, "Settings") {
				t.Errorf("light %s: tab bar missing Settings tab", f.name)
			}
		}
		rows := viewRows(m)
		if len(rows) > f.h {
			t.Errorf("light %s %dx%d: %d rows (terminal %d)", f.name, f.w, f.h, len(rows), f.h)
		}
		for i, l := range rows {
			if w := lipgloss.Width(l); w > f.w {
				t.Errorf("light %s %dx%d: row %d is %d wide (%d)", f.name, f.w, f.h, i+1, w, f.w)
			}
		}
		if !strings.Contains(stripANSI(allRows), tabLabels[0]) {
			t.Errorf("light %s: tab bar missing", f.name)
		}
	}
}

// TestLightFrameCoverage: every goldenFrames name must have an explicit
// builder in buildLightFrame. The test fails if any frame falls through
// to the default case — no frame is allowed to render a default screen.
func TestLightFrameCoverage(t *testing.T) {
	for _, f := range goldenFrames {
		// buildLightFrame t.fails with t.Fatalf for unknown names,
		// so a passing iteration proves the name has an explicit builder.
		_ = buildLightFrame(t, f.name, f.w, f.h)
	}
}

// TestLightThemeAgentChatRenders drives one markdown reply through the
func TestLightThemeAgentChatRenders(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	cfg := config.Default()
	v := NewAgentView(client, NewStyles("light"), "light", "", cfg.Agent)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 72, Height: 30})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hi")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	body := v.View()
	if out := stripANSI(body); !strings.Contains(out, "Answer") {
		t.Fatalf("light markdown reply missing from view:\n%s", out)
	}
	for i, l := range frameRows(body) {
		if w := lipgloss.Width(l); w > 72 {
			t.Errorf("light chat row %d is %d wide (72): %q", i+1, w, stripANSI(l))
		}
	}
}
