package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"selftui/internal/ollama"
)

// ToolPolicy is the opt-in workspace tool trust policy (v0.1 hardening).
//
// A Runner built through NewRunnerWithPolicy carries a policy: it advertises
// the six jailed V2c tools (Tools) and refuses to execute any path-based tool
// on a path AuthorizePath rejects. A Runner built through the compatibility
// constructor NewRunner carries no policy and is plain chat — no tool
// definition ever reaches the model, so nothing the model says can turn into
// a filesystem operation. The zero value is a fully armed policy; arming is
// expressed by which constructor the caller uses.
type ToolPolicy struct{}

// Tools returns the six V2c tools: read-only read_file/list_dir/grep, the
// confirmed write_file/edit_file, and the confirmed sandboxed run_command.
// The set is closed — no shell or interpreter is directly exposed.
func (ToolPolicy) Tools() []ollama.ToolDefinition {
	return AgentTools()
}

// sensitiveDirs are path components whose presence anywhere in a requested
// path is refused: these trees hold credentials (SSH keys, GPG keys, cloud
// provider and Kubernetes configs) that no workspace tool request may touch,
// even when the workspace itself would resolve inside them.
var sensitiveDirs = map[string]bool{
	".ssh":   true,
	".gnupg": true,
	".aws":   true,
	".azure": true,
	".kube":  true,
}

// forbiddenBasenames are credential file names rejected wherever they appear
// as the final path element. The .env family is matched as a prefix (.env,
// .env.local, .env.production, and any future .env.* variant) except the one
// template file that is safe to read and write: .env.example.
var forbiddenBasenames = map[string]bool{
	"credentials":      true,
	"credentials.json": true,
}

// AuthorizePath applies the sensitive-path policy to a requested path
// (relative to the workspace or absolute) before any tool touches it. It is
// lexical and independent of the canonical workspace containment in
// securePath/canonicalRoot — containment answers "can this path resolve
// inside the workspace?", this answers "is this path ever worth touching?".
// A path containing a sensitive component (.ssh, .gnupg, .aws, .azure, .kube
// or the .config/gcloud composite), or whose basename is a credential file
// (.env*, credentials, credentials.json), is refused; .env.example is the one
// dotenv file that stays allowed.
func (ToolPolicy) AuthorizePath(requested string) error {
	comps := splitPathComponents(requested)
	// A path that cleans to nothing (".", "./") has no named component to
	// police; containment answers whether it resolves inside the workspace.
	if len(comps) == 0 {
		return nil
	}
	for i, c := range comps {
		if sensitiveDirs[c] {
			return fmt.Errorf("path %q is not allowed by the workspace tool policy (path component %q)", requested, c)
		}
		if c == ".config" && i+1 < len(comps) && comps[i+1] == "gcloud" {
			return fmt.Errorf("path %q is not allowed by the workspace tool policy (path component %q)", requested, filepath.Join(".config", "gcloud"))
		}
	}
	base := comps[len(comps)-1]
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		if base == ".env.example" {
			return nil // the template carve-out
		}
		return fmt.Errorf("path %q is not allowed by the workspace tool policy (basename %q)", requested, base)
	}
	if forbiddenBasenames[base] {
		return fmt.Errorf("path %q is not allowed by the workspace tool policy (basename %q)", requested, base)
	}
	return nil
}

// splitPathComponents breaks a requested path into its named components on
// both separators, dropping empty and "." elements but keeping ".." so a
// traversal-shaped request still exposes any sensitive component it embeds
// (e.g. "../.ssh/config"). "~" is kept as a component for the same reason.
func splitPathComponents(path string) []string {
	cleaned := filepath.ToSlash(path)
	parts := strings.Split(cleaned, "/")
	comps := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		comps = append(comps, p)
	}
	return comps
}
