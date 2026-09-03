package config

import (
	"os"
	"path/filepath"
	"testing"
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

func TestFileOnly(t *testing.T) {
	p := writeFile(t, "host = \"http://192.168.1.50:11434\"\ntheme = \"light\"\n")
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
	if got := c.ConfigPath(); got != p {
		t.Errorf("ConfigPath = %q, want %q", got, p)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	p := writeFile(t, "host = \"file-host\"\ntheme = \"light\"\n")
	t.Setenv(envPrefix+"HOST", "env-host")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "env-host" {
		t.Errorf("Host = %q, want env-host (env beats file)", c.Host)
	}
	if c.Theme != "light" {
		t.Errorf("Theme = %q, want light (file value kept when env unset)", c.Theme)
	}
}

func TestOverridesWinEverything(t *testing.T) {
	p := writeFile(t, "host = \"file-host\"\ntheme = \"light\"\n")
	t.Setenv(envPrefix+"HOST", "env-host")
	flagHost := "flag-host"
	c, err := Load(Overrides{ConfigPath: &p, Host: &flagHost})
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "flag-host" {
		t.Errorf("Host = %q, want flag-host (flags beat env + file)", c.Host)
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
	if got := c.ConfigPath(); got != "" {
		t.Errorf("ConfigPath = %q, want empty when no file", got)
	}
}

func TestInvalidFileErrors(t *testing.T) {
	p := writeFile(t, "host = [unclosed")
	if _, err := Load(Overrides{ConfigPath: &p}); err == nil {
		t.Fatal("want error on malformed TOML, got nil")
	}
}
