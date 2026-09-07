package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Saved is one recorded transcript file as the resume picker shows it: the
// file name (newest-first ordering key), its path (the load key), and the
// mtime/size summary rendered in the picker row.
type Saved struct {
	Name    string
	Path    string
	ModTime time.Time
	Size    int64
}

// Turn is one parsed transcript block: a committed user or assistant message
// with the header metadata the recorder wrote. Meta is the assistant's
// elapsed·reason suffix verbatim ("" for user turns and plain commits).
type Turn struct {
	Role    string // "user" or "assistant"
	Model   string
	Meta    string
	At      time.Time // wall-clock time from the header (zero when absent)
	Content string    // verbatim markdown body of the block
}

// ListSessions returns the recorded transcripts under dir, newest first.
// A missing or empty directory is not an error (nothing has ever been
// recorded) — it yields an empty list. Only files the writer produces are
// listed: the "chat-*.md" naming Open creates.
func ListSessions(dir string) ([]Saved, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("session: list %s: %w", dir, err)
	}
	var out []Saved
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "chat-") || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // raced deletion; skip rather than fail the listing
		}
		out = append(out, Saved{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		// Equal mtimes (a fast restart can land two files in one mtime
		// tick): the millisecond timestamp in the file name breaks the tie
		// so the order stays deterministic and newest-first.
		return out[i].Name > out[j].Name
	})
	return out, nil
}

// Load reads and parses one transcript file. The path comes from
// ListSessions, so a missing file (deleted between listing and picking) is
// the caller's expected failure; per-block damage never fails the load —
// only bad blocks are skipped (see Parse).
func Load(path string) ([]Turn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session: read %s: %w", path, err)
	}
	return Parse(data), nil
}

// Parse turns recorded transcript markdown back into ordered turns. Lines
// starting with '#' before the first block are the file header and are
// skipped. A block starts at a "## user" / "## assistant" header line and
// runs to the next such header; its content is kept verbatim. Only these
// two recorded roles are block boundaries — a markdown heading inside
// committed content ("## context") is ordinary content, never a split. A
// damaged block (a truncated tail from a crash) parses to whatever was
// written; nothing here errors the whole load.
func Parse(data []byte) []Turn {
	lines := strings.Split(string(data), "\n")
	var turns []Turn
	for i := 0; i < len(lines); i++ {
		role, model, meta, at, ok := parseBlockHeader(lines[i])
		if !ok {
			continue // file-header comment, blank line, or non-block text
		}
		i++
		var body []string
		for i < len(lines) {
			if _, _, _, _, isBlock := parseBlockHeader(lines[i]); isBlock {
				i--
				break
			}
			body = append(body, lines[i])
			i++
		}
		// The writer puts exactly one blank line between the title and the
		// content, so exactly that one separator blank comes off here — and
		// only that one: blank lines at the start of the recorded content
		// are real content and must round-trip. Trailing blanks are the
		// writer's closing separator, never content (the writer already
		// trimmed the content's tail when it appended).
		if len(body) > 0 && body[0] == "" {
			body = body[1:]
		}
		content := strings.TrimRight(strings.Join(body, "\n"), "\n")
		turns = append(turns, Turn{Role: role, Model: model, Meta: meta, At: at, Content: content})
	}
	return turns
}

// headerSep is the " · " segment separator the writer puts between the
// header fields (four bytes in UTF-8: space + two-byte middle dot + space).
const headerSep = " · "

// parseBlockHeader recognizes one "## role (model) · time · meta" title and
// reports ok=false for every line that is not a block header. Matching is
// exact per the writer's grammar — "## role", optionally " (model)", then
// optionally " · HH:MM:SS" followed by the meta segments (which themselves
// contain " · ", e.g. "0.4s · stop") — so a body heading like "## user story"
// or a segment line the writer cannot produce is content, never a
// fabricated turn. The inherent ambiguity of a body line that spells an
// exact header form ("## user") remains: those stay block boundaries.
func parseBlockHeader(line string) (role, model, meta string, at time.Time, ok bool) {
	if !strings.HasPrefix(line, "## ") {
		return "", "", "", time.Time{}, false
	}
	rest := strings.TrimPrefix(line, "## ")
	role = rest
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		role, rest = rest[:i], rest[i+1:]
	} else {
		rest = ""
	}
	if role != "user" && role != "assistant" {
		return "", "", "", time.Time{}, false
	}
	// Continuations after the role are exact. The role split consumed the
	// one space the writer puts between role and the rest, so the shapes
	// here are: nothing (plain commit), "(model)", or "· "+segments —
	// anything else is content, never a fabricated turn.
	switch {
	case rest == "":
		return role, "", "", time.Time{}, true
	case strings.HasPrefix(rest, "("):
		i := strings.IndexByte(rest, ')')
		if i < 0 {
			return "", "", "", time.Time{}, false // unterminated model: content
		}
		model = rest[1:i]
		rest = rest[i+1:]
		if rest == "" {
			return role, model, "", time.Time{}, true
		}
		if !strings.HasPrefix(rest, headerSep) {
			return "", "", "", time.Time{}, false
		}
		rest = rest[len(headerSep):]
	case strings.HasPrefix(rest, "· "):
		rest = rest[len("· "):]
	default:
		return "", "", "", time.Time{}, false
	}
	// With segments present, the first must be the clock time; the rest are
	// the meta suffix verbatim. A first segment that is not a time is a
	// shape the writer never emits — treat the line as content.
	first := rest
	if i := strings.Index(rest, headerSep); i >= 0 {
		first, rest = rest[:i], rest[i+len(headerSep):]
	} else {
		rest = ""
	}
	t, err := time.Parse("15:04:05", first)
	if err != nil {
		return "", "", "", time.Time{}, false
	}
	at = t
	meta = rest
	if !at.IsZero() {
		// The header records only the clock time; the date is unknowable
		// from a per-process file, so the parsed timestamp is dateless and
		// display code must not treat it as a full instant.
		at = time.Date(0, time.January, 1, at.Hour(), at.Minute(), at.Second(), 0, time.UTC)
	}
	return role, model, meta, at, true
}
