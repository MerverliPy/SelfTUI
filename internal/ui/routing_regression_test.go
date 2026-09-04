package ui

// Regression (owner bug): mutation confirmations were dropped at the root
// App's message routing — ToolConfirmMsg was missing from the forwarded-case
// list — so in the real binary the "Allow write_file?" dialog never appeared
// and the runner blocked forever on the approval channel (statusline stuck at
// "⚙ write_file … esc interrupt"). These tests drive messages through
// App.Update, not AgentView directly.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"selftui/internal/agent"
	"selftui/internal/config"
	"selftui/internal/ollama"
)

// pumpAgent pumps one message from the agent's activity channel through the
// ROOT App routing (m.Update), so routing bugs surface here.
func pumpAgent(t *testing.T, m *App) App {
	t.Helper()
	select {
	case msg := <-m.agent.chatCh:
		nm, _ := m.Update(msg)
		return nm.(App)
	case <-time.After(5 * time.Second):
		t.Fatal("no agent message arrived")
		return *m
	}
}

func TestAppRoutingShowsMutationConfirm(t *testing.T) {
	root := t.TempDir()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			calls++
			w.Header().Set("Content-Type", "application/x-ndjson")
			if calls == 1 {
				args, _ := json.Marshal(map[string]any{"path": "timer.sh", "content": "#!/bin/sh\nsleep 10"})
				call := fmt.Sprintf(`{"function":{"name":"write_file","arguments":%s}}`, string(args))
				io.WriteString(w, fmt.Sprintf(`{"message":{"role":"assistant","tool_calls":[%s]},"done":true,"done_reason":"tool_calls"}`+"\n", call))
			} else {
				io.WriteString(w, chatEvent("wrote the 10s timer", true)+"\n")
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.WorkspaceRoot = root // the agent writes here, not the repo cwd
	cfg.ToolsEnabled = true  // this flow exercises the armed tool surface
	m := New(&cfg, NewStyles("dark"), ollama.New(srv.URL, ""))
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"}) // Agent tab
	if m.tab != agentTab {
		t.Fatalf("tab = %d, want Agent", m.tab)
	}

	// Send the request, then pump every agent message through the root.
	for _, r := range "write timer" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	for i := 0; i < 20 && !m.agent.ModalOpen(); i++ {
		m = pumpAgent(t, &m)
	}
	if !m.agent.ModalOpen() || m.agent.confirmation == nil {
		t.Fatal("confirmation never surfaced through the App routing (the bug): modal open =", m.agent.ModalOpen())
	}
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "Allow write_file") || !strings.Contains(out, "y / enter approve") {
		t.Fatalf("approval overlay missing through the root App:\n%s", out)
	}

	// Approve with y: the write lands and the final reply commits.
	m = updateTab(t, m, tea.KeyPressMsg{Text: "y"})
	for i := 0; i < 30 && m.agent.streaming; i++ {
		m = pumpAgent(t, &m)
	}
	if m.agent.streaming {
		t.Fatal("turn did not finish after approval")
	}
	if _, err := os.Stat(filepath.Join(root, "timer.sh")); err != nil {
		t.Fatalf("approved write_file did not create the file: %v", err)
	}
	if m.agent.chatErr != "" {
		t.Errorf("chatErr = %q", m.agent.chatErr)
	}
	if len(m.agent.history) != 2 || !strings.Contains(m.agent.history[1].Content, "wrote the 10s timer") {
		t.Errorf("history = %+v, want committed final reply", m.agent.history)
	}
	if calls != 2 {
		t.Errorf("chat calls = %d, want 2 (tool turn + final)", calls)
	}
}

func TestAppRoutingDeclineSkipsWrite(t *testing.T) {
	root := t.TempDir()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			calls++
			w.Header().Set("Content-Type", "application/x-ndjson")
			if calls == 1 {
				args, _ := json.Marshal(map[string]any{"path": "no.sh", "content": "echo no"})
				call := fmt.Sprintf(`{"function":{"name":"write_file","arguments":%s}}`, string(args))
				io.WriteString(w, fmt.Sprintf(`{"message":{"role":"assistant","tool_calls":[%s]},"done":true}`+"\n", call))
			} else {
				io.WriteString(w, chatEvent("ok, not writing it", true)+"\n")
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.WorkspaceRoot = root
	cfg.ToolsEnabled = true
	m := New(&cfg, NewStyles("dark"), ollama.New(srv.URL, ""))
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	for _, r := range "write no" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	for i := 0; i < 20 && !m.agent.ModalOpen(); i++ {
		m = pumpAgent(t, &m)
	}
	if !m.agent.ModalOpen() {
		t.Fatal("confirmation should surface (routing bug would hang here)")
	}
	m = updateTab(t, m, tea.KeyPressMsg{Text: "n"}) // decline
	for i := 0; i < 30 && m.agent.streaming; i++ {
		m = pumpAgent(t, &m)
	}
	if _, err := os.Stat(filepath.Join(root, "no.sh")); !os.IsNotExist(err) {
		t.Fatalf("declined write created the file: %v", err)
	}
	if !strings.Contains(m.agent.history[len(m.agent.history)-1].Content, "not writing it") {
		t.Errorf("final reply missing after decline: %+v", m.agent.history)
	}
}

// --- Phase 6: envelope routing ---------------------------------------------

// routingRow is one table row of the envelope routing test: an async child
// payload, the envelope that must carry it across the App shell, optional
// preconditions on the app (e.g. an in-flight stream so the payload has an
// observable effect), and an assertion that the payload actually reached the
// intended child's Update (state after Update — a payload dropped at the
// shell leaves the state untouched and fails the row).
type routingRow struct {
	name  string
	wrap  func(tea.Msg) tea.Msg // envelope constructor
	msg   tea.Msg               // the concrete async payload
	setup func(*App)
	want  func(App) error
}

// runEnvelopeRows boots a fresh app per row and pushes one enveloped payload
// through the ROOT App.Update, exactly as the tea runtime would after
// executing the child's command.
func runEnvelopeRows(t *testing.T, rows []routingRow) {
	t.Helper()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			m := newTestApp(t)
			m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
			if row.setup != nil {
				row.setup(&m)
			}
			nm, _ := m.Update(row.wrap(row.msg))
			m = nm.(App)
			if err := row.want(m); err != nil {
				t.Fatalf("payload did not reach the child through its envelope: %v", err)
			}
		})
	}
}

func errf(format string, args ...any) error { return fmt.Errorf(format, args...) }

func TestAppEnvelopeRoutingTable(t *testing.T) {
	envAgent := func(msg tea.Msg) tea.Msg { return agentEventMsg{msg: msg} }
	envModels := func(msg tea.Msg) tea.Msg { return modelsEventMsg{msg: msg} }

	streaming := func(a *App) { a.agent.streaming = true }

	t.Run("every agent async payload reaches AgentView", func(t *testing.T) {
		runEnvelopeRows(t, []routingRow{
			{"agentModelsLoadedMsg (model list)", envAgent, agentModelsLoadedMsg{models: sampleModels()}, nil,
				func(m App) error {
					if m.agent.loading || len(m.agent.models) != 2 {
						return errf("loading=%v models=%d, want loaded list", m.agent.loading, len(m.agent.models))
					}
					if m.agent.model != "qwen3:8b" {
						return errf("model = %q, want first entry selected", m.agent.model)
					}
					return nil
				}},
			{"agentModelsErrMsg (model list error)", envAgent, agentModelsErrMsg{err: "load refused"}, nil,
				func(m App) error {
					if m.agent.modelsErr != "load refused" {
						return errf("modelsErr = %q, want surfaced load error", m.agent.modelsErr)
					}
					return nil
				}},
			{"agentTokenMsg (legacy delta)", envAgent, agentTokenMsg{text: "legacy delta"}, streaming,
				func(m App) error {
					if m.agent.streamText != "legacy delta" {
						return errf("streamText = %q, want the delta appended", m.agent.streamText)
					}
					return nil
				}},
			{"agent.TokenMsg (delta)", envAgent, agent.TokenMsg{Text: "token delta"}, streaming,
				func(m App) error {
					if m.agent.streamText != "token delta" {
						return errf("streamText = %q, want the delta appended", m.agent.streamText)
					}
					return nil
				}},
			{"agent.ToolStartMsg", envAgent, agent.ToolStartMsg{Name: "read_file", Input: "a.txt"}, streaming,
				func(m App) error {
					if m.agent.toolStatus != "⚙ read_file a.txt" {
						return errf("toolStatus = %q, want tool-start activity", m.agent.toolStatus)
					}
					return nil
				}},
			{"agent.ToolResultMsg", envAgent, agent.ToolResultMsg{Name: "read_file", OK: true, Summary: "ok\nmore"}, streaming,
				func(m App) error {
					if m.agent.toolStatus != "✓ read_file: ok" {
						return errf("toolStatus = %q, want tool-result activity", m.agent.toolStatus)
					}
					return nil
				}},
			{"agent.ToolConfirmMsg (approval)", envAgent, agent.ToolConfirmMsg{Name: "write_file", Input: `{"path":"x"}`, Workspace: "/tmp/ws", Timeout: 30 * time.Second}, nil,
				func(m App) error {
					if m.agent.confirmation == nil || m.agent.confirmation.Name != "write_file" || !m.agent.ModalOpen() {
						return errf("confirmation = %+v ModalOpen=%v, want pending approval", m.agent.confirmation, m.agent.ModalOpen())
					}
					return nil
				}},
			{"agent.FallbackMsg", envAgent, agent.FallbackMsg{Reason: "plain chat fallback"}, nil,
				func(m App) error {
					if m.agent.notice != "plain chat fallback" {
						return errf("notice = %q, want fallback reason", m.agent.notice)
					}
					return nil
				}},
			{"agentDoneMsg (legacy done)", envAgent, agentDoneMsg{err: "", reason: "length"}, streaming,
				func(m App) error {
					if m.agent.streaming || m.agent.stopCancel != nil {
						return errf("streaming=%v stopCancel!=nil after done", m.agent.streaming)
					}
					return nil
				}},
			{"agent.AgentDoneMsg (done)", envAgent, agent.AgentDoneMsg{Err: "", Reason: "stop"}, streaming,
				func(m App) error {
					if m.agent.streaming || m.agent.chatCh != nil {
						return errf("streaming=%v chatCh!=nil after done", m.agent.streaming)
					}
					return nil
				}},
		})
	})

	t.Run("every models async payload reaches ModelsView", func(t *testing.T) {
		runEnvelopeRows(t, []routingRow{
			{"modelsLoadedMsg (list)", envModels, modelsLoadedMsg{list: sampleModels()}, nil,
				func(m App) error {
					if m.models.loading || len(m.models.models) != 2 {
						return errf("loading=%v models=%d, want loaded list", m.models.loading, len(m.models.models))
					}
					return nil
				}},
			{"modelsLoadErrMsg (list error)", envModels, modelsLoadErrMsg{err: "refused"}, nil,
				func(m App) error {
					if m.models.listErr != "refused" {
						return errf("listErr = %q, want surfaced load error", m.models.listErr)
					}
					return nil
				}},
			{"modelsShowMsg (details)", envModels, modelsShowMsg{name: "qwen3:8b", details: sampleDetails()}, nil,
				func(m App) error {
					if m.models.detail == nil || m.models.detailName != "qwen3:8b" || m.models.detailErr != "" {
						return errf("detail=%v name=%q err=%q, want inspect payload", m.models.detail != nil, m.models.detailName, m.models.detailErr)
					}
					return nil
				}},
			{"modelsShowErrMsg (inspect error)", envModels, modelsShowErrMsg{name: "qwen3:8b", err: "not found"}, nil,
				func(m App) error {
					if m.models.detailErr != "not found" {
						return errf("detailErr = %q, want surfaced show error", m.models.detailErr)
					}
					return nil
				}},
			{"modelsDeleteDoneMsg (delete result)", envModels, modelsDeleteDoneMsg{name: "gemma3:12b"}, nil,
				func(m App) error {
					if m.models.notice != "deleted gemma3:12b" {
						return errf("notice = %q, want delete confirmation", m.models.notice)
					}
					return nil
				}},
			{"modelsPullMsg (pull progress)", envModels, modelsPullMsg{name: "qwen3:0.6b", progress: ollama.PullProgress{Status: "pulling layer", Digest: "sha256:abc", Total: 100, Completed: 40}}, nil,
				func(m App) error {
					if m.models.pullStatus != "pulling layer" {
						return errf("pullStatus = %q, want streamed progress applied", m.models.pullStatus)
					}
					return nil
				}},
			{"modelsPullDoneMsg (pull completion)", envModels, modelsPullDoneMsg{name: "qwen3:0.6b"}, nil,
				func(m App) error {
					if m.models.pulling || m.models.notice != "pulled qwen3:0.6b" {
						return errf("pulling=%v notice=%q, want pull completion", m.models.pulling, m.models.notice)
					}
					return nil
				}},
			{"spinner.TickMsg (dialog spinner tick)", envModels, spinner.TickMsg{}, nil,
				func(m App) error {
					return nil // tick advance asserted below (needs before/after)
				}},
		})
	})

	// The spinner tick row needs before/after frames (a tick that reaches
	// ModelsView advances the dialog spinner by exactly one frame).
	t.Run("models spinner advances on an enveloped tick", func(t *testing.T) {
		m := newTestApp(t)
		m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
		before := m.models.spinner.View()
		nm, _ := m.Update(modelsEventMsg{msg: spinner.TickMsg{}})
		m = nm.(App)
		if got := m.models.spinner.View(); got == before {
			t.Fatalf("enveloped tick did not reach ModelsView (spinner frame %q unchanged)", got)
		}
	})
}

// TestUnrelatedSpinnerTickNotRoutedToModels: a bare spinner.TickMsg that was
// NOT wrapped in modelsEventMsg (an unrelated spinner, e.g. from another
// component) must not be silently routed to ModelsView. The raw type is no
// longer in the shell's routing list — it is dropped, so the Models dialog
// spinner never animates off its own events.
func TestUnrelatedSpinnerTickNotRoutedToModels(t *testing.T) {
	m := newTestApp(t)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	spinnerBefore := m.models.spinner.View()
	viewBefore := view(t, m)

	nm, cmd := m.Update(spinner.TickMsg{})
	m = nm.(App)
	if cmd != nil {
		t.Fatalf("raw spinner.TickMsg returned a command %v, want nil (nothing may resubscribe)", cmd)
	}
	if got := m.models.spinner.View(); got != spinnerBefore {
		t.Errorf("Models spinner advanced on a raw tick %q -> %q (silently routed?)", spinnerBefore, got)
	}
	if got := view(t, m); got != viewBefore {
		t.Error("shell view changed on a raw spinner tick (should be dropped)")
	}
}

// TestAppRoutingChatCompletesTurn is the chat-completion regression through
// the ROOT App: one full agent turn (typed in the Agent tab, streamed via the
// activity channel, committed on the terminal done event). Every event now
// crosses the shell as an agentEventMsg, so a turn that fails to complete
// here means the envelope resubscription loop is broken, not just a missing
// case list entry.
func TestAppRoutingChatCompletesTurn(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	cfg := config.Default()
	m := New(&cfg, NewStyles("dark"), client)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"}) // Agent tab
	if m.tab != agentTab {
		t.Fatalf("tab = %d, want Agent", m.tab)
	}

	for _, r := range "explain this" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.agent.streaming {
		t.Fatal("enter: agent turn should start")
	}
	for i := 0; i < 60 && m.agent.streaming; i++ {
		m = pumpAgent(t, &m)
	}
	if m.agent.streaming {
		t.Fatal("turn did not complete through the App shell")
	}
	if m.agent.chatErr != "" {
		t.Fatalf("chatErr = %q, want clean completion", m.agent.chatErr)
	}
	if n := len(m.agent.history); n != 2 {
		t.Fatalf("history len = %d, want 2 (user + committed assistant)", n)
	}
	if !strings.Contains(m.agent.history[1].Content, "func main()") {
		t.Errorf("committed assistant reply = %+v, want the streamed answer", m.agent.history[1])
	}
	if out := stripANSI(m.View().Content); !strings.Contains(out, "func main()") {
		t.Errorf("committed reply missing from the rendered transcript:\n%s", out)
	}
}

// TestAppRoutingPullCompletesAndReloads is the pull-completion regression
// through the ROOT App: p → name → enter starts the streamed pull, every
// progress/completion event crosses the shell as a modelsEventMsg, and the
// finished pull leaves the notice + reload state on ModelsView.
func TestAppRoutingPullCompletesAndReloads(t *testing.T) {
	client, _ := fakePullServer(t, pullUIFixture)
	cfg := config.Default()
	m := New(&cfg, NewStyles("dark"), client)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, modelsEventMsg{msg: modelsLoadedMsg{list: sampleModels()}})

	m = updateTab(t, m, tea.KeyPressMsg{Text: "p"})
	for _, r := range "qwen3:0.6b" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.models.pulling {
		t.Fatal("enter: pull should start")
	}

	steps := 0
	for m.models.pulling {
		select {
		case msg := <-m.models.pullCh:
			steps++
			if steps > 20 {
				t.Fatal("pull did not finish through the App shell")
			}
			nm, _ := m.Update(msg)
			m = nm.(App)
		case <-time.After(3 * time.Second):
			t.Fatal("pull stream stalled through the App shell")
		}
	}
	if m.models.pullErr != "" || m.models.pullStatus == "" {
		t.Errorf("pullErr=%q pullStatus=%q, want a clean streamed completion", m.models.pullErr, m.models.pullStatus)
	}
	if m.models.notice != "pulled qwen3:0.6b" {
		t.Errorf("notice = %q, want the pull-completion notice (reload cmd follows)", m.models.notice)
	}
}

// TestAppRoutingChatErrorSurfaced is the error-surfacing regression through
// the ROOT App: a rejected chat request ends the turn with the error on the
// Agent view (retry prompt), and the failure arrives as the terminal
// agentEventMsg — never dropped at the shell.
func TestAppRoutingChatErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			w.WriteHeader(404)
			io.WriteString(w, `{"error":"model 'ghost:tag' not found"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	m := New(&cfg, NewStyles("dark"), ollama.New(srv.URL, ""))
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	for _, r := range "ping" {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.agent.streaming {
		t.Fatal("enter: agent turn should start")
	}
	for i := 0; i < 40 && m.agent.streaming; i++ {
		m = pumpAgent(t, &m)
	}
	if m.agent.streaming {
		t.Fatal("turn did not end after the server error")
	}
	if !strings.Contains(m.agent.chatErr, "model 'ghost:tag' not found") {
		t.Errorf("chatErr = %q, want the server error surfaced", m.agent.chatErr)
	}
	if n := len(m.agent.history); n != 1 || m.agent.history[0].Content != "ping" {
		t.Errorf("history = %+v, want the user message kept for retry", m.agent.history)
	}
	if out := stripANSI(m.View().Content); !strings.Contains(out, "not found") {
		t.Errorf("error missing from the rendered Agent view:\n%s", out)
	}
}
