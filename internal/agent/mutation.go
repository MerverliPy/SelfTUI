package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
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

// atomicWrite atomically creates or replaces one regular file at path. The
// path must be the canonical absolute target produced by securePath or
// secureWritePath (every component a real directory or file at validation
// time, no symlinks). The commit is performed with descriptor-relative
// operations so a symlink swap of an ancestor directory (or of the final
// component) between validation and commit fails closed instead of
// redirecting the write outside the workspace:
//
//   - every path component down to the parent is opened O_NOFOLLOW, pinning
//     the validated directory with a file descriptor (a symlink component
//     fails the open; a later rename of the pinned directory cannot redirect
//     the fd-relative operations);
//   - the temporary file is created O_EXCL inside the pinned directory and
//     renamed with renameat, never by pathname;
//   - the final component is re-checked with fstatat(AT_SYMLINK_NOFOLLOW): a
//     target that is no longer a regular file (e.g. a freshly planted
//     symlink) refuses the whole operation before anything is written.
//
// This is the single write primitive shared by write_file, edit_file,
// write_files, journal undo/redo restores, and rollback restores.
func atomicWrite(path string, content []byte, mode os.FileMode, tool, requested string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s %q: write target %q is not absolute (internal error)", tool, requested, path)
	}
	dir, base := filepath.Split(path)
	dirfd, err := openDirChain(filepath.Clean(dir))
	if err != nil {
		return fmt.Errorf("%s %q: write path changed since validation (%v); refusing to write", tool, requested, err)
	}
	defer unix.Close(dirfd)
	// The final component must still be a regular file or absent. Following
	// (or silently replacing) a symlink planted here after validation would
	// write outside the workspace, so any non-regular target refuses.
	var st unix.Stat_t
	fstatErr := unix.Fstatat(dirfd, base, &st, unix.AT_SYMLINK_NOFOLLOW)
	switch {
	case fstatErr == nil && st.Mode&unix.S_IFMT != unix.S_IFREG:
		return fmt.Errorf("%s %q: write target changed since validation (no longer a regular file); refusing to write", tool, requested)
	case fstatErr != nil && !errors.Is(fstatErr, unix.ENOENT):
		return fmt.Errorf("%s %q: stat write target: %w", tool, requested, fstatErr)
	}
	temp, file, err := createTempInDir(dirfd, mode)
	if err != nil {
		return fmt.Errorf("%s %q: create temporary file: %w", tool, requested, err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		file.Close()
		unix.Unlinkat(dirfd, temp, 0) // best-effort cleanup of the temp file
	}()
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("%s %q: write: %w", tool, requested, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("%s %q: sync: %w", tool, requested, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("%s %q: close: %w", tool, requested, err)
	}
	if err := unix.Renameat(dirfd, temp, dirfd, base); err != nil {
		return fmt.Errorf("%s %q: replace: %w", tool, requested, err)
	}
	committed = true
	return nil
}

// openDirChain opens dir with no-follow semantics, one component at a time
// from the filesystem root, and returns a descriptor pinned to dir. The
// descriptor pins the validated directory: fd-relative operations (temp
// creation, rename, unlink) keep hitting that directory even if its path is
// renamed or swapped afterwards. Any symlink or missing component — a tree
// that changed since the canonical path was validated — fails the open.
func openDirChain(dir string) (int, error) {
	if !filepath.IsAbs(dir) {
		return -1, fmt.Errorf("directory %q is not absolute (internal error)", dir)
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open filesystem root: %w", err)
	}
	for _, comp := range strings.Split(strings.TrimPrefix(filepath.Clean(dir), string(filepath.Separator)), string(filepath.Separator)) {
		if comp == "" || comp == "." {
			continue
		}
		if comp == ".." {
			unix.Close(fd)
			return -1, errors.New("path contains .. (internal error)")
		}
		next, err := unix.Openat(fd, comp, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			if errors.Is(err, unix.ELOOP) {
				return -1, fmt.Errorf("directory %q is now a symlink", comp)
			}
			return -1, fmt.Errorf("open directory %q: %w", comp, err)
		}
		fd = next
	}
	return fd, nil
}

// createTempInDir creates a uniquely named regular file inside the pinned
// directory fd with O_EXCL (a symlink pre-planted at the candidate name is
// never followed) and forces the exact requested mode on it.
func createTempInDir(dirfd int, mode os.FileMode) (string, *os.File, error) {
	for i := 0; i < 128; i++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", nil, err
		}
		name := ".selftui-" + hex.EncodeToString(b[:])
		fd, err := unix.Openat(dirfd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
		if err != nil {
			if errors.Is(err, unix.EEXIST) {
				continue
			}
			return "", nil, err
		}
		if err := unix.Fchmod(fd, uint32(mode.Perm())); err != nil {
			unix.Close(fd)
			return "", nil, err
		}
		return name, os.NewFile(uintptr(fd), name), nil
	}
	return "", nil, errors.New("could not allocate a unique temporary file name")
}

// removeNoFollow removes the file at a canonical absolute path with the same
// no-follow protection as atomicWrite: a symlink or missing component in the
// path fails the removal instead of redirecting it outside the workspace.
// ENOENT at the final component is not an error (removal is idempotent).
func removeNoFollow(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("remove %q: path is not absolute (internal error)", path)
	}
	dir, base := filepath.Split(path)
	dirfd, err := openDirChain(filepath.Clean(dir))
	if err != nil {
		return err
	}
	defer unix.Close(dirfd)
	if err := unix.Unlinkat(dirfd, base, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return &os.PathError{Op: "remove", Path: path, Err: err}
	}
	return nil
}
