package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempEntries lists the direct children of dir (for temp-file-leak checks).
func tempEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(es))
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

// assertNoTempFiles fails when any same-directory temp file survived a Save.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".selftui-config-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Errorf("Save left temp files behind: %v", matches)
	}
}

// TestSaveChmodsTo0600 proves the config file is always private: a brand-new
// file and a pre-existing 0644 file both end up 0600 after a save.
func TestSaveChmodsTo0600(t *testing.T) {
	dir := t.TempDir()

	// Fresh file.
	fresh := filepath.Join(dir, "fresh.toml")
	c := Default()
	c.filePath = fresh
	if err := Save(c); err != nil {
		t.Fatalf("Save(fresh) error: %v", err)
	}
	if got := filePerm(t, fresh); got != 0o600 {
		t.Errorf("fresh config mode = %o, want 600", got)
	}

	// A pre-existing 0644 file (created by an older version or the user) must
	// be replaced by a 0600 file.
	loose := filepath.Join(dir, "loose.toml")
	if err := os.WriteFile(loose, []byte("old loose config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c = Default()
	c.filePath = loose
	if err := Save(c); err != nil {
		t.Fatalf("Save(loose) error: %v", err)
	}
	if got := filePerm(t, loose); got != 0o600 {
		t.Errorf("pre-existing 0644 config mode after Save = %o, want 600", got)
	}
	assertNoTempFiles(t, dir)
}

func filePerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

// TestSaveValidationFailureLeavesOldFileUntouched proves a failed validation
// happens before any write: the old file stays byte-identical and no temp file
// is left behind.
func TestSaveValidationFailureLeavesOldFileUntouched(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	old := []byte("host = \"http://localhost:11434\"\ntheme = \"dark\"\n")
	if err := os.WriteFile(p, old, 0o644); err != nil {
		t.Fatal(err)
	}

	c := Default()
	c.filePath = p
	c.Theme = "pink" // invalid: validation must fail before any write
	err := Save(c)
	if err == nil || err.Error() != `config: theme: must be "dark" or "light"` {
		t.Fatalf("Save error = %v, want theme validation error", err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(old) {
		t.Errorf("config file changed after failed validation:\n got %q\nwant %q", got, old)
	}
	if perm := filePerm(t, p); perm != 0o644 {
		t.Errorf("config mode changed after failed validation = %o, want 644 (untouched)", perm)
	}
	if names := tempEntries(t, dir); len(names) != 1 || names[0] != "config.toml" {
		t.Errorf("dir contents after failed save = %v, want only config.toml", names)
	}
}

// TestSaveReloadRoundTrip proves a successful save writes every field and the
// file reloads identically.
func TestSaveReloadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	workspace := t.TempDir()

	c := Default()
	c.filePath = p
	c.Host = "https://ollama.example:11434"
	c.AuthToken = "roundtrip-secret"
	c.DefaultModel = "qwen3:8b"
	c.Theme = "light"
	c.WorkspaceRoot = workspace
	c.Agent.Temperature = 0.33
	c.Agent.TopP = 0.81
	c.Agent.NumCtx = 2048
	c.Agent.MaxToolIterations = 9
	c.Agent.SystemPrompt = "roundtrip agent prompt"

	if err := Save(c); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if perm := filePerm(t, p); perm != 0o600 {
		t.Errorf("saved config mode = %o, want 600", perm)
	}

	loaded, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatalf("Load after Save error: %v", err)
	}
	if loaded.Host != c.Host || loaded.AuthToken != c.AuthToken ||
		loaded.DefaultModel != c.DefaultModel || loaded.Theme != c.Theme ||
		loaded.WorkspaceRoot != c.WorkspaceRoot {
		t.Errorf("top-level fields mismatch:\n got  %+v\n want %+v", loaded, c)
	}
	if loaded.Agent != c.Agent {
		t.Errorf("agent fields mismatch:\n got  %+v\n want %+v", loaded.Agent, c.Agent)
	}
	assertNoTempFiles(t, dir)
}

// TestSaveCreatesParentDir0700 proves Save creates the config directory
// private (0700) — the config can hold an auth token.
func TestSaveCreatesParentDir0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "nested")
	p := filepath.Join(dir, "config.toml")
	c := Default()
	c.filePath = p
	if err := Save(c); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if got := filePerm(t, dir); got != 0o700 {
		t.Errorf("created config dir mode = %o, want 700", got)
	}
}

// TestSaveWriteFailureKeepsOldFile proves a mid-write failure (directory no
// longer writable) surfaces an error, leaves the old file byte-identical, and
// leaves no temp file behind.
func TestSaveWriteFailureKeepsOldFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	old := []byte("original config bytes\n")
	if err := os.WriteFile(p, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	c := Default()
	c.filePath = p
	err := Save(c)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("Save to read-only dir error = %v, want permission denied", err)
	}

	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(old) {
		t.Errorf("config file changed after failed write:\n got  %q\n want %q", got, old)
	}
	assertNoTempFiles(t, dir)
}

// TestSaveRenameFailureCleansTempFile proves the temp file is removed when the
// final rename fails (here: the target path is an existing directory), so a
// failed save never litters the config directory.
func TestSaveRenameFailureCleansTempFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(p, 0o700); err != nil { // a directory where the file goes
		t.Fatal(err)
	}
	occupant := filepath.Join(p, "keep")
	if err := os.WriteFile(occupant, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := Default()
	c.filePath = p
	if err := Save(c); err == nil {
		t.Fatal("Save over an existing directory should fail")
	}

	if names := tempEntries(t, dir); len(names) != 1 || names[0] != "config.toml" {
		t.Errorf("dir contents after failed rename = %v, want only the config.toml dir (temp cleaned)", names)
	}
	assertNoTempFiles(t, dir)
}
