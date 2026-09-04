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

	tea "charm.land/bubbletea/v2"

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
