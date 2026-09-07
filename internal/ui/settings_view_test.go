package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

// settingsApp builds an App whose session config lives at a temp file path so
// a settings save can be observed without touching the real XDG location.
func settingsApp(t *testing.T) (App, config.Config) {
	t.Helper()
	dir := t.TempDir()
	// The config file exists (possibly empty = defaults): an explicit path
	// that does not exist is now a hard load error (P1-13), and these flows
	// model an install whose Settings tab will persist into this file.
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	m := New(&cfg, NewStyles(cfg.Theme), nil)
	return m, cfg
}

// drive delivers messages through the model and replays whatever commands the
// runtime would re-feed (bounded, mirroring bubbletea's cmd → msg loop).
// Hop policy is domain-driven rather than purely timer-driven: while the
// settings form is still editing, every pending command is animation or
// navigation noise (the form's cursor blink restarts a 530 ms self-scheduling
// chain on each keypress, and spinner ticks do the same) so those hops keep a
// short grace and are dropped; the moment the form completes
// (settingsSaving/settingsSaved) the pending command is real work — the
// off-loop config write that reports settingsSaveDoneMsg, or the model reload
// after a host change — which is awaited for real, because dropping a slow
// write strands the form on "writing config…" (the documented flake family).
func drive(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()
	cur := m
	for _, msg := range msgs {
		nm, cmd := cur.Update(msg)
		cur = nm
		for hops := 0; cmd != nil && hops < 8; hops++ {
			next := execHop(cmd, saving(cur))
			if next == nil {
				break
			}
			nm, cmd = cur.Update(next)
			cur = nm
		}
	}
	return cur
}

// saving reports whether the model is mid-save or freshly saved, i.e. whether
// the next pending command is real work that must be awaited (see drive).
func saving(m tea.Model) bool {
	app, ok := m.(App)
	if !ok {
		return false
	}
	return app.settings.state == settingsSaving || app.settings.state == settingsSaved
}

// execHop runs one command. When real is true it is awaited up to the cap and
// its message is returned (the cap is a deadlock guard; a slow config write is
// still delivered). When real is false the command is treated as presentation
// noise: anything that does not answer within the grace window is dropped, and
// known periodic ticks (spinner) are dropped even when they answer fast, so a
// self-rescheduling animation chain can never stall the synchronous driver.
func execHop(cmd tea.Cmd, real bool) tea.Msg {
	type result struct {
		msg tea.Msg
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{msg: cmd()}
	}()
	if !real {
		select {
		case r := <-ch:
			if isPeriodicTick(r.msg) {
				return nil
			}
			return r.msg
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	}
	select {
	case r := <-ch:
		return r.msg
	case <-time.After(2 * time.Second):
		return nil
	}
}

// isPeriodicTick reports whether a message is a self-rescheduling animation
// tick (the bubbles spinner). These are presentation noise for the synchronous
// driver: dropping them loses no state, while feeding them would make every
// hop block for the full interval.
func isPeriodicTick(msg tea.Msg) bool {
	switch msg.(type) {
	case spinner.TickMsg:
		return true
	}
	return false
}

func openSettings(t *testing.T, m App) App {
	t.Helper()
	m = updateTab(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "3"})
	if !m.settings.Editing() {
		t.Fatal("Settings tab did not open an editing form")
	}
	return m
}

func TestSettingsBeginsFormOverCurrentConfig(t *testing.T) {
	m, _ := settingsApp(t)
	m = openSettings(t, m)

	got := view(t, m)
	for _, want := range []string{"Connection", "Host", "Auth token"} {
		if !strings.Contains(got, want) {
			t.Errorf("settings view missing %q:\n%s", want, got)
		}
	}
	// Form seeded from the session config.
	s := m.settings
	if s.val.host != m.cfg.Host {
		t.Errorf("host field = %q, want config %q", s.val.host, m.cfg.Host)
	}
	if s.val.theme != m.cfg.Theme {
		t.Errorf("theme field = %q, want config %q", s.val.theme, m.cfg.Theme)
	}
}

// TestThemePreviewEmitsLiveThemeMsg drives the form to the Theme select
// (Connection: 2 fields, Model defaults: 4 fields, then the Theme page),
// arrows Dark→Light→Dark, and asserts each change surfaces a settingsThemeMsg
// immediately (before any save) while the session config — and the file —
// stay untouched.
func TestThemePreviewEmitsLiveThemeMsg(t *testing.T) {
	m, _ := settingsApp(t)
	m = openSettings(t, m)

	// Six enters land on the Theme select (pages 1..3).
	keys := make([]tea.Msg, 0, 7)
	for i := 0; i < 6; i++ {
		keys = append(keys, tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	out := drive(t, m, append(keys, tea.KeyPressMsg{Code: tea.KeyDown})...)
	app := out.(App)
	if !app.settings.Editing() {
		t.Fatal("should still be editing after reaching the Theme page")
	}
	if app.settings.val.theme != "light" {
		t.Fatalf("theme field = %q, want light after down", app.settings.val.theme)
	}

	// Arrow back up: Light→Dark must produce a live theme message.
	ns, cmd := app.settings.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	app.settings = ns
	if cmd == nil {
		t.Fatal("expected a live theme message command after arrowing back")
	}
	if msg := cmd(); msg != nil {
		tm, ok := msg.(settingsThemeMsg)
		if !ok {
			t.Fatalf("cmd produced %T, want settingsThemeMsg", msg)
		}
		if tm.theme != "dark" {
			t.Errorf("theme msg = %q, want dark", tm.theme)
		}
	}

	// Previews must never touch the session config or write the file.
	if app.cfg.Theme != "dark" {
		t.Errorf("cfg.Theme mutated by preview: %q", app.cfg.Theme)
	}
}

// TestSettingsSubmitPersistsAndApplies walks the whole form (11 fields across
// 4 groups — the Agent group gained the workspace-tools toggle) and submits
// unchanged values: the config file is written and the
// saved panel shows (persist path of the M4 exit). Value-change/apply is
// covered by TestSettingsApplyConfigLive.
func TestSettingsSaveErrorSurfacedAndRetry(t *testing.T) {
	m, cfg := settingsApp(t)
	dir := filepath.Dir(cfg.ConfigPath())
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	m = openSettings(t, m)
	keys := make([]tea.Msg, 11)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	app := drive(t, m, keys...).(App)

	// The save failure must surface as an explicit panel, never a silent
	// drop or a crash, and the error text must be visible.
	if app.settings.state != settingsError {
		t.Fatalf("state = %v, want settingsError (save to read-only dir should fail)\n%s",
			app.settings.state, view(t, app))
	}
	got := view(t, app)
	if !strings.Contains(got, "Could not save settings") || !strings.Contains(got, "permission denied") {
		t.Errorf("expected save-error panel naming the cause, got:\n%s", got)
	}

	// Retry once the cause is fixed: enter reopens the editing form and a
	// second submit succeeds.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	app = updateTab(t, app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.settings.Editing() {
		t.Fatalf("enter on the error panel should reopen the editing form, got:\n%s", view(t, app))
	}
	keys = make([]tea.Msg, 11)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	app = drive(t, app, keys...).(App)
	if app.settings.state != settingsSaved {
		t.Errorf("retry state = %v, want saved\n%s", app.settings.state, view(t, app))
	}
}

func TestSettingsSubmitPersistsAndApplies(t *testing.T) {
	m, cfg := settingsApp(t)
	path := cfg.ConfigPath()
	m = openSettings(t, m)

	keys := make([]tea.Msg, 11)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	out := drive(t, m, keys...)
	app := out.(App)

	if app.settings.state != settingsSaved {
		t.Fatalf("state = %v, want saved (form did not complete?)\n%s", app.settings.state, view(t, app))
	}
	if app.cfg.Host != cfg.Host {
		t.Errorf("live cfg Host = %q, want unchanged %q", app.cfg.Host, cfg.Host)
	}
	if got := view(t, app); !strings.Contains(got, "Settings saved") {
		t.Errorf("expected saved panel, got:\n%s", got)
	}

	reloaded, err := config.Load(config.Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatalf("reload saved config: %v", err)
	}
	if reloaded.Host != cfg.Host {
		t.Errorf("file not persisted: host=%q want %q", reloaded.Host, cfg.Host)
	}
}

// typeField feeds each rune into the focused huh input as a key press.
func typeField(t *testing.T, m App, s string) App {
	t.Helper()
	for _, r := range s {
		m = updateTab(t, m, tea.KeyPressMsg{Text: string(r)})
	}
	return m
}

// TestSettingsFieldValidationReusesConfigPolicy drives the form to the last
// field (max tool iterations, seeded "12") and appends a digit so the value
// becomes "120" — inside the OLD settings range (1..256, accepted) but outside
// the config policy (1..100, rejected). The inline error must be
// config.Validate's stable message: the form no longer keeps a validation
// policy of its own (it once drifted: 256 here vs 100 in config).
func TestSettingsFieldValidationReusesConfigPolicy(t *testing.T) {
	m, cfg := settingsApp(t)
	m = openSettings(t, m)

	// Advance to the max-tool-iterations field (10th of 11 across 4 groups;
	// the tools toggle follows it).
	keys := make([]tea.Msg, 9)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	m = drive(t, m, keys...).(App)
	m = typeField(t, m, "0") // "12" + "0" = "120": > 100, still < 256
	app := drive(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}).(App)

	if app.settings.state != settingsEditing {
		t.Fatalf("state = %v, want editing (out-of-range value must be rejected inline)\n%s",
			app.settings.state, view(t, app))
	}
	got := view(t, app)
	if !strings.Contains(got, "max_tool_iterations must be between 1 and 100") {
		t.Errorf("expected config.Validate's stable message inline, got:\n%s", got)
	}
	if b, err := os.ReadFile(cfg.ConfigPath()); err != nil || len(b) != 0 {
		t.Errorf("config file was written despite the rejected edit (len=%d err=%v); want it still empty", len(b), err)
	}
}

// P1-4 regression: when a config write fails after the user previewed a
// different theme, the shell must roll back to the theme this editing session
// started from — a failed save must not leave the app stuck on an unsaved
// preview (the "write failure leaves in-session state untouched" contract).
// RED before the fix: the error panel showed but the previewed theme stayed
// live because only the discard path rolled previews back.
func TestThemePreviewRollsBackOnSaveFailure(t *testing.T) {
	m, cfg := settingsApp(t)
	if cfg.Theme != "dark" {
		t.Fatalf("test fixture expects a dark start theme, got %q", cfg.Theme)
	}
	dir := filepath.Dir(cfg.ConfigPath())
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	m = openSettings(t, m)

	// Walk to the Theme select (Connection 2 + Model defaults 4 fields) and
	// preview Light.
	keys := make([]tea.Msg, 0, 7)
	for i := 0; i < 6; i++ {
		keys = append(keys, tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	app := drive(t, m, append(keys, tea.KeyPressMsg{Code: tea.KeyDown})...).(App)
	if app.curTheme != "light" || app.settings.val.theme != "light" {
		t.Fatalf("preview did not apply: curTheme=%q val.theme=%q, want light/light",
			app.curTheme, app.settings.val.theme)
	}

	// Submit the remaining fields (Theme select, then Agent group). The save
	// to the read-only dir must fail and surface the error panel.
	keys = make([]tea.Msg, 5)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	app = drive(t, app, keys...).(App)
	if app.settings.state != settingsError {
		t.Fatalf("state = %v, want settingsError (save to read-only dir should fail)\n%s",
			app.settings.state, view(t, app))
	}

	// The shell must be back on the committed (start) theme, not the unsaved
	// Light preview, and the session config must be untouched.
	if app.curTheme != "dark" {
		t.Errorf("curTheme after failed save = %q, want dark (roll back the unsaved preview)", app.curTheme)
	}
	if app.cfg.Theme != "dark" {
		t.Errorf("cfg.Theme mutated by failed save = %q, want dark", app.cfg.Theme)
	}
	got := view(t, app)
	if !strings.Contains(got, "Could not save settings") {
		t.Errorf("expected the save-error panel, got:\n%s", got)
	}
}

// P1-6 regression: a settings save that only changes scalar/agent values must
// reuse the App-owned Ollama client, not construct a fresh one — otherwise the
// Agent tab silently holds a different client instance than the Models tab
// after every non-host save (the shared-client seam drifts, and future
// client-local state/policies diverge between tabs). The App is built the way
// main does: one real client at construction.
func applySavedApp(t *testing.T) (App, config.Config) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	client := ollama.New(cfg.Host, cfg.AuthToken)
	m := New(&cfg, NewStyles(cfg.Theme), client)
	return m, cfg
}

func TestApplySavedReusesClientWhenHostTokenUnchanged(t *testing.T) {
	m, _ := applySavedApp(t)

	// Capture the client each tab holds before the save.
	preAgentClient := m.agent.client
	preModelsClient := m.models.client
	if preAgentClient == nil || preModelsClient == nil {
		t.Fatal("test fixture should hold a real client on both tabs")
	}

	// A scalar-only save (temperature changed, host/token identical).
	next := *m.cfg
	next.Agent.Temperature = 0.42
	_ = m.applySaved(next)

	if m.agent.client != preAgentClient {
		t.Error("Agent client was rebuilt on a host/token-unchanged save; want the same instance reused")
	}
	if m.models.client != preModelsClient {
		t.Error("Models client was rebuilt on a host/token-unchanged save; want the same instance reused")
	}
	// The scalar still applied.
	if m.agent.temperature != 0.42 {
		t.Errorf("agent temperature = %v after save, want 0.42", m.agent.temperature)
	}
	if m.cfg.Agent.Temperature != 0.42 {
		t.Errorf("cfg temperature = %v after save, want 0.42", m.cfg.Agent.Temperature)
	}
}

// TestApplySavedSwapsClientOnHostChange proves the rebuild still happens when
// host actually changes (both tabs point at the same new instance, and it is
// not the pre-save one).
func TestApplySavedSwapsClientOnHostChange(t *testing.T) {
	m, _ := applySavedApp(t)
	old := m.agent.client
	if old == nil {
		t.Fatal("test fixture should hold a client")
	}

	next := *m.cfg
	next.Host = "https://other.example:11434"
	_ = m.applySaved(next)

	if m.agent.client == nil || m.agent.client == old {
		t.Error("Agent client was not rebuilt on a host change; want a fresh instance")
	}
	if m.models.client != m.agent.client {
		t.Error("Models and Agent tabs must share one client after a host change")
	}
	if m.cfg.Host != "https://other.example:11434" {
		t.Errorf("cfg.Host = %q after save, want the new host", m.cfg.Host)
	}
}
