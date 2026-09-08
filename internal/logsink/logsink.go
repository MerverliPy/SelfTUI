// Package logsink holds the shared debug-log plumbing (PLAN.md §12 N5):
// one redacting writer fans every charmbracelet/log entry out to the
// durable file sink and an in-memory ring buffer that backs the TUI logs
// drawer, so the file and the drawer can never disagree and a secret that
// reaches the logger reaches neither.
package logsink

import (
	"io"
	"regexp"
	"strings"
	"sync"
)

// Redacted is the replacement written over every scrubbed credential.
const Redacted = "[redacted]"

// bearerRe matches "Authorization: Bearer <token>"-style credentials so a
// token that was never explicitly registered — echoed inside an error
// string, a future header trace, a test URL — is still scrubbed before it
// reaches any sink. The 4-character floor keeps prose like "Bearer of good
// news" intact; real tokens are longer.
var bearerRe = regexp.MustCompile(`(?i)\bbearer[ \t]+[A-Za-z0-9._~+/=-]{4,}`)

// Redact replaces every registered secret with [redacted] and every
// Bearer-style credential with "Bearer [redacted]". Pure over its inputs so
// it is table-testable; the registered secrets are checked first so the
// configured token is gone before the generic pattern runs. Empty secrets
// are skipped (they would replace everything).
func Redact(s string, secrets []string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, Redacted)
		}
	}
	return bearerRe.ReplaceAllString(s, "Bearer "+Redacted)
}

// Ring is a bounded, thread-safe in-memory buffer of the most recent log
// lines. It implements io.Writer so the shared sink can feed it directly;
// the TUI drawer reads it through Lines. Entries are whole lines: a write
// carrying embedded newlines (a multi-line error value) is split so the
// drawer can render one entry per row.
type Ring struct {
	mu    sync.Mutex
	max   int
	lines []string
}

// NewRing builds a ring holding the most recent max lines (max < 1 is
// clamped to 1, so the drawer always has somewhere to read from).
func NewRing(max int) *Ring {
	if max < 1 {
		max = 1
	}
	return &Ring{max: max}
}

// Write appends p to the ring after trimming the trailing newline, dropping
// the oldest lines once the cap is reached. The compacting drop keeps the
// backing array's capacity at the cap instead of growing it forever.
func (r *Ring) Write(p []byte) (int, error) {
	s := strings.TrimRight(string(p), "\n")
	if s == "" {
		return len(p), nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, strings.Split(s, "\n")...)
	if over := len(r.lines) - r.max; over > 0 {
		copy(r.lines, r.lines[over:])
		r.lines = r.lines[:len(r.lines)-over]
	}
	return len(p), nil
}

// Lines returns a copy of the buffered lines, oldest first. Callers may
// mutate the result without touching the ring.
func (r *Ring) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

// Sink is the shared debug-log sink: it redacts every entry, appends the
// result to the drawer's ring, and forwards it to the durable file writer.
// One charmbracelet/log logger writes through it (cmd/self-tui), so the
// --log-file output and the TUI drawer are the same stream (PLAN.md §12 N5).
//
// Redaction happens per Write. charmbracelet/log emits one formatted entry
// per Write call, so a token is never split across redaction boundaries;
// that invariant is why the redactor is not buffered/streaming.
type Sink struct {
	mu      sync.Mutex
	secrets []string
	file    io.Writer
	ring    *Ring
}

// New builds a sink that redacts secret (the configured bearer token, when
// set) before anything reaches ring or file. A nil file is allowed (tests:
// ring-only).
func New(file io.Writer, ring *Ring, secret string) *Sink {
	s := &Sink{file: file, ring: ring}
	if secret != "" {
		s.secrets = append(s.secrets, secret)
	}
	return s
}

// SetSecret registers a replacement bearer token (a Settings save changed
// cfg.AuthToken): later entries redact it too. Historical file content is
// deliberately untouched — rewriting the past is not redaction. Registering
// an empty or already-known secret is a no-op.
func (s *Sink) SetSecret(secret string) {
	if secret == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sec := range s.secrets {
		if sec == secret {
			return
		}
	}
	s.secrets = append(s.secrets, secret)
}

// Lines returns the ring's current contents (a copy): the drawer reads the
// same redacted stream the file receives.
func (s *Sink) Lines() []string {
	return s.ring.Lines()
}

// Write redacts p, appends the clean line to the ring, and forwards it to
// the file writer. The ring always gets the entry, so a wedged file does
// not starve the drawer.
func (s *Sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	clean := Redact(string(p), s.secrets)
	s.mu.Unlock()

	s.ring.Write([]byte(clean))
	if s.file == nil {
		return len(p), nil
	}
	return s.file.Write([]byte(clean))
}
