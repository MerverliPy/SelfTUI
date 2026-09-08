package agent

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// V2d agent breadth (PLAN.md §10): git-awareness + project indexing. Both are
// read-only context building for the agent loop — no new execution primitive,
// no change to the closed tool schema, and no mutation path. The workspace
// context is injected as a second pinned system message (BudgetMessages keeps
// the whole leading system prefix) so the model starts every turn knowing the
// repo state and layout instead of discovering it tool call by tool call.

const (
	// maxWorkspaceContextBytes bounds the whole injected block. It rides in
	// the pinned system prefix, so an unbounded build would silently crowd
	// the 3/4-numCtx input budget every turn.
	maxWorkspaceContextBytes = 8 << 10
	// maxTreeEntries caps indexed files so a huge checkout degrades to a
	// truncated index instead of a multi-second walk.
	maxTreeEntries = 300
	// maxTreeDepth caps directory descent below the workspace root
	// (depth 0 = the root itself).
	maxTreeDepth = 4
	// maxGitStatusLines caps `git status --porcelain -b` output.
	maxGitStatusLines = 40
	// gitTimeout bounds every host-side git invocation. The commands are
	// fixed read-only argv with no model-controlled input; the timeout only
	// protects the UI turn against a wedged repository.
	gitTimeout = 3 * time.Second
)

// WorkspaceContext builds the bounded, deterministic context block injected
// into an armed agent turn: git branch/status/log when root is inside a git
// work tree (fixed read-only argv on the host process, never in the V2c
// sandbox — the sandbox is for model-requested commands) plus a bounded
// project tree. Sections that do not apply are omitted; a workspace that
// cannot be read at all yields "" so nothing is injected.
func WorkspaceContext(ctx context.Context, root string) string {
	if err := ctx.Err(); err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Workspace context (auto-generated, bounded; paths are relative to the workspace root):\n")
	if git := gitContext(ctx, root); git != "" {
		b.WriteString(git)
	}
	if tree := workspaceTree(ctx, root); tree != "" {
		b.WriteString("Project index:\n")
		b.WriteString(tree)
	}
	out := b.String()
	if out == "" {
		return ""
	}
	if len(out) > maxWorkspaceContextBytes {
		out = out[:maxWorkspaceContextBytes] + "\n[workspace context truncated]"
	}
	return out
}

// workspaceFilesMaxResults bounds the picker listing so a huge checkout
// degrades to a truncated list instead of an unbounded slice.
const workspaceFilesMaxResults = 512

// WorkspaceFiles lists the workspace's regular files as workspace-relative
// paths for the composer's @-file picker (PLAN.md §12 N6). It shares the
// workspaceTree walk discipline: symlinks are never followed (WalkDir), .git
// is pruned, depth is capped at maxTreeDepth, and entries are capped so a
// huge checkout degrades to a truncated list. The paths are exactly what
// securePath will re-verify when the reference is expanded, so nothing here
// is an authority — it is a convenience listing inside the same jail.
func WorkspaceFiles(ctx context.Context, root string) []string {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil
	}
	info, err := os.Stat(realRoot)
	if err != nil || !info.IsDir() {
		return nil
	}
	var files []string
	err = filepath.WalkDir(realRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == realRoot {
				return walkErr // unreadable root: no listing at all
			}
			return nil // unreadable child: skip, keep listing the rest
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, relErr := filepath.Rel(realRoot, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		depth := strings.Count(rel, string(filepath.Separator))
		if d.IsDir() && depth >= maxTreeDepth {
			return filepath.SkipDir
		}
		if len(files) >= workspaceFilesMaxResults {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return fs.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // sockets/devices/symlinks: never offered
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil // canceled: offer nothing rather than a partial list
		}
		// A real traversal error with partial content keeps what was read;
		// the listing is advisory, not a correctness surface.
	}
	return files
}

// gitContext runs the fixed read-only git probes. Any failure (no git binary,
// not a repository, canceled context) returns "" and the block simply omits
// the git section — a non-git workspace is a supported shape, not an error.
func gitContext(ctx context.Context, root string) string {
	if !gitProbe(ctx, root) {
		return ""
	}
	var b strings.Builder
	b.WriteString("Git:\n")
	if status := gitCapture(ctx, root, "status", "--porcelain", "-b"); status != "" {
		b.WriteString("Status (porcelain):\n")
		b.WriteString(ensureNewline(boundedLines(status, maxGitStatusLines)))
	}
	if log := gitCapture(ctx, root, "log", "--oneline", "-n", "3"); log != "" {
		b.WriteString("Recent commits:\n")
		b.WriteString(ensureNewline(log))
	}
	return b.String()
}

// gitProbe reports whether root is inside a git work tree.
func gitProbe(ctx context.Context, root string) bool {
	out, err := gitRun(ctx, root, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// gitCapture runs one read-only git command and returns its trimmed output,
// or "" on any error.
func gitCapture(ctx context.Context, root string, args ...string) string {
	out, err := gitRun(ctx, root, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// gitRun executes a fixed read-only git argv against root with a hard
// timeout. argv is always built from the call sites above — never from model
// output — so no shell, interpolation, or argument injection exists here.
func gitRun(ctx context.Context, root string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	gitCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(gitCtx, "git", args...)
	cmd.Dir = root
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // diagnostics are irrelevant; failures just omit the section
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// boundedLines returns at most n newline-delimited lines of s, with a
// truncation marker when lines were dropped.
func boundedLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n[truncated]"
}

// ensureNewline guarantees s ends with exactly one newline so consecutive
// sections never run together after TrimSpace.
func ensureNewline(s string) string {
	return strings.TrimRight(s, "\n") + "\n"
}

// workspaceTree indexes the workspace as an indented file tree: regular
// files and directories only, `.git` pruned (same convention as Grep),
// depth- and entry-capped, WalkDir's lexical order. WalkDir does not follow
// symlinks, so nothing outside the workspace is ever named.
func workspaceTree(ctx context.Context, root string) string {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ""
	}
	info, err := os.Stat(realRoot)
	if err != nil || !info.IsDir() {
		return ""
	}
	var b strings.Builder
	entries := 0
	truncated := false
	err = filepath.WalkDir(realRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == realRoot {
				return walkErr // unreadable root: no index at all
			}
			return nil // unreadable child: skip, keep indexing the rest
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, relErr := filepath.Rel(realRoot, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		depth := strings.Count(rel, string(filepath.Separator))
		if d.IsDir() && depth >= maxTreeDepth {
			truncated = true
			return filepath.SkipDir
		}
		if entries >= maxTreeEntries {
			truncated = true
			if d.IsDir() {
				return filepath.SkipDir
			}
			return fs.SkipAll
		}
		entries++
		b.WriteString(strings.Repeat("  ", depth))
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		b.WriteString(name)
		b.WriteString("\n")
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return "" // canceled: inject nothing rather than a partial index
		}
		// A real traversal error with partial content keeps what was read;
		// the block is advisory context, not a correctness surface.
	}
	if entries == 0 {
		return ""
	}
	if truncated {
		b.WriteString("[index truncated]\n")
	}
	return b.String()
}
