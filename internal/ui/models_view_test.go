package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/ollama"
)

// fakeShowServer serves POST /api/show; showFn decides the response.
func fakeShowServer(t *testing.T, showFn func(w http.ResponseWriter, name string)) (*ollama.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			t.Errorf("path = %s, want /api/show", r.URL.Path)
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
	show, ok := msg.(modelsShowMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want modelsShowMsg", msg)
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

func TestWrapLines(t *testing.T) {
	cases := []struct {
		in    []string
		width int
		want  []string
	}{
		{[]string{"abcdef"}, 3, []string{"abc", "def"}},
		{[]string{"a b c"}, 3, []string{"a b", "c"}}, // wrap at the trailing space
		{[]string{"a b c"}, 4, []string{"a b", "c"}},
		{[]string{"abc"}, 0, []string{"abc"}}, // degenerate width: passthrough
	}
	for _, c := range cases {
		got := wrapLines(c.in, c.width)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrapLines(%v, %d) = %q (joined %q), want %q", c.in, c.width, got, strings.Join(got, "|"), c.want)
		}
	}
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
