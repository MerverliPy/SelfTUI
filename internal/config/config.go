// Package config holds SelfTUI's runtime configuration.
//
// Resolution priority (highest first): command-line flags > environment
// variables > config file (~/.config/selftui/config.toml) > built-in defaults.
// See load.go for the chain and config_test.go for the priority tests.
package config

// Config is the full configuration surface defined by PLAN.md §5.
type Config struct {
	// Host is the Ollama base URL, e.g. http://localhost:11434.
	Host string
	// AuthToken is an optional bearer token for remote Ollama hosts.
	AuthToken string
	// DefaultModel is auto-selected for the agent chat (first /api/tags entry
	// at runtime when empty; qwen3:8b is the recommended agent model).
	DefaultModel string
	// Theme is "dark" or "light".
	Theme string
	// WorkspaceRoot is the project root the agent operates on (defaults to cwd).
	WorkspaceRoot string

	// Agent parameters.
	Agent AgentConfig

	// filePath records the config file this was loaded from (for UI display).
	filePath string
}

// AgentConfig mirrors the agent.* keys of PLAN.md §5.
type AgentConfig struct {
	Temperature       float64
	TopP              float64
	NumCtx            int
	SystemPrompt      string
	MaxToolIterations int // bounds each agent run
}

// Default returns the built-in defaults (PLAN.md §5).
func Default() Config {
	return Config{
		Host:          "http://localhost:11434",
		AuthToken:     "",
		DefaultModel:  "",
		Theme:         "dark",
		WorkspaceRoot: "",
		Agent: AgentConfig{
			Temperature:       0.7,
			TopP:              0.9,
			NumCtx:            4096,
			SystemPrompt:      defaultSystemPrompt,
			MaxToolIterations: 12,
		},
	}
}

// defaultSystemPrompt is the placeholder agent persona until M3 wires the
// real one. Kept here so the config tree is complete from M0 on.
const defaultSystemPrompt = "You are SelfTUI, a project-aware coding agent running locally against an Ollama host."
