package ui

// Regression (owner bug): mutation confirmations and command output were
// dropped at the root App's message routing — ToolConfirmMsg/ToolOutputMsg
// were missing from the forwarded-case list — so in the real binary the
// "Allow write_file?" dialog never appeared and the runner blocked forever on
// the approval channel (statusline stuck at "⚙ write_file … esc interrupt").
// These tests drive messages through App.Update, not AgentView directly.

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

	tea "charm.land/bubbletea/v2"

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
	m := New(&cfg, NewStyles("dark"), ollama.New(srv.URL, ""))
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})
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
	m := New(&cfg, NewStyles("dark"), ollama.New(srv.URL, ""))
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 88, Height: 40})
	m = updateTab(t, m, agentModelsLoadedMsg{models: sampleModels()})
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
