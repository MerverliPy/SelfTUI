package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Validate enforces SelfTUI's configuration policy. It is the single source of
// truth for value rules and is reused everywhere a Config is accepted or
// produced: Load validates after all sources are applied, Save validates
// before marshaling, and the Settings form delegates its field checks here.
//
// Errors are stable and field-prefixed ("config: host: …") so every surface —
// startup load, a settings save, the Settings form — reports the same message
// no matter which source supplied the value. The first violation in a fixed
// field order is returned.
func Validate(c Config) error {
	if err := validateHost(c.Host, c.AuthToken); err != nil {
		return err
	}
	if c.Theme != "dark" && c.Theme != "light" {
		return fmt.Errorf(`config: theme: must be "dark" or "light"`)
	}

	if err := validateAgentRange("temperature", c.Agent.Temperature, 0, 2); err != nil {
		return err
	}
	if err := validateAgentRange("top_p", c.Agent.TopP, 0, 1); err != nil {
		return err
	}
	if err := validateAgentIntRange("num_ctx", c.Agent.NumCtx, 128, 1_048_576); err != nil {
		return err
	}
	if err := validateAgentIntRange("max_tool_iterations", c.Agent.MaxToolIterations, 1, 100); err != nil {
		return err
	}

	if c.WorkspaceRoot != "" {
		st, err := os.Stat(c.WorkspaceRoot)
		if err != nil || !st.IsDir() {
			return fmt.Errorf("config: workspace_root: %s does not exist or is not a directory", c.WorkspaceRoot)
		}
	}

	// Workspace tools are opt-in and only safe against a real project root.
	// With tools enabled, an empty root (→ cwd, unpredictable), "/" (the
	// whole filesystem) or the user's home directory (adjacent to
	// .ssh/.gnupg/.aws/…) would hand the model a much broader filesystem than
	// a workspace; reject that pairing outright.
	if c.ToolsEnabled {
		if err := validateToolsWorkspace(c.WorkspaceRoot); err != nil {
			return err
		}
	}
	return nil
}

// validateToolsWorkspace enforces the tools_enabled/workspace_root pairing
// (see Validate). workspaceRoot is compared after cleaning, so a trailing
// slash on the home directory does not dodge the rule.
func validateToolsWorkspace(workspaceRoot string) error {
	if workspaceRoot == "" {
		return errors.New("config: tools_enabled: workspace_root is required when tools are enabled")
	}
	if workspaceRoot == "/" {
		return errors.New("config: tools_enabled: workspace_root must not be / when tools are enabled")
	}
	home, err := os.UserHomeDir()
	if err == nil && filepath.Clean(workspaceRoot) == filepath.Clean(home) {
		return errors.New("config: tools_enabled: workspace_root must not be your home directory when tools are enabled")
	}
	return nil
}

func validateAgentRange(field string, v, lo, hi float64) error {
	if math.IsNaN(v) || v < lo || v > hi {
		return fmt.Errorf("config: agent: %s must be between %g and %g", field, lo, hi)
	}
	return nil
}

func validateAgentIntRange(field string, v, lo, hi int) error {
	if v < lo || v > hi {
		return fmt.Errorf("config: agent: %s must be between %d and %d", field, lo, hi)
	}
	return nil
}

// validateHost checks the Ollama base URL. Rules: the scheme must be exactly
// http or https; a hostname must be present; userinfo, query strings and
// fragments are rejected (none of them belong in an Ollama base URL); and a
// bearer token over plain http is only accepted for loopback hosts, where the
// traffic never leaves the machine.
func validateHost(host, token string) error {
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("config: host: missing hostname")
	}
	u, err := url.Parse(host)
	if err != nil {
		return fmt.Errorf("config: host: must be a valid http:// or https:// URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("config: host: scheme must be http or https")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("config: host: missing hostname")
	}
	if u.User != nil {
		return fmt.Errorf("config: host: userinfo is not allowed")
	}
	if u.RawQuery != "" {
		return fmt.Errorf("config: host: query string is not allowed")
	}
	if u.Fragment != "" {
		return fmt.Errorf("config: host: fragment is not allowed")
	}
	if token != "" && scheme != "https" && !LoopbackHost(host) {
		return fmt.Errorf("config: host: bearer token requires HTTPS for non-loopback host")
	}
	return nil
}

// LoopbackHost reports whether host — an http(s) base URL or a bare
// hostname — is one of the loopback spellings SelfTUI treats as local
// (localhost, 127.0.0.1, ::1). The UI reuses it to warn when workspace
// content could leave the machine; validateHost uses it for the plain-http
// token rule.
func LoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	if u, err := url.Parse(h); err == nil && u.Hostname() != "" {
		h = u.Hostname()
	}
	switch strings.ToLower(h) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
