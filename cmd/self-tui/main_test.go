package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adrg/xdg"
)

// TestLogFilePathPrecedence pins the N5 --log-file contract at the
// entrypoint boundary, flagset-style (run() owns the process-global set):
// an explicit --log-file wins verbatim, an empty flag falls back to the XDG
// state default ($XDG_STATE_HOME/selftui/log.txt), whose parent directory
// xdg.StateFile creates.
func TestLogFilePathPrecedence(t *testing.T) {
	fs := flag.NewFlagSet("selftui-test", flag.ContinueOnError)
	logFlag := fs.String("log-file", "", "debug log file path")
	if err := fs.Parse([]string{"-log-file", "/tmp/explicit-selftui.log"}); err != nil {
		t.Fatalf("parse -log-file: %v", err)
	}
	got, err := logFilePath(*logFlag)
	if err != nil {
		t.Fatalf("logFilePath(explicit): %v", err)
	}
	if got != "/tmp/explicit-selftui.log" {
		t.Errorf("logFilePath(explicit) = %q, want the flag verbatim", got)
	}

	// Empty flag value → the XDG state default (config_test's Reload pattern;
	// the state base has only a home, no dirs).
	prevHome := xdg.StateHome
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	xdg.StateHome = state
	xdg.Reload()
	defer func() {
		// Reload first (the env still points at the temp dir), then restore
		// the saved home — the same order config_test's Reload pattern uses.
		xdg.Reload()
		xdg.StateHome = prevHome
	}()

	fs2 := flag.NewFlagSet("selftui-test-default", flag.ContinueOnError)
	logFlag2 := fs2.String("log-file", "", "debug log file path")
	if err := fs2.Parse(nil); err != nil {
		t.Fatalf("parse default: %v", err)
	}
	got, err = logFilePath(*logFlag2)
	if err != nil {
		t.Fatalf("logFilePath(default): %v", err)
	}
	want := filepath.Join(state, "selftui", "log.txt")
	if got != want {
		t.Errorf("logFilePath(default) = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Dir(want)); err != nil {
		t.Errorf("log parent dir was not created: %v", err)
	}
}

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

// closeableModel is a minimal stand-in for ui.App: it exposes CloseSession
// and records whether the shutdown boundary reached it.
type closeableModel struct {
	err error
	got bool
}

func (c *closeableModel) CloseSession() error {
	c.got = true
	return c.err
}

// TestCloseSessionRecorder pins the M-04 shutdown boundary at the entrypoint:
// the final tea model is asked to flush/close the chat-transcript recorder
// exactly when it exposes CloseSession, and a nil or foreign final model is a
// harmless no-op (recording may be disabled or the model may not own one).
func TestCloseSessionRecorder(t *testing.T) {
	wantErr := errors.New("flush failed")
	m := &closeableModel{err: wantErr}
	if err := closeSessionRecorder(m); !errors.Is(err, wantErr) {
		t.Errorf("closeSessionRecorder(model with CloseSession) = %v, want %v", err, wantErr)
	}
	if !m.got {
		t.Error("CloseSession was not called on the final model")
	}

	if err := closeSessionRecorder(nil); err != nil {
		t.Errorf("closeSessionRecorder(nil) = %v, want nil", err)
	}
	type foreign struct{}
	if err := closeSessionRecorder(foreign{}); err != nil {
		t.Errorf("closeSessionRecorder(foreign model) = %v, want nil", err)
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

func TestUndoDirForRun(t *testing.T) {
	t.Setenv("SELFTUI_UNDO_DIR", "/tmp/custom-undo")
	if got := undoDirForRun(); got != "/tmp/custom-undo" {
		t.Errorf("UNDO_DIR: got %q", got)
	}
	t.Setenv("SELFTUI_UNDO_DIR", "")
	if got := undoDirForRun(); !strings.HasSuffix(got, "selftui/undo") {
		t.Errorf("default: got %q, want …/selftui/undo", got)
	}
}

// blockingModel is a final tea model whose CloseSession never returns until
// release is closed — the wedged-sink shape P1-1 targets: the recorder worker
// is stuck on a stalled filesystem write, so the shutdown flush cannot
// complete.
type blockingModel struct {
	release chan struct{}
}

func (b *blockingModel) CloseSession() error {
	<-b.release
	return nil
}

// TestCloseSessionRecorderBoundedOnWedgedSink proves the entrypoint never
// hangs on a wedged transcript sink: the bounded close must return (with a
// timeout error) shortly after its budget, instead of blocking run() forever.
// RED before the fix: closeSessionRecorder called CloseSession synchronously,
// so a stalled recorder made the process unkillable (NotifyContext still
// intercepts Ctrl+C while main awaits the close).
func TestCloseSessionRecorderBoundedOnWedgedSink(t *testing.T) {
	m := &blockingModel{release: make(chan struct{})}
	defer close(m.release) // unblock the worker goroutine when the test ends

	type res struct{ err error }
	done := make(chan res, 1)
	// Exercise the injectable budget seam directly so the test is fast and
	// deterministic; production run() uses sessionCloseTimeout.
	go func() { done <- res{err: closeSessionRecorderWithin(m, 100*time.Millisecond)} }()

	select {
	case r := <-done:
		if r.err == nil {
			t.Fatal("bounded close on a wedged recorder: want a timeout error, got nil")
		}
		if !strings.Contains(r.err.Error(), "timed out") {
			t.Errorf("error = %v, want the close-timeout message", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("closeSessionRecorder blocked forever on a wedged recorder; want a bounded timeout")
	}
}
