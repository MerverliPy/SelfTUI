package main

import (
	"bytes"
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
