package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenCreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "sessions")
	l, err := Open(dir, "http://localhost:11434")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("session dir perms = %o, want 700", fi.Mode().Perm())
	}
	ffi, err := os.Stat(l.Path())
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if ffi.Mode().Perm() != 0o600 {
		t.Errorf("session file perms = %o, want 600", ffi.Mode().Perm())
	}
	body, _ := os.ReadFile(l.Path())
	for _, want := range []string{"# SelfTUI chat session", "# started:", "# host: http://localhost:11434"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("header missing %q:\n%s", want, body)
		}
	}
}

func TestOpenRejectsEmptyDir(t *testing.T) {
	if _, err := Open("", "host"); err == nil {
		t.Fatal("Open with an empty dir should fail")
	}
}

func TestAppendWritesReadableBlocks(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	at := time.Date(2026, 9, 3, 21, 31, 2, 0, time.UTC)
	if err := l.Append("user", "qwen3:8b", "explain this repo", "", at); err != nil {
		t.Fatalf("user append: %v", err)
	}
	if err := l.Append("assistant", "qwen3:8b", "line one\nline two", "0.4s · stop", at.Add(18*time.Second)); err != nil {
		t.Fatalf("assistant append: %v", err)
	}
	if err := l.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	body, _ := os.ReadFile(l.Path())
	text := string(body)
	for _, want := range []string{
		"## user (qwen3:8b) · 21:31:02",
		"explain this repo",
		"## assistant (qwen3:8b) · 21:31:20 · 0.4s · stop",
		"line one\nline two",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("transcript missing %q:\n%s", want, text)
		}
	}
	// turns are separated by a blank line, and content newlines survive
	if !strings.Contains(text, "explain this repo\n\n## assistant") {
		t.Errorf("blocks not separated cleanly:\n%s", text)
	}
	if !strings.Contains(text, "line one\nline two\n\n") {
		t.Errorf("assistant content lost its newlines:\n%s", text)
	}
}

func TestUnknownRoleRejected(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.Append("system", "m", "x", "", time.Now()); err == nil {
		t.Fatal("unknown role should error")
	}
}

func TestAppendModeContinuesAfterReopen(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "h")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append("user", "m", "first", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	path := l.Path()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	// A second writer on the same file (same run semantics) appends, never
	// overwrites the header/first turn.
	l2, err := Open(dir, "h")
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if l2.Path() == path {
		t.Fatal("second Open should create a fresh per-run file")
	}
	if err := l2.Append("user", "m", "second", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "first") {
		t.Errorf("first turn lost after reopen:\n%s", body)
	}
}

func TestNilLogIsNoOp(t *testing.T) {
	var l *Log
	if l.Path() != "" {
		t.Error("nil Path should be empty")
	}
	if err := l.Append("user", "m", "x", "", time.Now()); err != nil {
		t.Errorf("nil append: %v", err)
	}
	if err := l.Flush(); err != nil {
		t.Errorf("nil flush: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("nil close: %v", err)
	}
}
