package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
)

// settingsApp builds an App whose session config lives at a temp file path so
// a settings save can be observed without touching the real XDG location.
func settingsApp(t *testing.T) (App, config.Config) {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/config.toml"
	cfg, err := config.Load(config.Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	m := New(&cfg, NewStyles(cfg.Theme), nil)
	return m, cfg
}

// drive delivers messages through the model and replays whatever commands the
// runtime would re-feed (bounded, mirroring bubbletea's cmd → msg loop).
// Cursor-blink commands (tea.Tick) would block the synchronous loop for their
// full interval, so any hop that does not produce a message within the grace
// window is treated as presentation noise and dropped.
func drive(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()
	cur := m
	for _, msg := range msgs {
		nm, cmd := cur.Update(msg)
		cur = nm
		for hops := 0; cmd != nil && hops < 8; hops++ {
			next := execHop(cmd)
			if next == nil {
				break
			}
			nm, cmd = cur.Update(next)
			cur = nm
		}
	}
	return cur
}

// execHop runs one command with a grace window, so blink ticks that would
// block for 530ms don't stall the test driver.
func execHop(cmd tea.Cmd) tea.Msg {
	type result struct {
		msg tea.Msg
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{msg: cmd()}
	}()
	select {
	case r := <-ch:
		return r.msg
	case <-time.After(100 * time.Millisecond):
		return nil
	}
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
	if _, err := os.Stat(cfg.ConfigPath()); !os.IsNotExist(err) {
		t.Errorf("config file should not exist after a rejected edit (nothing was saved)")
	}
}
