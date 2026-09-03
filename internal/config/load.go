package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/pelletier/go-toml/v2"
)

// file keys loaded in M0. Agent keys keep defaults until M4.
type fileConfig struct {
	Host          string `toml:"host"`
	AuthToken     string `toml:"auth_token"`
	DefaultModel  string `toml:"default_model"`
	Theme         string `toml:"theme"`
	WorkspaceRoot string `toml:"workspace_root"`
}

// envPrefix is the shared prefix for SelfTUI environment variables.
const envPrefix = "SELFTUI_"

// Overrides carries flag (or other highest-priority) values. Non-nil fields
// win over env and file. ConfigPath overrides the config file location
// (default: $XDG_CONFIG_HOME/selftui/config.toml via xdg).
type Overrides struct {
	ConfigPath    *string
	Host          *string
	Theme         *string
	DefaultModel  *string
	WorkspaceRoot *string
}

// Load resolves the configuration: defaults → config file → env → overrides.
func Load(ov Overrides) (Config, error) {
	cfg := Default()

	// 1. config file (only if it exists).
	path := ov.ConfigPath
	file := fileConfig{}
	if path == nil {
		p, err := xdg.ConfigFile(filepath.Join("selftui", "config.toml"))
		if err != nil {
			return cfg, fmt.Errorf("resolve config path: %w", err)
		}
		path = &p
	}
	if b, err := os.ReadFile(*path); err == nil {
		if err := toml.Unmarshal(b, &file); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", *path, err)
		}
		applyFile(&cfg, file)
		cfg.filePath = *path
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("read config %s: %w", *path, err)
	}

	// 2. environment.
	applyEnv(&cfg)

	// 3. flags/overrides.
	applyOverrides(&cfg, ov)

	return cfg, nil
}

// ConfigPath returns the config file path this Config was loaded from, or ""
// if no file existed.
func (c *Config) ConfigPath() string { return c.filePath }

func applyFile(c *Config, f fileConfig) {
	if f.Host != "" {
		c.Host = f.Host
	}
	if f.AuthToken != "" {
		c.AuthToken = f.AuthToken
	}
	if f.DefaultModel != "" {
		c.DefaultModel = f.DefaultModel
	}
	if f.Theme != "" {
		c.Theme = f.Theme
	}
	if f.WorkspaceRoot != "" {
		c.WorkspaceRoot = f.WorkspaceRoot
	}
}

func applyEnv(c *Config) {
	if v := os.Getenv(envPrefix + "HOST"); v != "" {
		c.Host = v
	}
	if v := os.Getenv(envPrefix + "THEME"); v != "" {
		c.Theme = v
	}
	if v := os.Getenv(envPrefix + "DEFAULT_MODEL"); v != "" {
		c.DefaultModel = v
	}
	if v := os.Getenv(envPrefix + "WORKSPACE_ROOT"); v != "" {
		c.WorkspaceRoot = v
	}
}

func applyOverrides(c *Config, ov Overrides) {
	if ov.Host != nil {
		c.Host = *ov.Host
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
}
