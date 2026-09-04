package main

import (
	"strings"
	"testing"
)

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
