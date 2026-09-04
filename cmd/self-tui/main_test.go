package main

import (
	"bytes"
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionFlagOutputFormat pins the `selftui -version` formatting
// contract: the printed line is exactly "selftui <Version>\n" whatever
// Version holds. Version is a var (defaults "dev") so release tooling and
// this test can set it without touching the -version path.
func TestVersionFlagOutputFormat(t *testing.T) {
	original := Version
	defer func() { Version = original }()

	for _, v := range []string{"dev", "1.2.3-rc1", "0.7.0"} {
		Version = v
		var out bytes.Buffer
		printVersion(&out)
		if want := "selftui " + v + "\n"; out.String() != want {
			t.Errorf("printVersion() with Version=%q = %q, want %q", v, out.String(), want)
		}
	}
}

// TestConfigPathOverrideBoundary pins the H-01 contract at the entrypoint
// boundary: flag.String returns a non-nil *string even when -config is omitted
// (value ""), and that empty value must become a nil ConfigPath override so
// config.Load falls back to the default XDG file
// ($XDG_CONFIG_HOME/selftui/config.toml). A non-empty explicit -config path
// must remain a non-nil override so the documented flag precedence holds. The
// flags are parsed on a private FlagSet because run() owns the process-global
// flag set — registering the same names twice panics.
func TestConfigPathOverrideBoundary(t *testing.T) {
	fs := flag.NewFlagSet("selftui-test", flag.ContinueOnError)
	cfgFlag := fs.String("config", "", "config file path")

	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse without -config: %v", err)
	}
	if got := configPathOverride(cfgFlag); got != nil {
		t.Errorf("omitted -config: ConfigPath override = %q, want nil so Load resolves the default XDG file", *got)
	}

	explicit := filepath.Join(t.TempDir(), "explicit.toml")
	if err := fs.Parse([]string{"-config", explicit}); err != nil {
		t.Fatalf("parse -config %s: %v", explicit, err)
	}
	if got := configPathOverride(cfgFlag); got == nil {
		t.Error("explicit -config: ConfigPath override = nil, want non-nil")
	} else if *got != explicit {
		t.Errorf("explicit -config: ConfigPath override = %q, want %q", *got, explicit)
	}
}

func TestSessionDirForRun(t *testing.T) {
	t.Setenv("SELFTUI_NO_SESSION", "1")
	if got := sessionDirForRun(); got != "" {
		t.Errorf("NO_SESSION: got %q, want empty", got)
	}

	t.Setenv("SELFTUI_NO_SESSION", "")
	t.Setenv("SELFTUI_SESSION_DIR", "/tmp/custom-sessions")
	if got := sessionDirForRun(); got != "/tmp/custom-sessions" {
		t.Errorf("SESSION_DIR: got %q", got)
	}

	t.Setenv("SELFTUI_SESSION_DIR", "")
	if got := sessionDirForRun(); !strings.HasSuffix(got, "selftui/sessions") {
		t.Errorf("default: got %q, want …/selftui/sessions", got)
	}
}
