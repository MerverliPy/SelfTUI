package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 4 (workspace tool trust): workspace tools are OFF by default and can
// only be enabled together with a workspace root that is not the whole
// filesystem or the user's home directory. These tests pin the tools_enabled
// surface: TOML key, SELFTUI_TOOLS_ENABLED env parsing, the default, and the
// safety validation on the pairing of tools_enabled with workspace_root.

func TestToolsEnabledDefaultsFalse(t *testing.T) {
	blankEnv(t)
	t.Setenv(envPrefix+"TOOLS_ENABLED", "")
	if c := Default(); c.ToolsEnabled {
		t.Error("Default().ToolsEnabled = true, want false (tools are opt-in)")
	}
	// A file that never mentions the key must also leave tools disabled.
	p := writeFile(t, "host = \"http://localhost:11434\"\n")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.ToolsEnabled {
		t.Error("Load with no tools_enabled key enabled tools")
	}
}

func TestToolsEnabledParsedFromFile(t *testing.T) {
	blankEnv(t)
	t.Setenv(envPrefix+"TOOLS_ENABLED", "")
	ws := t.TempDir()
	p := writeFile(t, "tools_enabled = true\nworkspace_root = "+quote(ws)+"\n")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if !c.ToolsEnabled || c.WorkspaceRoot != ws {
		t.Errorf("file source: ToolsEnabled=%v WorkspaceRoot=%q, want true + %q", c.ToolsEnabled, c.WorkspaceRoot, ws)
	}

	p = writeFile(t, "tools_enabled = false\n")
	c, err = Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.ToolsEnabled {
		t.Error("tools_enabled = false must load as disabled")
	}
}

func TestToolsEnabledEnvParsesBool(t *testing.T) {
	blankEnv(t)
	ws := t.TempDir()
	for _, v := range []string{"true", "1", "TRUE", "True", "t", "T"} {
		dir := t.TempDir()
		p := writeFile(t, "workspace_root = "+quote(ws)+"\n")
		t.Setenv(envPrefix+"TOOLS_ENABLED", v)
		c, err := Load(Overrides{ConfigPath: &p, WorkspaceRoot: &dir})
		if err != nil {
			t.Fatalf("env %q: %v", v, err)
		}
		if !c.ToolsEnabled {
			t.Errorf("SELFTUI_TOOLS_ENABLED=%q should enable tools", v)
		}
	}
	for _, v := range []string{"false", "0", "FALSE", "f", "F"} {
		dir := t.TempDir()
		p := writeFile(t, "workspace_root = "+quote(dir)+"\n")
		t.Setenv(envPrefix+"TOOLS_ENABLED", v)
		c, err := Load(Overrides{ConfigPath: &p})
		if err != nil {
			t.Fatalf("env %q: %v", v, err)
		}
		if c.ToolsEnabled {
			t.Errorf("SELFTUI_TOOLS_ENABLED=%q should leave tools disabled", v)
		}
	}
}

func TestToolsEnabledEnvGarbageIsRejected(t *testing.T) {
	blankEnv(t)
	ws := t.TempDir()
	p := writeFile(t, "workspace_root = "+quote(ws)+"\n")
	t.Setenv(envPrefix+"TOOLS_ENABLED", "yes")
	_, err := Load(Overrides{ConfigPath: &p})
	if err == nil || !strings.Contains(err.Error(), `parse SELFTUI_TOOLS_ENABLED="yes"`) {
		t.Fatalf("Load error = %v, want a strconv.ParseBool wrap naming the variable", err)
	}
}

func TestToolsEnabledEnvOverridesFile(t *testing.T) {
	blankEnv(t)
	ws := t.TempDir()
	// File enables tools; env disables them. Env beats file, so the result
	// must be disabled — and the file's workspace must not be validated as a
	// tools pairing (tools are off).
	p := writeFile(t, "tools_enabled = true\nworkspace_root = "+quote(ws)+"\n")
	t.Setenv(envPrefix+"TOOLS_ENABLED", "false")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.ToolsEnabled {
		t.Error("env false must override file true")
	}
}

// TestValidateToolsEnabledWorkspaceRoot pins the exact pairing rule: enabling
// tools with a workspace_root that is empty (→ cwd, unpredictable), "/" (the
// whole filesystem) or the current user's home directory (adjacent to
// .ssh/.gnupg/...) is rejected with a stable message. Disabled tools keep
// those roots legal (they are only unsafe when the agent gets file tools).
func TestValidateToolsEnabledWorkspaceRoot(t *testing.T) {
	blankEnv(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	good := t.TempDir()

	rejected := []struct{ name, ws, wantErr string }{
		{
			"empty", "",
			"config: tools_enabled: workspace_root is required when tools are enabled",
		},
		{
			"root", "/",
			"config: tools_enabled: workspace_root must not be / when tools are enabled",
		},
		{
			"home", home,
			"config: tools_enabled: workspace_root must not be your home directory when tools are enabled",
		},
		{
			"home trailing slash", home + string(filepath.Separator),
			"config: tools_enabled: workspace_root must not be your home directory when tools are enabled",
		},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			c.ToolsEnabled = true
			c.WorkspaceRoot = tc.ws
			if err := Validate(c); err == nil {
				t.Fatalf("Validate(tools on, workspace_root %q) = nil, want rejection", tc.ws)
			} else if err.Error() != tc.wantErr {
				t.Fatalf("error = %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}

	// A real project directory is the one safe pairing.
	c := Default()
	c.ToolsEnabled = true
	c.WorkspaceRoot = good
	if err := Validate(c); err != nil {
		t.Errorf("Validate(tools on, temp workspace) = %v, want nil", err)
	}

	// Tools off: the broad roots stay legal (no tools → no exposure).
	for _, ws := range []string{"", "/", home} {
		c := Default()
		c.ToolsEnabled = false
		c.WorkspaceRoot = ws
		if err := Validate(c); err != nil {
			t.Errorf("Validate(tools off, workspace_root %q) = %v, want nil", ws, err)
		}
	}
}

// TestValidateToolsWorkspaceAliasesRejected (H-02 regression): workspace-root
// spellings that resolve to "/" or the home directory are rejected by their
// canonical directory identity, not their spelling. Lexical aliases — /tmp/..,
// a symlink to "/", a symlink to home, or a relative path that resolves to
// home from the controlling cwd — used to pass validation and would have
// handed the enabled tool jail the whole filesystem or the home directory.
func TestValidateToolsWorkspaceAliasesRejected(t *testing.T) {
	blankEnv(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	rootMsg := "config: tools_enabled: workspace_root must not be / when tools are enabled"
	homeMsg := "config: tools_enabled: workspace_root must not be your home directory when tools are enabled"

	symlink := func(t *testing.T, target string) (string, error) {
		link := filepath.Join(t.TempDir(), "ws")
		if err := os.Symlink(target, link); err != nil {
			return "", err
		}
		return link, nil
	}

	cases := []struct {
		name string
		// ws returns the workspace_root spelling to validate; an error skips
		// the case (filesystem without symlinks, unmountable cwd, ...).
		ws   func(t *testing.T) (string, error)
		want string
	}{
		{"direct root", func(t *testing.T) (string, error) { return "/", nil }, rootMsg},
		{"direct home", func(t *testing.T) (string, error) { return home, nil }, homeMsg},
		// The literal spelling "/tmp/.." — a user-typed string, not a
		// filepath.Join result (Join would clean it to "/" already) — must
		// be rejected once resolved: filepath.Abs/Clean collapse it to "/".
		{"dotdot above tmp", func(t *testing.T) (string, error) {
			if st, err := os.Stat("/tmp"); err != nil || !st.IsDir() {
				t.Skipf("/tmp not available: %v", err)
			}
			return "/tmp/..", nil
		}, rootMsg},
		{"symlink to root", func(t *testing.T) (string, error) { return symlink(t, "/") }, rootMsg},
		{"symlink to home", func(t *testing.T) (string, error) { return symlink(t, home) }, homeMsg},
		// A relative spelling that resolves to home when the controlling cwd
		// is the parent of home must be rejected the same way.
		{"relative path resolving to home", func(t *testing.T) (string, error) {
			t.Chdir(filepath.Dir(home)) // restores when the subtest ends
			return filepath.Base(home), nil
		}, homeMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws, err := tc.ws(t)
			if err != nil {
				t.Skipf("case unavailable: %v", err)
			}
			c := Default()
			c.ToolsEnabled = true
			c.WorkspaceRoot = ws
			if err := Validate(c); err == nil {
				t.Fatalf("Validate(tools on, workspace_root %q) = nil, want rejection %q", ws, tc.want)
			} else if got := err.Error(); got != tc.want {
				t.Fatalf("error = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestValidateToolsWorkspaceSymlinkToRealProjectAccepted guards against
// over-rejection: a symlink to a real project directory stays legal. The
// boundary is canonical directory identity ("/" or home), not symlink use
// itself.
func TestValidateToolsWorkspaceSymlinkToRealProjectAccepted(t *testing.T) {
	blankEnv(t)
	project := t.TempDir()
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(project, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	c := Default()
	c.ToolsEnabled = true
	c.WorkspaceRoot = link
	if err := Validate(c); err != nil {
		t.Errorf("Validate(tools on, symlink to real project %q) = %v, want nil", link, err)
	}
}

// TestLoadToolsWorkspaceStoresCanonicalRoot (H-02 step 5): with tools armed,
// Load replaces the accepted spelling with its canonical absolute root so the
// runtime receives the same directory the tool jail canonicalizes against.
// The spelling alone (symlink, "..", relative) never reaches the agent layer;
// validation and execution cannot diverge on the boot path.
func TestLoadToolsWorkspaceStoresCanonicalRoot(t *testing.T) {
	blankEnv(t)
	project := t.TempDir()
	link := filepath.Join(t.TempDir(), "ws-link")
	if err := os.Symlink(project, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	sources := map[string]func() (Config, error){
		"toml": func() (Config, error) {
			p := writeFile(t, "tools_enabled = true\nworkspace_root = "+quote(link)+"\n")
			return Load(Overrides{ConfigPath: &p})
		},
		"env": func() (Config, error) {
			c := Default()
			c.ToolsEnabled = true
			c.WorkspaceRoot = link
			envFrom(t, c)
			missing := filepath.Join(t.TempDir(), "absent.toml")
			return Load(Overrides{ConfigPath: &missing})
		},
	}
	for name, load := range sources {
		t.Run(name, func(t *testing.T) {
			blankEnv(t)
			c, err := load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.WorkspaceRoot != project {
				t.Errorf("Load stored workspace_root %q, want canonical %q", c.WorkspaceRoot, project)
			}
		})
	}
}

// TestLoadToolsOffKeepsBroadWorkspaceSpelling: with tools disabled the broad
// roots stay legal and Load stores the spelling verbatim — canonicalization is
// a tools-armed property, never a config-wide rewrite.
func TestLoadToolsOffKeepsBroadWorkspaceSpelling(t *testing.T) {
	blankEnv(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	for _, ws := range []string{"/", home} {
		p := writeFile(t, "workspace_root = "+quote(ws)+"\n")
		c, err := Load(Overrides{ConfigPath: &p})
		if err != nil {
			t.Fatalf("Load(tools off, workspace_root %q): %v", ws, err)
		}
		if c.WorkspaceRoot != ws {
			t.Errorf("tools off: Load stored workspace_root %q, want verbatim %q", c.WorkspaceRoot, ws)
		}
	}
}

func TestSavePersistsToolsEnabled(t *testing.T) {
	blankEnv(t)
	ws := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := Load(Overrides{ConfigPath: &path, WorkspaceRoot: &ws})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ToolsEnabled = true
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, err := Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.ToolsEnabled || reloaded.WorkspaceRoot != ws {
		t.Errorf("reloaded = %+v, want tools enabled + workspace %q", reloaded, ws)
	}
	// An explicit false round-trips too.
	cfg.ToolsEnabled = false
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err = Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ToolsEnabled {
		t.Error("saved false reloaded as true")
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}
