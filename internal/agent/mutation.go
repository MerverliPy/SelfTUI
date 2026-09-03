package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteFile atomically creates or, with explicit permission, replaces one
// regular file under root. Both the destination (when present) and its parent
// are resolved before writing, so a symlink cannot redirect the operation.
func WriteFile(root, requested, content string, overwrite bool) error {
	path, err := secureWritePath(root, requested)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("write_file %q: not a regular file", requested)
		}
		if !overwrite {
			return fmt.Errorf("write_file %q: already exists (set overwrite to true)", requested)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("write_file %q: %w", requested, err)
	}
	mode := os.FileMode(0o600)
	if info != nil {
		mode = info.Mode().Perm()
	}
	return atomicWrite(path, []byte(content), mode, "write_file", requested)
}

// EditFile replaces exactly one occurrence of old in an existing regular file
// under root. Requiring one match makes the model's mutation deterministic.
func EditFile(root, requested, old, new string) error {
	if old == "" {
		return errors.New("edit_file: old must not be empty")
	}
	path, err := securePath(root, requested)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("edit_file %q: %w", requested, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("edit_file %q: not a regular file", requested)
	}
	if info.Size() > maxReadBytes {
		return fmt.Errorf("edit_file %q: file exceeds %d-byte limit", requested, maxReadBytes)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("edit_file %q: %w", requested, err)
	}
	if strings.Count(string(body), old) != 1 {
		return fmt.Errorf("edit_file %q: exact old text not found exactly once", requested)
	}
	return atomicWrite(path, []byte(strings.Replace(string(body), old, new, 1)), info.Mode().Perm(), "edit_file", requested)
}

func secureWritePath(root, requested string) (string, error) {
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
	parent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve parent for %q: %w", requested, err)
	}
	if !withinRoot(rootReal, parent) {
		return "", fmt.Errorf("path %q escapes workspace", requested)
	}
	if _, err := os.Lstat(candidate); err == nil {
		return securePath(rootReal, requested)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve path %q: %w", requested, err)
	}
	return candidate, nil
}

func withinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func atomicWrite(path string, content []byte, mode os.FileMode, tool, requested string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".selftui-*")
	if err != nil {
		return fmt.Errorf("%s %q: create temporary file: %w", tool, requested, err)
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return fmt.Errorf("%s %q: set mode: %w", tool, requested, err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return fmt.Errorf("%s %q: write: %w", tool, requested, err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("%s %q: sync: %w", tool, requested, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("%s %q: close: %w", tool, requested, err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("%s %q: replace: %w", tool, requested, err)
	}
	return nil
}
