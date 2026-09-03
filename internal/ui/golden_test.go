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

	"selftui/internal/config"
	"selftui/internal/ollama"
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

func bootApp(t *testing.T, w, h int) App {
	t.Helper()
	return updateTab(t, newTestApp(t), tea.WindowSizeMsg{Width: w, Height: h})
}

func buildModelsCompact(t *testing.T, w, h int) App {
	m := bootApp(t, w, h)
	return updateTab(t, m, modelsLoadedMsg{list: sampleModels()})
}

func buildModelsCompactInspect(t *testing.T, w, h int) App {
	m := buildModelsCompact(t, w, h)
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	return updateTab(t, m, modelsShowMsg{name: "qwen3:8b", details: sampleDetails()})
}

func buildModelsWideInspect(t *testing.T, w, h int) App {
	m := buildModelsCompact(t, w, h)
	return updateTab(t, m, modelsShowMsg{name: "qwen3:8b", details: sampleDetails()})
}

func buildAgent(t *testing.T, w, h int) App {
	m := bootApp(t, w, h)
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	return updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})
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
	{"settings-compact", 72, 30, buildSettingsEditing},
	// Wide: PC window.
	{"models-wide-inspect", 120, 40, buildModelsWideInspect},
	{"agent-wide", 120, 40, buildAgent},
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

// TestGoldenRender compares each scenario's deterministic text against its
// checked-in fixture; -update rewrites the fixtures.
func TestGoldenRender(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	for _, f := range goldenFrames {
		m := f.build(t, f.w, f.h)
		got := stripANSI(strings.Join(viewRows(m), "\n"))
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
		cfg := config.Default()
		v := NewAgentViewWithWorkspace(ollama.New(srv.URL, ""), NewStyles("dark"), "dark", "", root, cfg.Agent)
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
// terminal, and the tab chrome present.
func TestLightThemeRendersEveryTab(t *testing.T) {
	for _, f := range goldenFrames {
		cfg := config.Default()
		cfg.Theme = "light"
		m := New(&cfg, NewStyles("light"), ollama.New(cfg.Host, cfg.AuthToken))
		m = updateTab(t, m, tea.WindowSizeMsg{Width: f.w, Height: f.h})
		switch f.name {
		case "models-compact":
			m = updateTab(t, m, modelsLoadedMsg{list: sampleModels()})
		case "models-compact-inspect":
			m = updateTab(t, m, modelsLoadedMsg{list: sampleModels()})
			m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			m = updateTab(t, m, modelsShowMsg{name: "qwen3:8b", details: sampleDetails()})
		case "models-wide-inspect":
			m = updateTab(t, m, modelsLoadedMsg{list: sampleModels()})
			m = updateTab(t, m, modelsShowMsg{name: "qwen3:8b", details: sampleDetails()})
		case "agent-compact", "agent-wide":
			m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
			m = updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})
		case "settings-compact", "settings-wide":
			m = updateTab(t, m, tea.KeyPressMsg{Text: "3"})
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
		if !strings.Contains(stripANSI(strings.Join(rows, "\n")), tabLabels[0]) {
			t.Errorf("light %s: tab bar missing", f.name)
		}
	}
}

// TestLightThemeAgentChatRenders drives one markdown reply through the
// light glamour renderer at the phone geometry (heading, bold, code block)
// and checks it renders inside the frame.
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
