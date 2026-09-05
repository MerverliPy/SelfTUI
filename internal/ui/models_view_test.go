package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"selftui/internal/ollama"
)

// fakeShowServer serves POST /api/show; showFn decides the response.
func fakeShowServer(t *testing.T, showFn func(w http.ResponseWriter, name string)) (*ollama.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stray traffic to the fake host (this machine's localhost port
		// prober sends GET / at freshly bound ports, phase-5 LEDGER note)
		// must never fail a test for bytes the client did not send; a real
		// client mistake still fails through the client's own 404 error.
		if r.URL.Path != "/api/show" {
			w.WriteHeader(404)
			return
		}
		var req struct{ Name string }
		if err := readJSON(r, &req); err != nil {
			t.Fatalf("read request: %v", err)
		}
		showFn(w, req.Name)
	}))
	t.Cleanup(srv.Close)
	return ollama.New(srv.URL, ""), srv
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(out)
}

// fakeDeleteServer serves DELETE /api/delete; deleteFn decides the response.
func fakeDeleteServer(t *testing.T, deleteFn func(w http.ResponseWriter, name string)) (*ollama.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/delete" {
			w.WriteHeader(404)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := readJSON(r, &req); err != nil {
			t.Fatalf("read request: %v", err)
		}
		deleteFn(w, req.Name)
	}))
	t.Cleanup(srv.Close)
	return ollama.New(srv.URL, ""), srv
}

// fakePullServer serves POST /api/pull, streaming body verbatim.
func fakePullServer(t *testing.T, body string) (*ollama.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/pull" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return ollama.New(srv.URL, ""), srv
}

func testModels(t *testing.T, client *ollama.Client) ModelsView {
	t.Helper()
	if client == nil {
		client = ollama.New("http://localhost:1", "")
	}
	v := NewModelsView(client, NewStyles("dark"), "dark")
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	return v
}

func sampleModels() []ollama.Model {
	return []ollama.Model{
		{Name: "qwen3:8b", Family: "qwen3", ParameterSize: "8.2B", Quantization: "Q4_K_M",
			SizeBytes: 5150000000, ModifiedAt: time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)},
		{Name: "gemma3:12b", Family: "gemma3", ParameterSize: "12B", Quantization: "Q4_K_M",
			SizeBytes: 8123456789, ModifiedAt: time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC)},
	}
}

func TestModelsViewLoadsList(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	out := stripANSI(v.View())
	for _, name := range []string{"qwen3:8b", "gemma3:12b"} {
		if !strings.Contains(out, name) {
			t.Errorf("list missing %q:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "8.2B") || !strings.Contains(out, "Q4_K_M") {
		t.Errorf("expected summary details on rows:\n%s", out)
	}
	// The detail pane must NOT be open on the compact device until enter.
	if strings.Contains(out, "Apache-2.0") {
		t.Errorf("detail pane visible on compact before inspect:\n%s", out)
	}
}

func TestModelsViewEmptyHint(t *testing.T) {
	v := testModels(t, nil)
	out := stripANSI(v.View())
	if !strings.Contains(out, "no models installed") {
		t.Errorf("expected empty-state hint, got:\n%s", out)
	}
}

func TestModelsViewLoadError(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadErrMsg{err: "connection refused"})
	out := stripANSI(v.View())
	if !strings.Contains(out, "connection refused") || !strings.Contains(out, "r to retry") {
		t.Errorf("expected error + retry hint, got:\n%s", out)
	}
}

func TestModelsViewSelectionMoves(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	if idx := v.list.Index(); idx != 0 {
		t.Fatalf("initial Index = %d, want 0", idx)
	}

	v, _ = v.Update(tea.KeyPressMsg{Text: "j"})
	if idx := v.list.Index(); idx != 1 {
		t.Errorf("Index after j = %d, want 1", idx)
	}
	v, _ = v.Update(tea.KeyPressMsg{Text: "k"})
	if idx := v.list.Index(); idx != 0 {
		t.Errorf("Index after k = %d, want 0", idx)
	}
}

func TestModelsViewEnterInspectsOnCompact(t *testing.T) {
	client, _ := fakeShowServer(t, func(w http.ResponseWriter, name string) {
		if name != "qwen3:8b" {
			t.Errorf("show requested for %q, want qwen3:8b", name)
		}
		w.Write([]byte(showFixture))
	})

	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	// enter opens the pane and starts a show fetch.
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.showPane {
		t.Error("showPane = false after enter, want true")
	}
	if cmd == nil {
		t.Fatal("enter: expected show command, got nil")
	}

	msg := cmd()
	ev, ok := msg.(modelsEventMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want modelsEventMsg envelope", msg)
	}
	show, ok := ev.msg.(modelsShowMsg)
	if !ok {
		t.Fatalf("envelope payload = %T, want modelsShowMsg", ev.msg)
	}
	if show.name != "qwen3:8b" {
		t.Errorf("show name = %q, want qwen3:8b", show.name)
	}
	v, _ = v.Update(show)

	out := stripANSI(v.View())
	for _, want := range []string{"qwen3:8b", "family qwen3", "param 8.2B", "temperature 0.7", "Apache-2.0", "completion, tools"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "4.8 GB") {
		t.Errorf("expected human size, got:\n%s", out)
	}
}

func TestModelsViewEnterTogglesPaneOff(t *testing.T) {
	client, _ := fakeShowServer(t, func(w http.ResponseWriter, name string) {
		w.Write([]byte(showFixture))
	})
	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter: expected command")
	}
	v, _ = v.Update(cmd())

	// enter again on the same model closes the pane instead of refetching.
	v, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.showPane {
		t.Error("showPane = true after second enter, want false (toggle off)")
	}
	if cmd != nil {
		t.Errorf("second enter returned command %v, want nil", cmd)
	}
}

func TestModelsViewAutoInspectsOnWide(t *testing.T) {
	client, _ := fakeShowServer(t, func(w http.ResponseWriter, name string) {
		w.Write([]byte(showFixture))
	})
	v := NewModelsView(client, NewStyles("dark"), "dark")
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	v, cmd := v.Update(modelsLoadedMsg{list: sampleModels()})
	if cmd == nil {
		t.Fatal("wide load: expected auto-inspect command, got nil")
	}
	if !v.loadingShow {
		t.Error("loadingShow = false after auto-inspect, want true")
	}
}

func TestModelsViewInspectError(t *testing.T) {
	client, _ := fakeShowServer(t, func(w http.ResponseWriter, name string) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"model '%s' not found"}`, name)
	})
	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter: expected command")
	}
	v, _ = v.Update(cmd())

	out := stripANSI(v.View())
	if !strings.Contains(out, "model 'qwen3:8b' not found") {
		t.Errorf("expected surfaced show error, got:\n%s", out)
	}
}

func TestModelsViewDetailScroll(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	// Feed a long detail directly (no HTTP needed) so the pane overflows.
	details := longDetails()
	v, _ = v.Update(modelsShowMsg{name: "qwen3:8b", details: details})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open the stacked pane

	wantMax := v.maxScroll()
	if wantMax < 1 {
		t.Fatalf("long fixture should overflow the pane (maxScroll = %d)", wantMax)
	}

	if v.scroll != 0 {
		t.Fatalf("initial scroll = %d, want 0", v.scroll)
	}
	v, _ = v.Update(tea.KeyPressMsg{Text: "d"})
	if v.scroll != 1 {
		t.Errorf("scroll after d = %d, want 1", v.scroll)
	}
	v, _ = v.Update(tea.KeyPressMsg{Text: "u"})
	if v.scroll != 0 {
		t.Errorf("scroll after u = %d, want 0", v.scroll)
	}
	// Never below zero.
	v, _ = v.Update(tea.KeyPressMsg{Text: "u"})
	if v.scroll != 0 {
		t.Errorf("scroll after extra u = %d, want 0 (clamped)", v.scroll)
	}
	// Never beyond the last line.
	for i := 0; i < wantMax+50; i++ {
		v, _ = v.Update(tea.KeyPressMsg{Text: "d"})
	}
	if v.scroll != wantMax {
		t.Errorf("scroll after many d = %d, want max %d (clamped)", v.scroll, wantMax)
	}
}

// longDetails is a detail payload whose modelfile overflows a phone-height
// pane, giving the detail scroll somewhere to go.
func longDetails() ollama.Details {
	var modelfile strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&modelfile, "RUN parameter stop \"<|token_%d|>\"\n", i)
	}
	return ollama.Details{
		License:      "Apache-2.0",
		Modelfile:    modelfile.String(),
		Parameters:   "temperature 0.7",
		Template:     "{{ .Prompt }}",
		Capabilities: []string{"completion", "tools"},
	}
}

func TestModelsViewReload(t *testing.T) {
	v := testModels(t, nil)
	v, cmd := v.Update(tea.KeyPressMsg{Text: "r"})
	if !v.loading {
		t.Error("loading = false after r, want true")
	}
	if cmd == nil {
		t.Fatal("r: expected reload command, got nil")
	}
}

func TestModelsViewNoFetchOnEmptyList(t *testing.T) {
	// j/k on an empty list must not panic and must not start a fetch.
	v := testModels(t, nil)
	v, _ = v.Update(tea.KeyPressMsg{Text: "j"})
	if v.loading || len(v.models) != 0 {
		t.Errorf("unexpected state after j on empty list: loading=%v models=%d", v.loading, len(v.models))
	}
}

// wrapEmoji samples used by the cell-safe wrap cases. ZWJ sequences are a
// single grapheme cluster of 2 display cells (person + ZWJ + glyph).
const (
	familyEmoji = "\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466" // 👨‍👩‍👧‍👦
	techEmoji   = "\U0001F469\u200d\U0001F4BB"                                 // 👩‍💻
)

func TestWrapLines(t *testing.T) {
	cases := []struct {
		name  string
		in    []string
		width int
		// want nil means the case is invariant-checked only (valid UTF-8,
		// every row ≤ width cells, no text loss/duplication, ANSI intact,
		// combining/ZWJ marks never stranded at a row head).
		want []string
	}{
		{"ascii-hard-split", []string{"abcdef"}, 3, []string{"abc", "def"}},
		{"ascii-word-boundary", []string{"a b c"}, 3, []string{"a b", "c"}}, // wrap at the trailing space
		{"ascii-word-boundary-room", []string{"a b c"}, 4, []string{"a b", "c"}},
		{"ascii-overflow-word", []string{"abcdefghij"}, 3, []string{"abc", "def", "ghi", "j"}},
		{"degenerate-zero-width", []string{"abc"}, 0, []string{"abc"}}, // degenerate width: passthrough

		// M-05: cell-/ANSI-aware wrapping. Wide CJK, emoji and ZWJ clusters
		// occupy 2 cells each; combining marks occupy 0 and must never be
		// split from their base rune across a row break.
		{"cjk-words", []string{"你好 世界 你好 世界 你好 世界"}, 5,
			[]string{"你好", "世界", "你好", "世界", "你好", "世界"}},
		{"cjk-no-space", []string{"你好你好你好你好你好你好"}, 5,
			[]string{"你好", "你好", "你好", "你好", "你好", "你好"}},
		{"cjk-word-wider-than-limit", []string{"字字字字字字字字"}, 3,
			[]string{"字", "字", "字", "字", "字", "字", "字", "字"}},
		{"emoji", []string{"🚀🚀🚀🚀🚀🚀"}, 5, []string{"🚀🚀", "🚀🚀", "🚀🚀"}},
		{"zwj-family", []string{familyEmoji + familyEmoji + familyEmoji}, 4,
			[]string{familyEmoji + familyEmoji, familyEmoji}},
		{"zwj-technologist", []string{techEmoji + techEmoji + techEmoji + techEmoji}, 5,
			[]string{techEmoji + techEmoji, techEmoji + techEmoji}},
		{"combining-marks", []string{strings.Repeat("e\u0301", 10)}, 5,
			[]string{strings.Repeat("e\u0301", 5), strings.Repeat("e\u0301", 5)}},
		{"combining-marks-narrow", []string{strings.Repeat("e\u0301", 10)}, 3,
			[]string{strings.Repeat("e\u0301", 3), strings.Repeat("e\u0301", 3), strings.Repeat("e\u0301", 3), "e\u0301"}},
		{"double-combining-marks", []string{strings.Repeat("q\u0301\u0301", 9)}, 4, nil},
		{"combining-cjk-mixed", []string{strings.Repeat("e\u0301", 4) + " 中文混合 text with 標點符號 and caf\u00e9"}, 8, nil},
		{"styled-fits-passthrough", []string{"\x1b[1mhi there\x1b[0m"}, 80, []string{"\x1b[1mhi there\x1b[0m"}},
		{"styled-ascii-overflow", []string{"\x1b[1mthe quick brown fox jumps over the lazy dog\x1b[0m"}, 12,
			[]string{"\x1b[1mthe quick", "brown fox", "jumps over", "the lazy dog\x1b[0m"}},
		{"styled-cjk-overflow", []string{"\x1b[31m样式 样式 样式 样式 样式\x1b[0m"}, 9,
			[]string{"\x1b[31m样式 样式", "样式 样式", "样式\x1b[0m"}},
		{"styled-long-payload", []string{"\x1b[32m" + strings.Repeat("payload data ", 30) + "\x1b[0m"}, 24, nil},
		{"mixed-ascii-unicode-ansi", []string{"\x1b[33mwarning: 錯誤 狀態 code 0x1F680 你你你你 you are here\x1b[0m"}, 10, nil},
	}
	for _, c := range cases {
		got := wrapLines(c.in, c.width)
		if c.want != nil {
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("%s: wrapLines(%q, %d) = %q (joined %q), want %q",
					c.name, strings.Join(c.in, "\n"), c.width, got, strings.Join(got, "|"), strings.Join(c.want, "|"))
			}
		}
		checkWrapRowInvariants(t, c.name, c.width, got, strings.Join(c.in, "\n"))
	}
}

// checkWrapRowInvariants asserts the M-05 contract on every wrapped row: valid
// UTF-8, at most width display cells, no row stranded with a combining/ZWJ
// mark at its head (which would visually detach the mark from its base), no
// ANSI opener split from its parameters/reset, and visible text preserved.
func checkWrapRowInvariants(t *testing.T, name string, width int, rows []string, in string) {
	t.Helper()
	for i, r := range rows {
		if !utf8.ValidString(r) {
			t.Errorf("%s: row %d is not valid UTF-8: %q", name, i, r)
		}
		if width > 0 && lipgloss.Width(r) > width {
			t.Errorf("%s: row %d is %d cells wide (limit %d): %q", name, i, lipgloss.Width(r), width, r)
		}
	}
	for i, r := range rows {
		if i == 0 {
			continue // the original text may itself begin with a mark
		}
		if rr, _ := utf8.DecodeRuneInString(rows[i][ansiHeadLen(rows[i]):]); rr == '\u200d' ||
			unicode.Is(unicode.Mn, rr) || unicode.Is(unicode.Mc, rr) || unicode.Is(unicode.Me, rr) {
			t.Errorf("%s: row %d begins with a detached mark %q (from %q): %q", name, i, string(rr), in, r)
		}
	}
	// No ANSI sequence may be split across a row boundary: an ESC that opens
	// on a row must reach its final byte on the same row, or the opener's
	// parameters are lost and its final byte renders as stray text.
	for i, r := range rows {
		open := false
		for _, c := range r {
			if c == '\x1b' {
				if open {
					t.Errorf("%s: row %d contains a stray ESC mid-sequence: %q", name, i, r)
				}
				open = true
				continue
			}
			if open && isAnsiFinal(c) {
				open = false
			}
		}
		if open {
			t.Errorf("%s: row %d ends inside an ANSI sequence (opener split): %q", name, i, r)
		}
	}
	if strings.TrimSpace(stripANSI(strings.Join(rows, "\n"))) == "" {
		if len(rows) > 0 && rows[0] != "" {
			t.Errorf("%s: wrap erased all visible content", name)
		}
	}
	if got, want := canonicalVisible(strings.Join(rows, "\n")), canonicalVisible(in); got != want {
		t.Errorf("%s: visible text changed across wrapping:\n got %q\nwant %q", name, got, want)
	}
}

// canonicalVisible is the whitespace-insensitive visible form of s used to
// prove wrapping neither loses nor duplicates text: ANSI is stripped and all
// whitespace (including the row breaks wrapping may substitute for it) is
// removed, so wide/combining/ZWJ content must survive in exact rune order.
func canonicalVisible(s string) string {
	s = stripANSI(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{5150000000, "4.8 GB"},
		{8123456789, "7.6 GB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// showFixture mirrors a real POST /api/show response.
const showFixture = `{
  "license": "Apache-2.0",
  "modelfile": "# Modelfile generated by ollama show\nFROM qwen3:8b",
  "parameters": "temperature 0.7",
  "template": "{{ .Prompt }}",
  "details": {
    "parent_model": "",
    "format": "gguf",
    "family": "qwen3",
    "families": ["qwen3"],
    "parameter_size": "8.2B",
    "quantization_level": "Q4_K_M"
  },
  "model_info": {
    "general.architecture": "qwen3",
    "general.parameter_count": 8240000000
  },
  "projector_info": {},
  "capabilities": ["completion", "tools"]
}`

// --- M1b: delete + pull flows ----------------------------------------------

func TestModelsViewDeleteConfirmFlow(t *testing.T) {
	deleted := ""
	client, _ := fakeDeleteServer(t, func(w http.ResponseWriter, name string) {
		deleted = name
		w.Write([]byte(`{"status":"success"}`))
	})
	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	// x opens the confirm dialog on the selected model.
	v, cmd := v.Update(tea.KeyPressMsg{Text: "x"})
	if !v.confirmDelete || v.deleteTarget != "qwen3:8b" {
		t.Fatalf("confirmDelete=%v target=%q", v.confirmDelete, v.deleteTarget)
	}
	if cmd != nil {
		t.Errorf("x: unexpected cmd %v", cmd)
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "Delete qwen3:8b?") || !strings.Contains(out, "y confirm") {
		t.Errorf("confirm dialog missing text:\n%s", out)
	}

	// y fires the DELETE command (nothing hit the server yet).
	v, cmd = v.Update(tea.KeyPressMsg{Text: "y"})
	if !v.deleting {
		t.Fatal("deleting = false after y")
	}
	if deleted != "" {
		t.Errorf("Delete called before cmd() ran: %q", deleted)
	}
	if cmd == nil {
		t.Fatal("y: expected delete command")
	}

	// M-07: from approval until the DELETE completes, the busy overlay must
	// render — deleting title/spinner with the exact target — and the
	// interactive list must not appear as the active body (the audit's gap:
	// View() had no deleting branch, so the frame fell back to the list).
	out = stripANSI(v.View())
	for _, want := range []string{"Deleting qwen3:8b", "deleting qwen3:8b…"} {
		if !strings.Contains(out, want) {
			t.Errorf("deleting overlay missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, stripANSI(v.spinner.View())) {
		t.Errorf("deleting overlay missing the spinner frame:\n%s", out)
	}
	if strings.Contains(out, "gemma3:12b") {
		t.Errorf("interactive model list visible while deleting:\n%s", out)
	}
	// The busy state ignores keys: list navigation neither moves nor dismisses it.
	idx := v.list.Index()
	v, _ = v.Update(tea.KeyPressMsg{Text: "j"})
	if v.list.Index() != idx || !v.deleting {
		t.Errorf("keys acted on the list while deleting (idx %d → %d, deleting=%v)",
			idx, v.list.Index(), v.deleting)
	}

	// The command performs the DELETE and posts the result (batched with the
	// dialog spinner tick, which the tea runtime unwraps — so does the test).
	dm, ok := deleteResultFromCmd(cmd)
	if !ok {
		t.Fatal("y: command did not produce a delete result")
	}
	if deleted != "qwen3:8b" {
		t.Errorf("Delete called with %q, want qwen3:8b", deleted)
	}

	v, cmd = v.Update(dm)
	if v.deleting || v.confirmDelete {
		t.Error("dialog should close after a successful delete")
	}
	if v.notice != "deleted qwen3:8b" {
		t.Errorf("notice = %q", v.notice)
	}
	if cmd == nil {
		t.Fatal("expected reload command after delete")
	}
}

// TestModelsViewDeleteBusyOverlayFitsGeometries renders the in-flight delete
// overlay at both canonical terminal geometries and asserts the busy frame is
// bounded — exactly bodyH rows, no row wider than the terminal — while showing
// the deleting title/spinner with the target model and hiding the list body.
func TestModelsViewDeleteBusyOverlayFitsGeometries(t *testing.T) {
	for _, geom := range [][2]int{{72, 30}, {120, 40}} {
		w, h := geom[0], geom[1]
		v := testModels(t, nil)
		v, _ = v.Update(tea.WindowSizeMsg{Width: w, Height: h})
		v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
		v, cmd := v.Update(tea.KeyPressMsg{Text: "x"})
		if cmd != nil {
			t.Errorf("%dx%d: x returned a command", w, h)
		}
		v, cmd = v.Update(tea.KeyPressMsg{Text: "y"})
		if !v.deleting || v.deleteTarget != "qwen3:8b" {
			t.Fatalf("%dx%d: after y deleting=%v target=%q", w, h, v.deleting, v.deleteTarget)
		}
		if cmd == nil {
			t.Fatalf("%dx%d: y returned no delete command", w, h)
		}

		out := stripANSI(v.View())
		rows := frameRows(out)
		if len(rows) != h-2 {
			t.Errorf("%dx%d: deleting overlay body is %d rows, want exactly %d (body height)",
				w, h, len(rows), h-2)
		}
		for i, r := range rows {
			if lw := lipgloss.Width(r); lw > w {
				t.Errorf("%dx%d: deleting overlay row %d is %d columns wide (terminal %d): %q",
					w, h, i+1, lw, w, truncate(r, 48))
			}
		}
		for _, want := range []string{"Deleting qwen3:8b", "deleting qwen3:8b…"} {
			if !strings.Contains(out, want) {
				t.Errorf("%dx%d: busy overlay missing %q:\n%s", w, h, want, out)
			}
		}
		if !strings.Contains(out, stripANSI(v.spinner.View())) {
			t.Errorf("%dx%d: busy overlay missing the spinner frame:\n%s", w, h, out)
		}
		if strings.Contains(out, "gemma3:12b") {
			t.Errorf("%dx%d: interactive model list visible under the busy overlay:\n%s", w, h, out)
		}
	}
}

func TestModelsViewDeleteDialogGestures(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	// esc cancels.
	v, _ = v.Update(tea.KeyPressMsg{Text: "x"})
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.confirmDelete {
		t.Error("esc: confirm dialog should close")
	}
	if cmd != nil {
		t.Errorf("esc: unexpected cmd %v", cmd)
	}

	// n cancels too.
	v, _ = v.Update(tea.KeyPressMsg{Text: "x"})
	v, _ = v.Update(tea.KeyPressMsg{Text: "n"})
	if v.confirmDelete {
		t.Error("n: confirm dialog should close")
	}
}

func TestModelsViewDeleteErrorSurfaced(t *testing.T) {
	client, _ := fakeDeleteServer(t, func(w http.ResponseWriter, name string) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"delete failed: %s"}`, name)
	})
	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	v, _ = v.Update(tea.KeyPressMsg{Text: "x"})
	v, cmd := v.Update(tea.KeyPressMsg{Text: "y"})
	if cmd == nil {
		t.Fatal("y: expected command")
	}
	dm, ok := deleteResultFromCmd(cmd)
	if !ok || dm.err == "" {
		t.Fatalf("delete command result missing or without error: %+v", dm)
	}
	v, _ = v.Update(dm)

	// The dialog stays open with the error inline so the user can retry.
	if !v.confirmDelete || v.deleteErr == "" {
		t.Error("dialog should stay open with inline error")
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "delete failed: qwen3:8b") {
		t.Errorf("error not surfaced in the dialog:\n%s", out)
	}
}

func TestModelsViewDeleteWithNoModels(t *testing.T) {
	v := testModels(t, nil)
	v, cmd := v.Update(tea.KeyPressMsg{Text: "x"})
	if v.confirmDelete || cmd != nil {
		t.Error("x on an empty list must not open the confirm dialog")
	}
}

func TestModelsViewDialogSwallowsNavKeys(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Text: "x"})
	v, _ = v.Update(tea.KeyPressMsg{Text: "j"}) // list nav while confirming
	if idx := v.list.Index(); idx != 0 {
		t.Errorf("list moved to %d while the confirm dialog is open", idx)
	}
}

// deleteResultFromCmd runs a command (and any batch of sub-commands the
// runtime would execute) and extracts the modelsDeleteDoneMsg result from the
// modelsEventMsg envelope every async command now returns.
func deleteResultFromCmd(cmd tea.Cmd) (modelsDeleteDoneMsg, bool) {
	if cmd == nil {
		return modelsDeleteDoneMsg{}, false
	}
	msg := cmd()
	switch m := msg.(type) {
	case modelsEventMsg:
		if dm, ok := m.msg.(modelsDeleteDoneMsg); ok {
			return dm, true
		}
	case tea.BatchMsg:
		for _, sub := range m {
			if dm, ok := deleteResultFromCmd(sub); ok {
				return dm, true
			}
		}
	}
	return modelsDeleteDoneMsg{}, false
}

// pullUIFixture is a short NDJSON pull stream (2 events + success).
const pullUIFixture = `{"status":"pulling manifest"}
` +
	`{"status":"pulling layer","digest":"sha256:abc","total":100,"completed":40}
` +
	`{"status":"success"}
`

func TestModelsViewPullInputAndSuccess(t *testing.T) {
	client, _ := fakePullServer(t, pullUIFixture)
	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	// p opens the name input.
	v, cmd := v.Update(tea.KeyPressMsg{Text: "p"})
	if !v.inputMode {
		t.Fatal("p: input mode should open")
	}
	// Typing goes into the textinput.
	v, _ = v.Update(tea.KeyPressMsg{Text: "qwen3:0.6b"})
	if v.input.Value() != "qwen3:0.6b" {
		t.Errorf("input value = %q", v.input.Value())
	}
	// enter starts the pull and returns the subscription command.
	v, cmd = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.pulling || v.pullName != "qwen3:0.6b" {
		t.Fatalf("pulling=%v name=%q", v.pulling, v.pullName)
	}
	if cmd == nil {
		t.Fatal("enter: expected subscription command")
	}
	if v.inputMode {
		t.Error("input mode should close on enter")
	}

	// Mid-pull: a synthetic progress message renders spinner + progress bar.
	v, cmd = v.Update(modelsPullMsg{name: "qwen3:0.6b",
		progress: ollama.PullProgress{Status: "pulling layer", Digest: "sha256:abc", Total: 100, Completed: 40}})
	if v.pullStatus != "pulling layer" {
		t.Errorf("pullStatus = %q", v.pullStatus)
	}
	if cmd == nil {
		t.Fatal("progress: expected resubscribed command")
	}
	out := stripANSI(v.View())
	for _, want := range []string{"Pulling qwen3:0.6b", "pulling layer", "40%", "40 B / 100 B", "esc cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("pull dialog missing %q:\n%s", want, out)
		}
	}

	// Drain the real stream to completion. The tea runtime unwraps batched
	// cmds and delivers pull messages directly, so the test reads the
	// activity channel itself rather than calling the batch cmd.
	steps := 0
	for v.pulling {
		select {
		case msg := <-v.pullCh:
			steps++
			if steps > 10 {
				t.Fatal("pull stream did not finish")
			}
			v, cmd = v.Update(msg)
		case <-time.After(2 * time.Second):
			t.Fatal("pull stream stalled")
		}
	}
	if v.notice != "pulled qwen3:0.6b" {
		t.Errorf("notice = %q", v.notice)
	}
	if cmd == nil {
		t.Fatal("expected reload command after a successful pull")
	}
}

func TestModelsViewPullEmptyNameIgnored(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Text: "p"})
	if !v.inputMode {
		t.Fatal("p: input mode should open")
	}
	// enter with an empty name stays in the input, no pull starts.
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.inputMode {
		t.Error("enter with empty name should stay in input mode")
	}
	if v.pulling || cmd != nil {
		t.Error("no pull should start on an empty name")
	}
	// esc aborts cleanly.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.inputMode {
		t.Error("esc should close input mode")
	}
}

func TestModelsViewPullEscCancels(t *testing.T) {
	// A server that streams one line then stalls until the client leaves.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"status":"pulling manifest"}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	client := ollama.New(srv.URL, "")

	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Text: "p"})
	v, _ = v.Update(tea.KeyPressMsg{Text: "qwen3:big"})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.pulling {
		t.Fatal("enter: pull should start")
	}

	// esc asks the pull goroutine to stop; the stream reports cancellation.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.pullCancel == nil {
		t.Fatal("pullCancel missing after start")
	}
	v.pullCancel()

	steps := 0
	for v.pulling {
		select {
		case msg := <-v.pullCh:
			steps++
			if steps > 100 {
				t.Fatal("pull did not finish after cancellation")
			}
			v, _ = v.Update(msg)
		case <-time.After(2 * time.Second):
			t.Fatal("pull stream did not finish after cancellation")
		}
	}
	if v.pullErr == "" {
		t.Error("expected a surfaced cancellation error")
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "context canceled") || !strings.Contains(out, "qwen3:8b") {
		t.Errorf("error not surfaced above the list:\n%s", out)
	}

	// A refresh clears the stale error.
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	if v.pullErr != "" {
		t.Error("reload should clear the pull error")
	}
}

func TestModelsViewPullErrorSurfaced(t *testing.T) {
	client, _ := fakePullServer(t, `{"status":"pulling manifest"}
`+`{"error":"pull model manifest: file does not exist"}
`)
	v := testModels(t, client)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	v, _ = v.Update(tea.KeyPressMsg{Text: "p"})
	v, _ = v.Update(tea.KeyPressMsg{Text: "nope"})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.pulling {
		t.Fatal("enter: pull should start")
	}

	steps := 0
	for v.pulling {
		select {
		case msg := <-v.pullCh:
			steps++
			if steps > 10 {
				t.Fatal("pull stream did not finish")
			}
			v, _ = v.Update(msg)
		case <-time.After(2 * time.Second):
			t.Fatal("pull stream stalled")
		}
	}
	if v.pullErr == "" {
		t.Fatal("expected surfaced pull error")
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "pull model manifest: file does not exist") {
		t.Errorf("in-band pull error not surfaced:\n%s", out)
	}
}

func TestModelsViewLegendHint(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	out := stripANSI(v.View())
	for _, want := range []string{"x delete", "p pull", "r refresh"} {
		if !strings.Contains(out, want) {
			t.Errorf("legend hint missing %q:\n%s", want, out)
		}
	}
}

// Model names contain digits; an open dialog must keep the global 1/2/3 tab
// keys out of the way so the digits land in the input instead.
func TestModelsViewModalBlocksTabKeys(t *testing.T) {
	v := testModels(t, nil)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Text: "p"})
	if !v.ModalOpen() {
		t.Fatal("ModalOpen() = false while the input dialog is open")
	}

	// '3' while typing a model name must reach the textinput, not the shell.
	v, _ = v.Update(tea.KeyPressMsg{Text: "qwen3:0.6b"})
	if v.input.Value() != "qwen3:0.6b" {
		t.Errorf("input value = %q, want qwen3:0.6b (digit swallowed by tab jump?)", v.input.Value())
	}

	// And a standalone '3' must not navigate while the dialog is open.
	v, _ = v.Update(tea.KeyPressMsg{Text: "3"})
	if v.input.Value() != "qwen3:0.6b3" {
		t.Errorf("input value = %q, want qwen3:0.6b3", v.input.Value())
	}
}

// --- M-03: stale async model/host responses -------------------------------

// showStaleAFixture is the /api/show body the fake host returns for the model
// under A (qwen3:8b). The "MODEL-A-MARKER" license distinguishes an A detail
// from the list rows and from B's detail in the stale-response tests.
const showStaleAFixture = `{
  "license": "MODEL-A-MARKER qwen3-license",
  "modelfile": "FROM qwen3:8b\n# stale A",
  "parameters": "temperature 0.7",
  "template": "{{ .Prompt }}",
  "details": {
    "parent_model": "",
    "format": "gguf",
    "family": "qwen3",
    "families": ["qwen3"],
    "parameter_size": "8.2B",
    "quantization_level": "Q4_K_M"
  },
  "model_info": {"general.architecture": "qwen3"},
  "projector_info": {},
  "capabilities": ["completion", "tools"]
}`

// showBFixture is the /api/show body for gemma3:12b; its MODEL-B-MARKER
// license must never be replaced by a stale MODEL-A-MARKER detail.
const showBFixture = `{
  "license": "MODEL-B-MARKER gemma3-license",
  "modelfile": "FROM gemma3:12b\n# fresh B",
  "parameters": "temperature 0.2",
  "template": "{{ .Prompt }}",
  "details": {
    "parent_model": "",
    "format": "gguf",
    "family": "gemma3",
    "families": ["gemma3"],
    "parameter_size": "12B",
    "quantization_level": "Q4_K_M"
  },
  "model_info": {"general.architecture": "gemma3"},
  "projector_info": {},
  "capabilities": ["completion"]
}`

// tagsServer serves GET /api/tags listing exactly the named models, so a
// caller can distinguish which host a real list result came from.
func tagsServer(t *testing.T, names ...string) (*ollama.Client, *httptest.Server) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"models":[`)
	for i, n := range names {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"modified_at":"2026-09-03T08:00:00Z","size":1,"details":{}}`, n)
	}
	b.WriteString(`]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, b.String())
	}))
	t.Cleanup(srv.Close)
	return ollama.New(srv.URL, ""), srv
}

// runBatchToShow executes a tea command (and any batch of sub-commands) and
// returns the first modelsShowMsg it produced. This is how a test captures a
// /api/show result in a controlled order without racing the real HTTP
// goroutines the tea runtime would start.
func runBatchToShow(t *testing.T, cmd tea.Cmd) *modelsShowMsg {
	t.Helper()
	var found *modelsShowMsg
	var walk func(c tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		msg := c()
		switch m := msg.(type) {
		case tea.BatchMsg:
			for _, sub := range m {
				walk(sub)
			}
		case modelsEventMsg:
			switch p := m.msg.(type) {
			case modelsShowMsg:
				pp := p
				found = &pp
			}
		case modelsShowMsg:
			pp := m
			found = &pp
		}
	}
	walk(cmd)
	return found
}

// TestModelsViewStaleShowCannotReplaceNewerSelection reproduces M-03 on the
// side-by-side auto-inspect path with a controlled completion order: model
// A's /api/show is still in flight when the user moves the selection to B.
// B's result completes first, then A's arrives late — the stale A detail must
// never repaint the pane under B, and moving to B during the load must have
// requested B's detail in the first place.
func TestModelsViewStaleShowCannotReplaceNewerSelection(t *testing.T) {
	client, _ := fakeShowServer(t, func(w http.ResponseWriter, name string) {
		switch name {
		case "qwen3:8b":
			w.Write([]byte(showStaleAFixture)) // A
		case "gemma3:12b":
			w.Write([]byte(showBFixture)) // B
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	v := NewModelsView(client, NewStyles("dark"), "dark")
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40}) // side-by-side auto-inspect

	// The list lands and the auto-inspection of qwen3:8b (A) starts; its
	// result is captured but not delivered yet — A is still "in flight".
	v, cmd := v.Update(modelsLoadedMsg{list: sampleModels()})
	if !v.loadingShow {
		t.Fatal("auto-inspect of the first model did not start")
	}
	msgA := runBatchToShow(t, cmd)
	if msgA == nil || msgA.name != "qwen3:8b" {
		t.Fatalf("expected an in-flight show for qwen3:8b, got %+v", msgA)
	}

	// The user selects gemma3:12b (B) while A's show is still loading.
	v, nav := v.Update(tea.KeyPressMsg{Text: "j"})
	if v.list.Index() != 1 || v.selIdx != 1 {
		t.Fatalf("selection after j = %d/%d, want 1/1", v.list.Index(), v.selIdx)
	}

	// A's late result arrives while B is selected: it must be dropped, never
	// shown under B (current code accepts it — M-03).
	v, _ = v.Update(*msgA)
	if v.detailName == "qwen3:8b" {
		t.Fatalf("M-03: stale %q detail replaced the newer selection %q: detailName=%q index=%d",
			msgA.name, v.modelName(v.list.Index()), v.detailName, v.list.Index())
	}

	// The selection change during the load must have requested B's detail.
	if nav == nil {
		t.Error("M-03: selection change during a show must request the newly selected model")
		return
	}
	msgB := runBatchToShow(t, nav)
	if msgB == nil || msgB.name != "gemma3:12b" {
		t.Fatalf("expected a show for gemma3:12b after the selection change, got %+v", msgB)
	}
	v, _ = v.Update(*msgB)

	// B's detail is displayed; re-delivering the stale A result after B's
	// completion must still not overwrite it.
	v, _ = v.Update(*msgA)
	if v.detailName != "gemma3:12b" {
		t.Errorf("detailName = %q, want gemma3:12b", v.detailName)
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "MODEL-B-MARKER") {
		t.Errorf("expected B's detail under the B selection:\n%s", out)
	}
	if strings.Contains(out, "MODEL-A-MARKER") {
		t.Errorf("stale A detail leaked under the B selection:\n%s", out)
	}
}

// TestModelsViewApplyClientRejectsOldHostResults reproduces M-03's host-swap
// shape: an old host's list request is in flight when the user saves a new
// host in Settings (ApplyClient). The new host's list lands first; the old
// host's late result must not replace it.
func TestModelsViewApplyClientRejectsOldHostResults(t *testing.T) {
	oldClient, _ := tagsServer(t, "old-host-model")
	newClient, _ := tagsServer(t, "new-host-model")

	v := testModels(t, oldClient)

	// An old-host list fetch completes in the background before ApplyClient
	// is called; its result is captured (it was "in flight" across the swap).
	staleEv := v.loadCmd()()
	staleList, ok := staleEv.(modelsEventMsg).msg.(modelsLoadedMsg)
	if !ok || len(staleList.list) != 1 || staleList.list[0].Name != "old-host-model" {
		t.Fatalf("captured stale load = %#v", staleList)
	}

	// The user saves a new host: ApplyClient reloads from the new client.
	v, cmd := v.ApplyClient(newClient)
	if cmd == nil {
		t.Fatal("ApplyClient: expected a reload command")
	}
	freshEv := cmd()
	freshList, ok := freshEv.(modelsEventMsg).msg.(modelsLoadedMsg)
	if !ok {
		t.Fatalf("ApplyClient reload produced %T, want modelsLoadedMsg", freshEv)
	}
	v, _ = v.Update(freshList)
	if len(v.models) != 1 || v.models[0].Name != "new-host-model" {
		t.Fatalf("new-host list not applied: %v", modelNames(v.models))
	}

	// The old host's result arrives late: it must be ignored.
	v, _ = v.Update(staleList)
	if len(v.models) != 1 || v.models[0].Name != "new-host-model" {
		t.Errorf("M-03: old-host list result replaced the new host's list after ApplyClient: %v", modelNames(v.models))
	}
}

// TestModelsViewApplyClientRejectsOldHostShow is the /api/show half of the
// host swap: a detail fetch started against the old host completes after
// ApplyClient and must not populate the pane for the new host's state.
func TestModelsViewApplyClientRejectsOldHostShow(t *testing.T) {
	oldClient, _ := fakeShowServer(t, func(w http.ResponseWriter, name string) {
		w.Write([]byte(showStaleAFixture))
	})
	newClient, _ := tagsServer(t, "new-host-model")

	v := testModels(t, oldClient)
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})

	// Inspect the selected model; the show is in flight when the host swaps.
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	staleShow := runBatchToShow(t, cmd)
	if staleShow == nil || staleShow.name != "qwen3:8b" {
		t.Fatalf("expected an in-flight show for qwen3:8b, got %+v", staleShow)
	}

	v, cmd = v.ApplyClient(newClient)
	if cmd == nil {
		t.Fatal("ApplyClient: expected a reload command")
	}
	freshEv := cmd()
	freshList, ok := freshEv.(modelsEventMsg).msg.(modelsLoadedMsg)
	if !ok {
		t.Fatalf("ApplyClient reload produced %T, want modelsLoadedMsg", freshEv)
	}
	v, _ = v.Update(freshList)
	if len(v.models) != 1 || v.models[0].Name != "new-host-model" {
		t.Fatalf("new-host list not applied: %v", modelNames(v.models))
	}

	// The old host's detail arrives late: it must not populate the pane.
	v, _ = v.Update(*staleShow)
	if v.detail != nil || v.detailName != "" {
		t.Errorf("M-03: old-host show result populated the pane after ApplyClient: detailName=%q", v.detailName)
	}
}

func modelNames(models []ollama.Model) []string {
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Name
	}
	return names
}

// P1-3 regression: after a reload removes the inspected model, the compact
// stacked detail pane must not paint the new selection's header over the old
// model's payload. Inspect model A (detail for A), reload with only model B
// present (cursor clamps to B), and the pane must show B's own facts, never
// A's under B's name.
func TestModelsViewCompactReloadDropsStaleDetail(t *testing.T) {
	shows := map[string]string{}
	client, _ := fakeShowServer(t, func(w http.ResponseWriter, name string) {
		// Per-name payload so A's facts are distinguishable from B's.
		payload, ok := shows[name]
		if !ok {
			payload = fmt.Sprintf(`{"parameters":"temperature 0.7","details":{"family":"%s","parameter_size":"9B","quantization_level":"Q4_K_M"},"capabilities":["completion"]}`, name)
		}
		w.Write([]byte(payload))
	})
	v := NewModelsView(client, NewStyles("dark"), "dark")
	// Compact stacked geometry (the measured device width).
	v, _ = v.Update(tea.WindowSizeMsg{Width: 72, Height: 30})

	// Load [A=qwen3:8b, B=gemma3:12b]; enter inspects A.
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter: expected show command")
	}
	if ev, ok := cmd().(modelsEventMsg); !ok {
		t.Fatalf("enter cmd() = %T, want modelsEventMsg", cmd())
	} else if show, ok := ev.msg.(modelsShowMsg); !ok || show.name != "qwen3:8b" {
		t.Fatalf("enter cmd() payload = %#v, want show for qwen3:8b", ev.msg)
	} else {
		v, _ = v.Update(show) // A's detail lands
	}
	if v.detailName != "qwen3:8b" || v.detail == nil {
		t.Fatalf("after inspect: detailName=%q detail=%v, want qwen3:8b + payload", v.detailName, v.detail)
	}

	// Reload with only gemma3:12b present (the inspected A is gone). The list
	// cursor clamps to index 0 = gemma3:12b. The stale A payload must not
	// survive to render B's header over A's facts.
	v, cmd = v.Update(modelsLoadedMsg{list: sampleModels()[1:]})
	if v.detailName != "" || v.detail != nil {
		t.Errorf("after reload: stale detail survived (detailName=%q detail!=nil=%v); want it dropped", v.detailName, v.detail != nil)
	}
	if cmd == nil {
		t.Fatal("reload with open pane: expected a show command for the new selection, got nil")
	}
	if ev, ok := cmd().(modelsEventMsg); !ok {
		t.Fatalf("reload cmd() = %T, want modelsEventMsg", cmd())
	} else if show, ok := ev.msg.(modelsShowMsg); !ok || show.name != "gemma3:12b" {
		t.Fatalf("reload cmd() payload = %#v, want show for gemma3:12b", ev.msg)
	} else {
		v, _ = v.Update(show) // B's detail lands
	}
	if v.detailName != "gemma3:12b" {
		t.Errorf("detailName after reload = %q, want gemma3:12b", v.detailName)
	}
	out := stripANSI(v.View())
	if strings.Contains(out, "family qwen3") || strings.Contains(out, "qwen3:8b") {
		t.Errorf("stale A facts/header leaked into the pane:\n%s", out)
	}
	for _, want := range []string{"gemma3:12b", "family gemma3"} {
		if !strings.Contains(out, want) {
			t.Errorf("pane missing %q after reload:\n%s", want, out)
		}
	}
}
