package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
)

// writeFile helper creates a config file and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	c := Default()
	if c.Host != "http://localhost:11434" {
		t.Errorf("Host = %q, want default base URL", c.Host)
	}
	if c.Theme != "dark" {
		t.Errorf("Theme = %q, want dark", c.Theme)
	}
	if c.Agent.Temperature != 0.7 || c.Agent.NumCtx != 4096 || c.Agent.MaxToolIterations != 12 {
		t.Errorf("agent defaults off: %+v", c.Agent)
	}
}

// withXDGConfigHome points the adrg/xdg base-directory state at dir for the
// duration of the test and restores the prior XDG_CONFIG_HOME and package
// state afterwards, so no test ever touches the real user config. xdg caches
// its base directories in package vars at init, so an env change alone is not
// enough: Reload() must be called after setting and after restoring the env.
func withXDGConfigHome(t *testing.T, dir string) {
	t.Helper()
	prev, had := os.LookupEnv("XDG_CONFIG_HOME")
	prevHome, prevDirs := xdg.ConfigHome, xdg.ConfigDirs
	if err := os.Setenv("XDG_CONFIG_HOME", dir); err != nil {
		t.Fatal(err)
	}
	xdg.Reload()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("XDG_CONFIG_HOME", prev)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
		xdg.Reload()
		xdg.ConfigHome, xdg.ConfigDirs = prevHome, prevDirs
	})
}

// TestLoadResolvesDefaultXDGConfigPath proves Load(Overrides{}) — the shape
// main's entrypoint produces when -config is omitted (nil ConfigPath) —
// resolves and reads $XDG_CONFIG_HOME/selftui/config.toml, and that
// ConfigPath() reports the resolved path. This is the config-package half of
// the H-01 regression: with no -config flag the persisted default file must
// actually load on startup.
func TestLoadResolvesDefaultXDGConfigPath(t *testing.T) {
	dir := t.TempDir()
	withXDGConfigHome(t, dir)

	path := filepath.Join(dir, "selftui", "config.toml")
	content := "host = \"http://192.168.7.7:11434\"\ntheme = \"light\"\ndefault_model = \"qwen3:0.6b\"\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := Load(Overrides{})
	if err != nil {
		t.Fatalf("Load(Overrides{}) with default XDG file: %v", err)
	}
	if c.Host != "http://192.168.7.7:11434" {
		t.Errorf("Host = %q, want the default-file value", c.Host)
	}
	if c.Theme != "light" || c.DefaultModel != "qwen3:0.6b" {
		t.Errorf("file fields = theme %q default_model %q, want light / qwen3:0.6b", c.Theme, c.DefaultModel)
	}
	if got := c.ConfigPath(); got != path {
		t.Errorf("ConfigPath() = %q, want resolved default %q", got, path)
	}
}

// TestSaveToDefaultXDGPathReloadsOnFreshLoad proves a Settings save lands in
// the resolved default XDG file and a subsequent Load(Overrides{}) — an
// ordinary startup with no -config flag — picks it up. This closes the H-01
// save→restart loop: the file Save writes must be the file a default startup
// reads.
func TestSaveToDefaultXDGPathReloadsOnFreshLoad(t *testing.T) {
	dir := t.TempDir()
	withXDGConfigHome(t, dir)

	c := Default()
	c.Host = "https://persist.example:11434"
	c.Theme = "light"
	c.DefaultModel = "qwen3:8b"
	// filePath deliberately empty: Save resolves the default XDG path itself.
	if err := Save(c); err != nil {
		t.Fatalf("Save(default-path config): %v", err)
	}

	reloaded, err := Load(Overrides{})
	if err != nil {
		t.Fatalf("Load(Overrides{}) after save: %v", err)
	}
	if reloaded.Host != c.Host || reloaded.Theme != c.Theme || reloaded.DefaultModel != c.DefaultModel {
		t.Errorf("reloaded fields = host %q theme %q default_model %q, want the saved values",
			reloaded.Host, reloaded.Theme, reloaded.DefaultModel)
	}
	want := filepath.Join(dir, "selftui", "config.toml")
	if got := reloaded.ConfigPath(); got != want {
		t.Errorf("reloaded ConfigPath() = %q, want %q", got, want)
	}
}

func TestFileOnly(t *testing.T) {
	p := writeFile(t, "host = \"http://192.168.1.50:11434\"\ntheme = \"light\"\ndefault_model = \"qwen3:0.6b\"\n\n[agent]\ntemperature = 0.21\ntop_p = 0.91\nnum_ctx = 2048\nmax_tool_iterations = 9\nsystem_prompt = \"test agent prompt\"\n")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "http://192.168.1.50:11434" {
		t.Errorf("Host = %q", c.Host)
	}
	if c.Theme != "light" {
		t.Errorf("Theme = %q", c.Theme)
	}
	if c.DefaultModel != "qwen3:0.6b" {
		t.Errorf("DefaultModel = %q", c.DefaultModel)
	}
	if c.Agent.Temperature != 0.21 || c.Agent.TopP != 0.91 || c.Agent.NumCtx != 2048 || c.Agent.MaxToolIterations != 9 {
		t.Errorf("agent fields = %+v", c.Agent)
	}
	if got := c.ConfigPath(); got != p {
		t.Errorf("ConfigPath = %q, want %q", got, p)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	p := writeFile(t, "host = \"http://file.example:11434\"\ntheme = \"light\"\n")
	t.Setenv(envPrefix+"HOST", "https://env.example:11434")
	t.Setenv(envPrefix+"AUTH_TOKEN", "secret")
	t.Setenv(envPrefix+"AGENT_TEMPERATURE", "0.11")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "https://env.example:11434" {
		t.Errorf("Host = %q, want https://env.example:11434 (env beats file)", c.Host)
	}
	if c.AuthToken != "secret" {
		t.Errorf("AuthToken = %q", c.AuthToken)
	}
	if c.Agent.Temperature != 0.11 {
		t.Errorf("Temperature = %v", c.Agent.Temperature)
	}
	if c.Theme != "light" {
		t.Errorf("Theme = %q, want light (file value kept when env unset)", c.Theme)
	}
}

func TestOverridesWinEverything(t *testing.T) {
	p := writeFile(t, "host = \"http://file.example:11434\"\ntheme = \"dark\"\n[agent]\ntemperature = 0.11\n")
	t.Setenv(envPrefix+"HOST", "http://env.example:11434")
	flagHost := "https://flag.example:11434"
	flagTheme := "light"
	flagTemp := 0.33
	flagTopP := 0.88
	flagNumCtx := 1234
	flagModel := "qwen3:latest"
	flagIter := 15
	flagPrompt := "custom agent"
	c, err := Load(Overrides{
		ConfigPath:        &p,
		Host:              &flagHost,
		Theme:             &flagTheme,
		Temperature:       &flagTemp,
		TopP:              &flagTopP,
		NumCtx:            &flagNumCtx,
		DefaultModel:      &flagModel,
		MaxToolIterations: &flagIter,
		SystemPrompt:      &flagPrompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "https://flag.example:11434" {
		t.Errorf("Host = %q, want https://flag.example:11434 (flags beat env + file)", c.Host)
	}
	if c.Theme != "light" {
		t.Errorf("Theme = %q, want light", c.Theme)
	}
	if c.Agent.Temperature != 0.33 || c.Agent.TopP != 0.88 || c.Agent.NumCtx != 1234 || c.Agent.MaxToolIterations != 15 {
		t.Errorf("agent fields = %+v", c.Agent)
	}
	if c.DefaultModel != "qwen3:latest" {
		t.Errorf("DefaultModel = %q", c.DefaultModel)
	}
	if c.Agent.SystemPrompt != "custom agent" {
		t.Errorf("SystemPrompt = %q", c.Agent.SystemPrompt)
	}
}

func TestMissingFileFallsBackToDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.toml")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != Default().Host || c.Theme != Default().Theme {
		t.Errorf("expected defaults on missing file, got host=%q theme=%q", c.Host, c.Theme)
	}
	if got := c.ConfigPath(); got != p {
		t.Errorf("ConfigPath = %q, want %q", got, p)
	}
}

func TestSaveWritesConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "saved.toml")
	c := Default()
	c.filePath = p
	c.Host = "https://example.test:11434"
	c.AuthToken = "abc123"
	c.Theme = "light"
	c.DefaultModel = "gemma3:12b"
	c.WorkspaceRoot = t.TempDir() // must exist: non-empty workspace_root is validated
	c.Agent.Temperature = 0.33
	c.Agent.TopP = 0.81
	c.Agent.NumCtx = 1234
	c.Agent.MaxToolIterations = 9
	c.Agent.SystemPrompt = "assistant for tests"

	if err := Save(c); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatalf("Load() error after Save: %v", err)
	}
	if loaded.Host != c.Host {
		t.Errorf("Host = %q, want %q", loaded.Host, c.Host)
	}
	if loaded.AuthToken != c.AuthToken {
		t.Errorf("AuthToken = %q, want %q", loaded.AuthToken, c.AuthToken)
	}
	if loaded.Agent.Temperature != c.Agent.Temperature || loaded.Agent.TopP != c.Agent.TopP || loaded.Agent.MaxToolIterations != c.Agent.MaxToolIterations {
		t.Errorf("agent values = %+v", loaded.Agent)
	}
	if loaded.Agent.NumCtx != c.Agent.NumCtx {
		t.Errorf("NumCtx = %d, want %d", loaded.Agent.NumCtx, c.Agent.NumCtx)
	}
	if loaded.Agent.SystemPrompt != c.Agent.SystemPrompt {
		t.Errorf("SystemPrompt = %q, want %q", loaded.Agent.SystemPrompt, c.Agent.SystemPrompt)
	}
}

func TestInvalidFileErrors(t *testing.T) {
	p := writeFile(t, "host = [unclosed")
	if _, err := Load(Overrides{ConfigPath: &p}); err == nil {
		t.Fatal("want error on malformed TOML, got nil")
	}
}
