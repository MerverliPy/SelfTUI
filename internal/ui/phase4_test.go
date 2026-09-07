package ui

// Phase 4 (workspace tool trust) UI surface: the status bar and the Agent
// view advertise the canonical workspace and the tools state, tools off is
// the default (the runner gets no tool definitions until the user opts in),
// and enabling tools against a non-loopback host raises a persistent warning
// that workspace content may leave the machine. The Settings toggle is the
// opt-in control; no onboarding wizard ships in v0.1.

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

// newAgentTools builds an Agent tab with workspace tools enabled (the path
// production takes when cfg.ToolsEnabled is true) so the tool loop and its
// status indicators can be exercised.
func newAgentTools(t *testing.T, client *ollama.Client, host, root string) AgentView {
	t.Helper()
	cfg := config.Default()
	if client == nil {
		client = ollama.New(host, "")
	}
	return newAgentView(nil, client, NewStyles("dark"), "dark", "", root, "", cfg.Agent, true, host)
}

func TestAgentStatusShowsToolsOffByDefault(t *testing.T) {
	v := testAgent(t, nil)
	if v.toolsEnabled {
		t.Fatal("compat Agent view must default to tools disabled")
	}
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	out := stripANSI(v.View())
	if !strings.Contains(out, "tools off") {
		t.Errorf("default statusline missing 'tools off':\n%s", out)
	}
	if strings.Contains(out, "tools on") {
		t.Errorf("default statusline shows 'tools on':\n%s", out)
	}
}

func TestAgentStatusShowsToolsAndWorkspaceWhenEnabled(t *testing.T) {
	root := t.TempDir()
	v := newAgentTools(t, nil, "http://127.0.0.1:11434", root)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	if !v.toolsEnabled {
		t.Fatal("explicitly enabled Agent view must keep tools enabled")
	}
	out := stripANSI(v.View())
	for _, want := range []string{"tools on", root} {
		if !strings.Contains(out, want) {
			t.Errorf("enabled statusline missing %q:\n%s", want, out)
		}
	}
}

func TestAgentRemoteHostWarningPersistent(t *testing.T) {
	root := t.TempDir()

	// Tools on + remote host: the persistent warning row replaces the legend
	// and names the risk — workspace content may be sent to that host.
	v := newAgentTools(t, nil, "http://ollama.lan:11434", root)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	out := stripANSI(v.View())
	for _, want := range []string{"tools on", "may be sent to", "ollama.lan"} {
		if !strings.Contains(out, want) {
			t.Errorf("remote+tools view missing %q:\n%s", want, out)
		}
	}

	// Tools on + loopback host: no warning, plain identity row.
	v = newAgentTools(t, nil, "http://localhost:11434", root)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	out = stripANSI(v.View())
	if strings.Contains(out, "may be sent to") {
		t.Errorf("loopback+tools shows a remote warning:\n%s", out)
	}

	// Tools off + remote host: no warning either (nothing leaves the machine).
	cfg := config.Default()
	host := "http://ollama.lan:11434"
	v = newAgentView(nil, ollama.New(host, ""), NewStyles("dark"), "dark", "", root, "", cfg.Agent, false, host)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	out = stripANSI(v.View())
	if strings.Contains(out, "may be sent to") {
		t.Errorf("tools-off + remote shows a warning:\n%s", out)
	}
}

func TestStatusBarShowsWorkspaceAndTools(t *testing.T) {
	// N4: the observability row sheds the workspace first under width
	// pressure (today's discipline, now triggered once the ctx meter claims
	// budget), so these cases pin a short fixed workspace — the same stable
	// absolute path the golden frames use — to keep testing the contract
	// "the row advertises the canonical workspace when it fits". The
	// long-path drop order is covered by TestStatusBarWidthPressure.
	ws := "/tmp"

	cfg := config.Default()
	cfg.WorkspaceRoot = ws
	m := New(&cfg, NewStyles(cfg.Theme), nil)
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	out := stripANSI(view(t, m))
	for _, want := range []string{"⏻ http://localhost:11434", "tools off", ws} {
		if !strings.Contains(out, want) {
			t.Errorf("default status bar missing %q:\n%s", want, out)
		}
	}

	cfg2 := config.Default()
	cfg2.WorkspaceRoot = ws
	cfg2.ToolsEnabled = true
	m2 := New(&cfg2, NewStyles(cfg2.Theme), nil)
	m2 = updateTab(t, m2, tea.WindowSizeMsg{Width: 120, Height: 40})
	out2 := stripANSI(view(t, m2))
	if !strings.Contains(out2, "tools on") || !strings.Contains(out2, ws) {
		t.Errorf("enabled status bar missing tools on / workspace:\n%s", out2)
	}

	// Tools on + remote host: the status bar itself warns.
	cfg3 := config.Default()
	cfg3.WorkspaceRoot = ws
	cfg3.ToolsEnabled = true
	cfg3.Host = "http://ollama.lan:11434"
	m3 := New(&cfg3, NewStyles(cfg3.Theme), nil)
	m3 = updateTab(t, m3, tea.WindowSizeMsg{Width: 120, Height: 40})
	out3 := stripANSI(view(t, m3))
	if !strings.Contains(out3, "may be sent to the remote host") {
		t.Errorf("remote+tools status bar missing the warning:\n%s", out3)
	}
}

func TestSettingsToolsToggleSaves(t *testing.T) {
	m, cfg := settingsApp(t)
	ws := t.TempDir()
	m.cfg.WorkspaceRoot = ws // the App owns the live config; Begin seeds from it
	path := cfg.ConfigPath()
	m = openSettings(t, m)

	// Ten enters land on the eleventh field, the tools toggle (the Agent
	// group gained it after max tool iterations).
	keys := make([]tea.Msg, 10)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	m = drive(t, m, keys...).(App)
	if !m.settings.Editing() {
		t.Fatalf("expected to still be editing on the tools toggle:\n%s", view(t, m))
	}
	if got := view(t, m); !strings.Contains(got, "Enable workspace tools") {
		t.Fatalf("tools toggle field missing:\n%s", got)
	}

	// 'y' accepts the confirm (sets true) and submits the form; the form's
	// internal next-field/submit hops are re-fed like the runtime would.
	app := drive(t, m, tea.KeyPressMsg{Text: "y"}).(App)
	if app.settings.state != settingsSaved {
		t.Fatalf("state = %v, want saved after enabling tools\n%s", app.settings.state, view(t, app))
	}
	reloaded, err := config.Load(config.Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.ToolsEnabled || reloaded.WorkspaceRoot != ws {
		t.Errorf("reloaded = %+v, want tools enabled + %q", reloaded, ws)
	}
	if !app.agent.toolsEnabled {
		t.Error("live Agent view did not switch to tools enabled after the save")
	}
}

func TestSettingsToolsToggleRejectsUnsafeWorkspace(t *testing.T) {
	m, cfg := settingsApp(t)
	path := cfg.ConfigPath()
	m = openSettings(t, m)

	keys := make([]tea.Msg, 10)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	m = drive(t, m, keys...).(App)
	if got := view(t, m); !strings.Contains(got, "Enable workspace tools") {
		t.Fatalf("tools toggle field missing:\n%s", got)
	}

	// Enabling tools with an empty workspace root must be rejected with the
	// config policy's stable message (tools need a real project root).
	app := drive(t, m, tea.KeyPressMsg{Text: "y"}).(App)
	out := view(t, app)
	if app.settings.state == settingsSaved {
		t.Fatalf("tools enabled with empty workspace_root and saved:\n%s", out)
	}
	if !strings.Contains(out, "workspace_root is required when tools are enabled") {
		t.Errorf("expected the stable config policy message inline, got:\n%s", out)
	}
	if b, err := os.ReadFile(path); err != nil || len(b) != 0 {
		t.Errorf("config file was written despite the rejected enable (len=%d err=%v); want it still empty", len(b), err)
	}
	if app.agent.toolsEnabled {
		t.Error("Agent view enabled tools despite the rejected save")
	}
}
