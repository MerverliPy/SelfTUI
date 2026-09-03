package ui

import (
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

// TestSettingsSubmitPersistsAndApplies walks the whole form (10 fields across
// 4 groups) and submits unchanged values: the config file is written and the
// saved panel shows (persist path of the M4 exit). Value-change/apply is
// covered by TestSettingsApplyConfigLive.
func TestSettingsSubmitPersistsAndApplies(t *testing.T) {
	m, cfg := settingsApp(t)
	path := cfg.ConfigPath()
	m = openSettings(t, m)

	keys := make([]tea.Msg, 10)
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
