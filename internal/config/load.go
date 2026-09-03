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
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("read config %s: %w", *path, err)
	}

	// 2. environment.
	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}

	// 3. flags/overrides.
	applyOverrides(&cfg, ov)

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
	if v := os.Getenv(envPrefix + "AGENT_TEMPERATURE"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("parse %sTEMPERATURE=%q: %w", envPrefix, v, err)
		}
		c.Agent.Temperature = parsed
	}
	if v := os.Getenv(envPrefix + "AGENT_TOP_P"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("parse %sTOP_P=%q: %w", envPrefix, v, err)
		}
		c.Agent.TopP = parsed
	}
	if v := os.Getenv(envPrefix + "AGENT_NUM_CTX"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse %sNUM_CTX=%q: %w", envPrefix, v, err)
		}
		c.Agent.NumCtx = parsed
	}
	if v := os.Getenv(envPrefix + "AGENT_SYSTEM_PROMPT"); v != "" {
		c.Agent.SystemPrompt = v
	}
	if v := os.Getenv(envPrefix + "AGENT_MAX_TOOL_ITERATIONS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse %sMAX_TOOL_ITERATIONS=%q: %w", envPrefix, v, err)
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

// Save writes the current Config state to the resolved config path.
func Save(c Config) error {
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

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}

	return nil
}

func ptr[T any](v T) *T { return &v }
