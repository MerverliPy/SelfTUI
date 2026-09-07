package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	uAt := time.Date(2026, 9, 7, 21, 31, 2, 0, time.UTC)
	aAt := uAt.Add(18 * time.Second)
	turnsIn := []struct {
		role, model, content, meta string
		at                         time.Time
	}{
		{"user", "qwen3:8b", "explain this repo", "", uAt},
		{"assistant", "qwen3:8b", "line one\nline two", "0.4s · stop", aAt},
		{"user", "qwen3:8b", "second question with\n\nblank lines", "", aAt.Add(time.Minute)},
		{"assistant", "qwen3:1.7b", "different model answer", "3.1s · length", aAt.Add(2 * time.Minute)},
		{"user", "qwen3:8b", "\n\nleading newlines are content", "", aAt.Add(3 * time.Minute)},
	}
	for _, in := range turnsIn {
		if err := l.Append(in.role, in.model, in.content, in.meta, in.at); err != nil {
			t.Fatalf("%s append: %v", in.role, err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := Load(l.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(turnsIn) {
		t.Fatalf("parsed %d turns, want %d:\n%+v", len(got), len(turnsIn), got)
	}
	for i, in := range turnsIn {
		g := got[i]
		if g.Role != in.role || g.Model != in.model || g.Meta != in.meta || g.Content != in.content {
			t.Errorf("turn %d = %+v, want role=%q model=%q meta=%q content=%q",
				i, g, in.role, in.model, in.meta, in.content)
		}
		// The header carries only the clock time; the parsed instant must
		// reproduce it (hour/min/sec) but is not a dated timestamp.
		if g.At.Hour() != in.at.Hour() || g.At.Minute() != in.at.Minute() || g.At.Second() != in.at.Second() {
			t.Errorf("turn %d time = %v, want clock %v", i, g.At.Format("15:04:05"), in.at.Format("15:04:05"))
		}
	}
}

func TestParseSkipsHeaderComments(t *testing.T) {
	data := "# SelfTUI chat session\n# started: 2026-09-07 21:31:02\n# host: http://localhost:11434\n\n## user (m) · 21:31:02\n\nhello\n\n"
	turns := Parse([]byte(data))
	if len(turns) != 1 {
		t.Fatalf("parsed %d turns, want 1: %+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Content != "hello" {
		t.Errorf("turn = %+v", turns[0])
	}
}

func TestParseUnknownRoleDroppedAndUnknownHeaderIsContent(t *testing.T) {
	// Recorded roles are the only block boundaries, and only under the
	// exact writer grammar. An unknown-role block with no preceding turn is
	// dropped; header-like lines the writer cannot produce (a non-role
	// heading, a first segment that is not a clock time) are content —
	// content integrity wins over skipping.
	data := strings.Join([]string{
		"# SelfTUI chat session",
		"",
		"## system (m)", // unknown role before any block: dropped
		"",
		"system content",
		"",
		"## user (m) · 21:31:02",
		"",
		"kept one",
		"",
		"## garbage header with no role structure", // not a recorded role: content
		"",
		"## assistant (m) · not-a-time · 1.2s · stop", // first segment is not a time: content
		"",
		"kept two",
		"",
	}, "\n")
	turns := Parse([]byte(data))
	if len(turns) != 1 {
		t.Fatalf("parsed %d turns, want 1: %+v", len(turns), turns)
	}
	if turns[0].Role != "user" || turns[0].Model != "m" {
		t.Errorf("turn 0 = %+v", turns[0])
	}
	want := "kept one\n\n## garbage header with no role structure\n\n## assistant (m) · not-a-time · 1.2s · stop\n\nkept two"
	if turns[0].Content != want {
		t.Errorf("content = %q, want %q", turns[0].Content, want)
	}
}

func TestParseBodyHeadingsAreContent(t *testing.T) {
	// Blocker regression: any line beginning "## user"/"## assistant" used
	// to split the block. Under the exact writer grammar only well-formed
	// headers do; body headings like "## user story" are content. An exact
	// form ("## assistant") is inherently ambiguous with the writer's own
	// output and stays a header.
	data := strings.Join([]string{
		"## user (m) · 21:31:02",
		"",
		"before",
		"",
		"## user story",
		"mid",
		"",
		"## user (cont",
		"after",
		"",
		"## assistant (m) · 21:31:20",
		"",
		"after too",
		"",
	}, "\n")
	turns := Parse([]byte(data))
	if len(turns) != 2 {
		t.Fatalf("parsed %d turns, want 2: %+v", len(turns), turns)
	}
	want0 := "before\n\n## user story\nmid\n\n## user (cont\nafter"
	if turns[0].Role != "user" || turns[0].Content != want0 {
		t.Errorf("turn 0 = %+v, want content %q", turns[0], want0)
	}
	if turns[1].Role != "assistant" || turns[1].Content != "after too" {
		t.Errorf("turn 1 = %+v", turns[1])
	}
}

func TestParseEmptyAndHeaderOnly(t *testing.T) {
	if got := Parse(nil); len(got) != 0 {
		t.Errorf("nil parse = %+v", got)
	}
	// A transcript whose writer crashed after the header parses to nothing.
	turns := Parse([]byte("# SelfTUI chat session\n# started: 2026-09-07 21:31:02\n\n"))
	if len(turns) != 0 {
		t.Errorf("header-only parse = %+v", turns)
	}
}

func TestParseContentStartingWithHashesSurvives(t *testing.T) {
	// Markdown headings inside content (## not followed by a recorded role)
	// are content, not block boundaries.
	data := "## user (m)\n\n## context\nsee notes\n\n## assistant (m)\n\n## reply\nok\n"
	turns := Parse([]byte(data))
	if len(turns) != 2 {
		t.Fatalf("parsed %d turns, want 2: %+v", len(turns), turns)
	}
	if turns[0].Content != "## context\nsee notes" {
		t.Errorf("turn 0 content = %q", turns[0].Content)
	}
	if turns[1].Content != "## reply\nok" {
		t.Errorf("turn 1 content = %q", turns[1].Content)
	}
}

func TestListSessionsNewestFirstAndEmptyDir(t *testing.T) {
	// A missing directory is an empty listing, not an error.
	got, err := ListSessions(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(got) != 0 {
		t.Fatalf("missing dir: (%v, %v)", got, err)
	}
	got, err = ListSessions("")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty dir arg: (%v, %v)", got, err)
	}

	dir := t.TempDir()
	old := filepath.Join(dir, "chat-20260906-100000.000-111.md")
	newer := filepath.Join(dir, "chat-20260907-214503.123-222.md")
	for path, body := range map[string]string{old: "old", newer: "newer"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A stale mtime on the "old" file so the ordering is data, not creation order.
	stale := time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)
	if err := os.Chtimes(old, stale, stale); err != nil {
		t.Fatal(err)
	}
	// Non-transcript files are not listed.
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "chat-20260905-000000.000-333.md"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err = ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("listed %d entries, want 2: %+v", len(got), got)
	}
	if got[0].Name != "chat-20260907-214503.123-222.md" || got[1].Name != "chat-20260906-100000.000-111.md" {
		t.Errorf("order = [%s, %s], want newest first", got[0].Name, got[1].Name)
	}
	if got[0].Size != int64(len("newer")) {
		t.Errorf("size = %d, want %d", got[0].Size, len("newer"))
	}
	if !got[1].ModTime.Equal(stale) {
		t.Errorf("mtime = %v, want %v", got[1].ModTime, stale)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "chat-none.md")); err == nil {
		t.Fatal("missing file should error")
	}
}

func TestListSessionsModTimeTieBreakByName(t *testing.T) {
	// Equal mtimes (a fast restart can land two files inside one mtime
	// tick): the file name breaks the tie so the listing is deterministic
	// and stays newest-first (names embed a millisecond timestamp).
	dir := t.TempDir()
	for _, name := range []string{"chat-20260907-214503.123-222.md", "chat-20260907-214503.123-111.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stamp := time.Date(2026, 9, 7, 21, 45, 3, 0, time.UTC)
	for _, name := range []string{"chat-20260907-214503.123-222.md", "chat-20260907-214503.123-111.md"} {
		if err := os.Chtimes(filepath.Join(dir, name), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("listed %d entries, want 2: %+v", len(got), got)
	}
	if got[0].Name != "chat-20260907-214503.123-222.md" || got[1].Name != "chat-20260907-214503.123-111.md" {
		t.Errorf("tie order = [%s, %s], want name-descending (newest first)", got[0].Name, got[1].Name)
	}
}
