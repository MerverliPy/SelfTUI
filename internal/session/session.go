// Package session appends a SelfTUI chat transcript to a per-process
// markdown file so a conversation stays recoverable after the process exits.
// Chat itself remains in-memory (PLAN §11 / M6: "chat is per-process"); this
// log is the durable mirror the owner asked for: it records committed user
// and assistant turns as they land, survives crashes and exits, and lives in
// the XDG state dir where log.txt already does (0600 perms).
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Log is an append-only transcript writer. A process writes one file for its
// whole run (conversations are per-process); every committed message is one
// block, so a crashed or quit app leaves a complete recoverable record.
type Log struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

// Open creates the session directory (0700) and a fresh per-run transcript
// file (0600) under dir, writing a small header. A nil-but-non-nil writer is
// never returned: any failure is returned as an error so the UI can surface
// it once and keep chatting.
func Open(dir, host string) (*Log, error) {
	if dir == "" {
		return nil, fmt.Errorf("session: empty directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("session: mkdir %s: %w", dir, err)
	}
	// The name is a millisecond timestamp plus the pid, so two Opens in the
	// same process can collide when they land within one clock tick (the
	// reopen test, a fast restart). Never reuse an existing file: O_EXCL
	// turns the collision into a retry on the next tick instead of silently
	// appending into the previous run's transcript.
	var f *os.File
	var name string
	for i := 0; i < 20; i++ {
		candidate := sessionFileCandidate(dir)
		fh, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f, name = fh, candidate
			break
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("session: open %s: %w", candidate, err)
		}
		time.Sleep(time.Millisecond) // the next tick yields a fresh name
	}
	if f == nil {
		return nil, fmt.Errorf("session: no fresh transcript name in %s after repeated same-tick collisions", dir)
	}
	l := &Log{f: f, path: name}
	header := "# SelfTUI chat session\n"
	header += "# started: " + time.Now().Format("2006-01-02 15:04:05") + "\n"
	if host != "" {
		header += "# host: " + host + "\n"
	}
	header += "\n"
	if _, err := f.WriteString(header); err != nil {
		f.Close()
		return nil, fmt.Errorf("session: header: %w", err)
	}
	return l, nil
}

// sessionFileCandidate returns the transcript name for the current instant
// (millisecond timestamp + pid).
func sessionFileCandidate(dir string) string {
	return filepath.Join(dir, fmt.Sprintf("chat-%s-%d.md", time.Now().Format("20060102-150405.000"), os.Getpid()))
}

// Path returns the transcript file path (for the /export hint and error text).
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Append writes one committed turn. role is "user" or "assistant"; model is
// the turn's model; meta is the assistant's elapsed·reason suffix ("" for
// user turns and for plain commits without a footer). Content is preserved
// verbatim (it is markdown already).
func (l *Log) Append(role, model, content, meta string, at time.Time) error {
	if l == nil {
		return nil
	}
	if role != "user" && role != "assistant" {
		return fmt.Errorf("session: unknown role %q", role)
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	title := "## " + role
	if model != "" {
		title += " (" + model + ")"
	}
	if !at.IsZero() {
		title += " · " + at.Format("15:04:05")
	}
	if meta != "" {
		title += " · " + meta
	}
	block := title + "\n\n" + strings.TrimRight(content, "\n") + "\n\n"
	if _, err := l.f.WriteString(block); err != nil {
		return fmt.Errorf("session: append: %w", err)
	}
	return nil
}

// Flush makes every appended turn durable (used by /export and before reads).
func (l *Log) Flush() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.f.Sync(); err != nil {
		return fmt.Errorf("session: flush: %w", err)
	}
	return nil
}

// Close flushes and closes the writer. Idempotent.
func (l *Log) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Sync()
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	l.f = nil
	return err
}
