// Package agent contains SelfTUI's bounded coding-agent loop and its jailed
// filesystem tools.
package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"selftui/internal/ollama"
)

const (
	maxReadBytes   = 256 << 10
	maxSearchBytes = 1 << 20
	maxResultBytes = 64 << 10
)

// AgentTools is the explicit schema exposed to models. Keep this list closed:
// an unknown model-generated name can never become an execution primitive.
func AgentTools() []ollama.ToolDefinition {
	stringArg := func(name, description string) ollama.ToolFunction {
		return ollama.ToolFunction{
			Name: name, Description: description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			},
		}
	}
	return []ollama.ToolDefinition{
		{Type: "function", Function: stringArg("read_file", "Read a UTF-8 file inside the workspace.")},
		{Type: "function", Function: stringArg("list_dir", "List one directory inside the workspace.")},
		{Type: "function", Function: ollama.ToolFunction{
			Name: "grep", Description: "Search text with a regular expression inside the workspace.",
			Parameters: map[string]any{
				"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}},
				"required": []string{"pattern", "path"},
			},
		}},
		{Type: "function", Function: ollama.ToolFunction{
			Name: "write_file", Description: "Atomically write a file inside the workspace. Requires user approval.",
			Parameters: map[string]any{
				"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}, "overwrite": map[string]any{"type": "boolean"}},
				"required": []string{"path", "content"},
			},
		}},
		{Type: "function", Function: ollama.ToolFunction{
			Name: "edit_file", Description: "Replace one exact string in a workspace file. Requires user approval.",
			Parameters: map[string]any{
				"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "old": map[string]any{"type": "string"}, "new": map[string]any{"type": "string"}},
				"required": []string{"path", "old", "new"},
			},
		}},
		{Type: "function", Function: ollama.ToolFunction{
			Name: "run_command", Description: "Run an approved allowlisted argv in the workspace. Requires user approval.",
			Parameters: map[string]any{
				"type": "object", "properties": map[string]any{"argv": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "timeout": map[string]any{"type": "integer"}},
				"required": []string{"argv"},
			},
		}},
	}
}

// ReadOnlyTools remains available for focused M3a compatibility tests and
// callers. It deliberately excludes every mutation primitive.
func ReadOnlyTools() []ollama.ToolDefinition {
	return AgentTools()[:3]
}

func ReadFile(root, path string) (string, error) {
	p, err := securePath(root, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("read_file %q: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("read_file %q: is a directory", path)
	}
	if info.Size() > maxReadBytes {
		return "", fmt.Errorf("read_file %q: file exceeds %d-byte limit", path, maxReadBytes)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("read_file %q: %w", path, err)
	}
	return string(b), nil
}

func ListDir(root, path string) (string, error) {
	p, err := securePath(root, path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return "", fmt.Errorf("list_dir %q: %w", path, err)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	return boundedResult(strings.Join(lines, "\n")), nil
}

func Grep(root, pattern, path string) (string, error) {
	rootReal, err := canonicalRoot(root)
	if err != nil {
		return "", err
	}
	if pattern == "" {
		return "", errors.New("grep: empty pattern")
	}
	rx, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("grep: invalid pattern: %w", err)
	}
	p, err := securePath(rootReal, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("grep %q: %w", path, err)
	}
	var files []string
	if info.IsDir() {
		err = filepath.WalkDir(p, func(file string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink == 0 {
				files = append(files, file)
			}
			return nil
		})
	} else if !info.Mode().IsRegular() {
		return "", fmt.Errorf("grep %q: not a regular file or directory", path)
	} else {
		files = []string{p}
	}
	if err != nil {
		return "", fmt.Errorf("grep %q: walk: %w", path, err)
	}
	sort.Strings(files)

	var out strings.Builder
	for _, file := range files {
		if out.Len() >= maxResultBytes {
			break
		}
		if err := grepFile(rootReal, file, rx, &out); err != nil {
			return "", err
		}
	}
	if out.Len() > maxResultBytes {
		return out.String()[:maxResultBytes] + "\n[output truncated]", nil
	}
	return out.String(), nil
}

func grepFile(root, file string, rx *regexp.Regexp, out *strings.Builder) error {
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	if info.Size() > maxSearchBytes {
		return nil // skip likely binary/large generated files
	}
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("grep %q: %w", file, err)
	}
	defer f.Close()
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(io.LimitReader(f, maxSearchBytes+1))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if rx.MatchString(line) {
			fmt.Fprintf(out, "%s:%d:%s\n", rel, lineNo, line)
			if out.Len() >= maxResultBytes {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("grep %q: %w", rel, err)
	}
	return nil
}

// securePath resolves both the workspace and the requested path before
// comparing them. This rejects ../ escapes and symlinks that point outside
// the workspace; the working directory alone is not a security boundary.
func securePath(root, requested string) (string, error) {
	rootReal, err := canonicalRoot(root)
	if err != nil {
		return "", err
	}
	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootReal, candidate)
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", requested, err)
	}
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", requested, err)
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace", requested)
	}
	return real, nil
}

func canonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("workspace root is empty")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("workspace root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("workspace root: %w", err)
	}
	info, err := os.Stat(rootReal)
	if err != nil {
		return "", fmt.Errorf("workspace root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace root %q is not a directory", root)
	}
	return rootReal, nil
}

func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("missing arguments")
	}
	// Some content-embedded emitters put the arguments object in a JSON
	// string, while native Ollama calls use an object. Accept both forms but
	// still decode into the strict destination below.
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		raw = json.RawMessage(encoded)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func boundedResult(s string) string {
	if len(s) <= maxResultBytes {
		return s
	}
	return s[:maxResultBytes] + "\n[output truncated]"
}
