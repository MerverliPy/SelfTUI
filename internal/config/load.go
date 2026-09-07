package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/adrg/xdg"
	"github.com/pelletier/go-toml/v2"
)

// fileAgentConfig mirrors the `agent` table in config.toml and uses pointers so
// we can tell when a key is intentionally present (including empty strings).
type fileAgentConfig struct {
	Temperature       *float64 `toml:"temperature"`
	TopP              *float64 `toml:"top_p"`
	NumCtx            *int     `toml:"num_ctx"`
	SystemPrompt      *string  `toml:"system_prompt"`
	MaxToolIterations *int     `toml:"max_tool_iterations"`
}

// fileConfig is the on-disk TOML shape, including nested `agent` keys.
type fileConfig struct {
	Host          *string          `toml:"host"`
	AuthToken     *string          `toml:"auth_token"`
	DefaultModel  *string          `toml:"default_model"`
	Theme         *string          `toml:"theme"`
	WorkspaceRoot *string          `toml:"workspace_root"`
	ToolsEnabled  *bool            `toml:"tools_enabled"`
	Agent         *fileAgentConfig `toml:"agent"`
}

// envPrefix is the shared prefix for SelfTUI environment variables.
const envPrefix = "SELFTUI_"

// Overrides carries flag (or other highest-priority) values. Nil fields do not
// override env/file/defaults. ConfigPath overrides the config file location
// (default: $XDG_CONFIG_HOME/selftui/config.toml via xdg).
type Overrides struct {
	ConfigPath        *string
	Host              *string
	AuthToken         *string
	Theme             *string
	DefaultModel      *string
	WorkspaceRoot     *string
	Temperature       *float64
	TopP              *float64
	NumCtx            *int
	SystemPrompt      *string
	MaxToolIterations *int
}

// Load resolves the configuration: defaults -> config file -> env -> overrides.
// The resolved value is validated with Validate (the single configuration
// policy), so an invalid value is rejected with the same stable error no
// matter which source supplied it.
func Load(ov Overrides) (Config, error) {
	cfg := Default()

	// 1. config file (if it exists).
	path := ov.ConfigPath
	if path == nil {
		p, err := xdg.ConfigFile(filepath.Join("selftui", "config.toml"))
		if err != nil {
			return cfg, fmt.Errorf("resolve config path: %w", err)
		}
		path = &p
	}
	cfg.filePath = *path

	if b, err := os.ReadFile(*path); err == nil {
		var file fileConfig
		if err := toml.Unmarshal(b, &file); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", *path, err)
		}
		applyFile(&cfg, file)
	} else if os.IsNotExist(err) {
		// A missing *explicit* -config path is a user-intent statement (a typo'd
		// path must not silently run with defaults); only the auto-resolved
		// default XDG file may be absent on first boot (H-01).
		if ov.ConfigPath != nil {
			return cfg, fmt.Errorf("config file not found at %s", *path)
		}
	} else {
		return cfg, fmt.Errorf("read config %s: %w", *path, err)
	}

	// 2. environment.
	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}

	// 3. flags/overrides.
	applyOverrides(&cfg, ov)

	// 4. policy check on the fully resolved value.
	if err := Validate(cfg); err != nil {
		return cfg, err
	}

	// 5. canonical persistence: with workspace tools armed, Load replaces the
	// accepted spelling with its canonical absolute root, so every consumer of
	// this Config — the agent runner, the status bar, a later Settings save —
	// receives the same directory the tool jail canonicalizes against (H-02).
	// Validate just resolved the same spelling successfully, so this cannot
	// fail unless the filesystem changes in between; on that rare race the
	// spelling is kept and the tool layer still canonicalizes per call.
	if cfg.ToolsEnabled && cfg.WorkspaceRoot != "" {
		if canon, err := canonicalDir(cfg.WorkspaceRoot); err == nil {
			cfg.WorkspaceRoot = canon
		}
	}

	return cfg, nil
}

// ConfigPath returns the config file path this Config was resolved to.
func (c *Config) ConfigPath() string { return c.filePath }

func applyFile(c *Config, f fileConfig) {
	if f.Host != nil {
		c.Host = *f.Host
	}
	if f.AuthToken != nil {
		c.AuthToken = *f.AuthToken
	}
	if f.DefaultModel != nil {
		c.DefaultModel = *f.DefaultModel
	}
	if f.Theme != nil {
		c.Theme = *f.Theme
	}
	if f.WorkspaceRoot != nil {
		c.WorkspaceRoot = *f.WorkspaceRoot
	}
	if f.ToolsEnabled != nil {
		c.ToolsEnabled = *f.ToolsEnabled
	}
	if f.Agent == nil {
		return
	}
	if f.Agent.Temperature != nil {
		c.Agent.Temperature = *f.Agent.Temperature
	}
	if f.Agent.TopP != nil {
		c.Agent.TopP = *f.Agent.TopP
	}
	if f.Agent.NumCtx != nil {
		c.Agent.NumCtx = *f.Agent.NumCtx
	}
	if f.Agent.SystemPrompt != nil {
		c.Agent.SystemPrompt = *f.Agent.SystemPrompt
	}
	if f.Agent.MaxToolIterations != nil {
		c.Agent.MaxToolIterations = *f.Agent.MaxToolIterations
	}
}

func applyEnv(c *Config) error {
	if v := os.Getenv(envPrefix + "HOST"); v != "" {
		c.Host = v
	}
	if v := os.Getenv(envPrefix + "THEME"); v != "" {
		c.Theme = v
	}
	if v := os.Getenv(envPrefix + "AUTH_TOKEN"); v != "" {
		c.AuthToken = v
	}
	if v := os.Getenv(envPrefix + "DEFAULT_MODEL"); v != "" {
		c.DefaultModel = v
	}
	if v := os.Getenv(envPrefix + "WORKSPACE_ROOT"); v != "" {
		c.WorkspaceRoot = v
	}
	if v := os.Getenv(envPrefix + "TOOLS_ENABLED"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse %sTOOLS_ENABLED=%q: %w", envPrefix, v, err)
		}
		c.ToolsEnabled = parsed
	}
	if v := os.Getenv(envPrefix + "AGENT_TEMPERATURE"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("parse %sAGENT_TEMPERATURE=%q: %w", envPrefix, v, err)
		}
		c.Agent.Temperature = parsed
	}
	if v := os.Getenv(envPrefix + "AGENT_TOP_P"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("parse %sAGENT_TOP_P=%q: %w", envPrefix, v, err)
		}
		c.Agent.TopP = parsed
	}
	if v := os.Getenv(envPrefix + "AGENT_NUM_CTX"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse %sAGENT_NUM_CTX=%q: %w", envPrefix, v, err)
		}
		c.Agent.NumCtx = parsed
	}
	if v := os.Getenv(envPrefix + "AGENT_SYSTEM_PROMPT"); v != "" {
		c.Agent.SystemPrompt = v
	}
	if v := os.Getenv(envPrefix + "AGENT_MAX_TOOL_ITERATIONS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse %sAGENT_MAX_TOOL_ITERATIONS=%q: %w", envPrefix, v, err)
		}
		c.Agent.MaxToolIterations = parsed
	}
	return nil
}

func applyOverrides(c *Config, ov Overrides) {
	if ov.Host != nil {
		c.Host = *ov.Host
	}
	if ov.AuthToken != nil {
		c.AuthToken = *ov.AuthToken
	}
	if ov.Theme != nil {
		c.Theme = *ov.Theme
	}
	if ov.DefaultModel != nil {
		c.DefaultModel = *ov.DefaultModel
	}
	if ov.WorkspaceRoot != nil {
		c.WorkspaceRoot = *ov.WorkspaceRoot
	}
	if ov.Temperature != nil {
		c.Agent.Temperature = *ov.Temperature
	}
	if ov.TopP != nil {
		c.Agent.TopP = *ov.TopP
	}
	if ov.NumCtx != nil {
		c.Agent.NumCtx = *ov.NumCtx
	}
	if ov.SystemPrompt != nil {
		c.Agent.SystemPrompt = *ov.SystemPrompt
	}
	if ov.MaxToolIterations != nil {
		c.Agent.MaxToolIterations = *ov.MaxToolIterations
	}
}

// Save writes the current Config state to the resolved config path. The value
// is validated first — config.Validate is the single policy, reused by Load,
// Save, and the Settings form — then written atomically via writeFileAtomic so
// a failure at any point leaves any previous file untouched.
func Save(c Config) error {
	if err := Validate(c); err != nil {
		return err
	}

	path := c.filePath
	if path == "" {
		p, err := xdg.ConfigFile(filepath.Join("selftui", "config.toml"))
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		path = p
	}

	saved := fileConfig{
		Host:          ptr(c.Host),
		AuthToken:     ptr(c.AuthToken),
		DefaultModel:  ptr(c.DefaultModel),
		Theme:         ptr(c.Theme),
		WorkspaceRoot: ptr(c.WorkspaceRoot),
		ToolsEnabled:  ptr(c.ToolsEnabled),
		Agent: &fileAgentConfig{
			Temperature:       &c.Agent.Temperature,
			TopP:              &c.Agent.TopP,
			NumCtx:            &c.Agent.NumCtx,
			SystemPrompt:      ptr(c.Agent.SystemPrompt),
			MaxToolIterations: &c.Agent.MaxToolIterations,
		},
	}

	b, err := toml.Marshal(saved)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return writeFileAtomic(path, b)
}

// writeFileAtomic writes b to path through a temp file in the same directory
// (0600), fsynced and renamed over the target, so a reader never observes a
// partially written config and a crash cannot corrupt the file. The temp file
// is removed on every failure; directories created along the way are 0700.
func writeFileAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, ".selftui-config-*.tmp")
	if err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	tmp := f.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmp) // best effort: never leave a temp file behind
		}
	}()

	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	committed = true

	// The temp file is 0600 by construction (CreateTemp + chmod) and rename
	// preserves the mode; re-assert 0600 on the final path so a config that
	// may hold an auth token is never world-readable.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

func ptr[T any](v T) *T { return &v }
