package config

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// allEnvVars is every SELFTUI_* variable applyEnv reads. Source-driven tests
// blank all of them first so a developer's shell exports can never leak into
// a case (env beats the config file, so a leaked var would change the result).
var allEnvVars = []string{
	"HOST", "AUTH_TOKEN", "THEME", "DEFAULT_MODEL", "WORKSPACE_ROOT",
	"AGENT_TEMPERATURE", "AGENT_TOP_P", "AGENT_NUM_CTX",
	"AGENT_SYSTEM_PROMPT", "AGENT_MAX_TOOL_ITERATIONS",
}

func blankEnv(t *testing.T) {
	t.Helper()
	for _, v := range allEnvVars {
		t.Setenv(envPrefix+v, "")
	}
}

// envFrom sets the process environment to mirror applyEnv(c): every non-empty
// config field becomes its SELFTUI_* variable, and everything else is blanked.
// The numeric agent fields are always set so an intentional boundary value
// (e.g. temperature 0) survives the env round trip.
func envFrom(t *testing.T, c Config) {
	t.Helper()
	blankEnv(t)
	if c.Host != "" {
		t.Setenv(envPrefix+"HOST", c.Host)
	}
	if c.AuthToken != "" {
		t.Setenv(envPrefix+"AUTH_TOKEN", c.AuthToken)
	}
	if c.DefaultModel != "" {
		t.Setenv(envPrefix+"DEFAULT_MODEL", c.DefaultModel)
	}
	if c.Theme != "" {
		t.Setenv(envPrefix+"THEME", c.Theme)
	}
	if c.WorkspaceRoot != "" {
		t.Setenv(envPrefix+"WORKSPACE_ROOT", c.WorkspaceRoot)
	}
	t.Setenv(envPrefix+"AGENT_TEMPERATURE", strconv.FormatFloat(c.Agent.Temperature, 'f', -1, 64))
	t.Setenv(envPrefix+"AGENT_TOP_P", strconv.FormatFloat(c.Agent.TopP, 'f', -1, 64))
	t.Setenv(envPrefix+"AGENT_NUM_CTX", strconv.Itoa(c.Agent.NumCtx))
	t.Setenv(envPrefix+"AGENT_MAX_TOOL_ITERATIONS", strconv.Itoa(c.Agent.MaxToolIterations))
	if c.Agent.SystemPrompt != "" {
		t.Setenv(envPrefix+"AGENT_SYSTEM_PROMPT", c.Agent.SystemPrompt)
	}
}

// tomlSource renders c the way Save would, so the file source carries exactly
// the same values as the env and overrides sources.
func tomlSource(t *testing.T, c Config) string {
	t.Helper()
	f := fileConfig{
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
	b, err := toml.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// overrideSource returns an Overrides carrying every field of c and a config
// path that does not exist (so no real file participates).
func overrideSource(t *testing.T, c Config) Overrides {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "absent.toml")
	return Overrides{
		ConfigPath:        &missing,
		Host:              &c.Host,
		AuthToken:         &c.AuthToken,
		Theme:             &c.Theme,
		DefaultModel:      &c.DefaultModel,
		WorkspaceRoot:     &c.WorkspaceRoot,
		Temperature:       &c.Agent.Temperature,
		TopP:              &c.Agent.TopP,
		NumCtx:            &c.Agent.NumCtx,
		SystemPrompt:      &c.Agent.SystemPrompt,
		MaxToolIterations: &c.Agent.MaxToolIterations,
	}
}

// loadSource materializes c through one of the three configuration sources and
// returns the Load error (nil when the value is accepted).
func loadSource(t *testing.T, source string, c Config) error {
	t.Helper()
	switch source {
	case "toml":
		p := writeFile(t, tomlSource(t, c))
		_, err := Load(Overrides{ConfigPath: &p})
		return err
	case "env":
		envFrom(t, c)
		missing := filepath.Join(t.TempDir(), "absent.toml")
		_, err := Load(Overrides{ConfigPath: &missing})
		return err
	default: // overrides
		_, err := Load(overrideSource(t, c))
		return err
	}
}

var allSources = []string{"toml", "env", "overrides"}

// TestValidateRejectsInvalidValuesFromEverySource proves the validation policy
// is source-independent: each invalid value yields the exact same stable error
// whether it reached the config through the TOML file, the environment, or
// Overrides.
func TestValidateRejectsInvalidValuesFromEverySource(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "no-such-workspace")
	cases := []struct {
		name   string
		want   string
		mutate func(*Config)
	}{
		{"host scheme not http(s)",
			"config: host: scheme must be http or https",
			func(c *Config) { c.Host = "ftp://ollama.example:11434" }},
		{"host missing hostname",
			"config: host: missing hostname",
			func(c *Config) { c.Host = "http://" }},
		{"host userinfo rejected",
			"config: host: userinfo is not allowed",
			func(c *Config) { c.Host = "http://user:pass@ollama.example:11434" }},
		{"host query rejected",
			"config: host: query string is not allowed",
			func(c *Config) { c.Host = "http://ollama.example:11434?x=1" }},
		{"host fragment rejected",
			"config: host: fragment is not allowed",
			func(c *Config) { c.Host = "http://ollama.example:11434#frag" }},
		{"auth token over plain http non-loopback",
			"config: host: bearer token requires HTTPS for non-loopback host",
			func(c *Config) { c.Host = "http://ollama.example:11434"; c.AuthToken = "sekrit" }},
		{"theme not dark or light",
			`config: theme: must be "dark" or "light"`,
			func(c *Config) { c.Theme = "blue" }},
		{"temperature above range",
			"config: agent: temperature must be between 0 and 2",
			func(c *Config) { c.Agent.Temperature = 2.5 }},
		{"temperature below range",
			"config: agent: temperature must be between 0 and 2",
			func(c *Config) { c.Agent.Temperature = -0.1 }},
		{"top_p above range",
			"config: agent: top_p must be between 0 and 1",
			func(c *Config) { c.Agent.TopP = 1.5 }},
		{"num_ctx below range",
			"config: agent: num_ctx must be between 128 and 1048576",
			func(c *Config) { c.Agent.NumCtx = 127 }},
		{"num_ctx above range",
			"config: agent: num_ctx must be between 128 and 1048576",
			func(c *Config) { c.Agent.NumCtx = 1048577 }},
		{"max_tool_iterations below range",
			"config: agent: max_tool_iterations must be between 1 and 100",
			func(c *Config) { c.Agent.MaxToolIterations = 0 }},
		{"max_tool_iterations above range",
			"config: agent: max_tool_iterations must be between 1 and 100",
			func(c *Config) { c.Agent.MaxToolIterations = 101 }},
		{"workspace_root does not exist",
			fmt.Sprintf("config: workspace_root: %s does not exist or is not a directory", missingDir),
			func(c *Config) { c.WorkspaceRoot = missingDir }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, source := range allSources {
				t.Run(source, func(t *testing.T) {
					c := Default()
					tc.mutate(&c)
					err := loadSource(t, source, c)
					if err == nil {
						t.Fatalf("%s: Load accepted invalid config; want error %q", source, tc.want)
					}
					if got := err.Error(); got != tc.want {
						t.Errorf("%s: error = %q, want exact stable error %q", source, got, tc.want)
					}
				})
			}
		})
	}
}

// TestValidateAcceptsValidValuesFromEverySource pins the inclusive boundaries
// and the loopback-token allowance, again through all three sources.
func TestValidateAcceptsValidValuesFromEverySource(t *testing.T) {
	workspace := t.TempDir()
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"token over http localhost",
			func(c *Config) { c.Host = "http://localhost:11434"; c.AuthToken = "tok" }},
		{"token over http 127.0.0.1",
			func(c *Config) { c.Host = "http://127.0.0.1:11434"; c.AuthToken = "tok" }},
		{"token over http ::1",
			func(c *Config) { c.Host = "http://[::1]:11434"; c.AuthToken = "tok" }},
		{"token over https remote",
			func(c *Config) { c.Host = "https://ollama.example:11434"; c.AuthToken = "tok" }},
		{"https remote without token",
			func(c *Config) { c.Host = "https://ollama.example:11434" }},
		{"http remote without token",
			func(c *Config) { c.Host = "http://ollama.example:11434" }},
		{"temperature lower boundary", func(c *Config) { c.Agent.Temperature = 0 }},
		{"temperature upper boundary", func(c *Config) { c.Agent.Temperature = 2 }},
		{"top_p lower boundary", func(c *Config) { c.Agent.TopP = 0 }},
		{"top_p upper boundary", func(c *Config) { c.Agent.TopP = 1 }},
		{"num_ctx lower boundary", func(c *Config) { c.Agent.NumCtx = 128 }},
		{"num_ctx upper boundary", func(c *Config) { c.Agent.NumCtx = 1048576 }},
		{"max_tool_iterations lower boundary", func(c *Config) { c.Agent.MaxToolIterations = 1 }},
		{"max_tool_iterations upper boundary", func(c *Config) { c.Agent.MaxToolIterations = 100 }},
		{"theme light", func(c *Config) { c.Theme = "light" }},
		{"workspace_root existing directory", func(c *Config) { c.WorkspaceRoot = workspace }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, source := range allSources {
				t.Run(source, func(t *testing.T) {
					c := Default()
					tc.mutate(&c)
					if err := loadSource(t, source, c); err != nil {
						t.Errorf("%s: Load rejected valid config: %v", source, err)
					}
				})
			}
		})
	}
}

// TestValidateHostLevelRules drills into host parsing without the source
// plumbing: empty/whitespace hosts, scheme-less URLs, and a malformed URL all
// get a stable config: host: error.
func TestValidateHostLevelRules(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"", "config: host: missing hostname"},
		{"   ", "config: host: missing hostname"},
		{"http://", "config: host: missing hostname"},
		{"ftp://host", "config: host: scheme must be http or https"},
		{"localhost:11434", "config: host: scheme must be http or https"},
		{"http://user@host:11434", "config: host: userinfo is not allowed"},
		{"http://host:11434?x=1", "config: host: query string is not allowed"},
		{"http://host:11434#f", "config: host: fragment is not allowed"},
		{"http://[::1", "config: host: must be a valid http:// or https:// URL"},
	}
	for _, tc := range cases {
		c := Default()
		c.Host = tc.host
		if err := Validate(c); err == nil || err.Error() != tc.want {
			t.Errorf("Validate(host %q) error = %v, want %q", tc.host, err, tc.want)
		}
	}
}

func TestValidateRejectsNaN(t *testing.T) {
	c := Default()
	c.Agent.Temperature = math.NaN()
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Errorf("NaN temperature: error = %v, want a temperature error", err)
	}
}
