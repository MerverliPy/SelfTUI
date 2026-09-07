package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultCommandTimeout = 30 * time.Second
	maxCommandTimeout     = 60 * time.Second
	maxCommandOutput      = 256 << 10
)

// commandSlot serializes all command invocations. A channel, rather than a
// mutex, lets cancellation abort a command that is waiting behind another
// invocation instead of making the caller wait for an unrelated timeout.
var commandSlot = make(chan struct{}, 1)

// RunCommand executes the deliberately small argv allowlist inside the
// bubblewrap sandbox. It accepts argv, never a shell string. The sandbox is
// the primary V2c containment boundary; timeout, bounded output, process-group
// cancellation, and serialization remain enforced by this executor too.
func RunCommand(ctx context.Context, root string, argv []string, timeoutSeconds int, emit func(stream, text string)) (string, error) {
	rootReal, err := canonicalRoot(root)
	if err != nil {
		return "", err
	}
	if err := validateCommand(argv); err != nil {
		return "", err
	}

	timeout := defaultCommandTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	if timeout > maxCommandTimeout {
		return "", fmt.Errorf("run_command: timeout exceeds %s", maxCommandTimeout)
	}
	if err := acquireCommand(ctx); err != nil {
		return "", err
	}
	defer releaseCommand()

	binary, sandboxBinary, err := allowlistedBinary(argv[0])
	if err != nil {
		return "", err
	}
	bwrap, err := bubblewrapBinary()
	if err != nil {
		return "", err
	}
	args, err := sandboxArgs(rootReal, binary, sandboxBinary, argv[1:])
	if err != nil {
		return "", err
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := execCtx.Err(); err != nil {
		return "", err
	}
	stdout := &commandOutput{stream: "stdout", emit: emit}
	stderr := &commandOutput{stream: "stderr", emit: emit}
	cmd := exec.Command(bwrap, args...)
	cmd.Dir = rootReal
	cmd.Env = commandEnv()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// The outer bwrap process and its descendants are put in a process group.
	// bwrap's --new-session gives the sandbox its own session, while this
	// group remains the reliable kill target for the caller's cancellation.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("run_command: start %q: %w", argv[0], err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-wait:
		if err := execCtx.Err(); err != nil {
			return commandResult(stdout, stderr), err
		}
	case <-execCtx.Done():
		waitErr = terminateCommand(cmd, wait)
		if err := execCtx.Err(); err != nil {
			return commandResult(stdout, stderr), err
		}
	}
	result := commandResult(stdout, stderr)
	if waitErr != nil {
		return result, fmt.Errorf("run_command %q: %w", argv[0], waitErr)
	}
	return result, nil
}

func acquireCommand(ctx context.Context) error {
	select {
	case commandSlot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseCommand() { <-commandSlot }

func validateCommand(argv []string) error {
	if len(argv) == 0 || len(argv) > 32 || strings.TrimSpace(argv[0]) == "" {
		return errors.New("run_command: argv must contain 1–32 arguments")
	}
	if filepath.Base(argv[0]) != argv[0] {
		return errors.New("run_command: executable must be a bare allowlisted name")
	}
	for _, arg := range argv {
		if len(arg) > 4096 || arg == "" {
			return errors.New("run_command: argv elements must be non-empty and at most 4096 bytes")
		}
		if strings.ContainsAny(arg, "\x00\r\n;|&$`<>") {
			return fmt.Errorf("run_command: unsafe argv element %q", arg)
		}
		if pathEscapesWorkspace(arg) {
			return fmt.Errorf("run_command: path escapes are not allowed: %q", arg)
		}
	}
	switch argv[0] {
	case "go":
		if len(argv) < 2 || !map[string]bool{"test": true, "vet": true, "build": true, "list": true, "env": true, "version": true}[argv[1]] {
			return errors.New("run_command: allowed go subcommands are test, vet, build, list, env, version")
		}
		for _, arg := range argv[2:] {
			if arg == "-w" || strings.HasPrefix(arg, "-exec") || strings.HasPrefix(arg, "-toolexec") {
				return fmt.Errorf("run_command: unsafe go option %q", arg)
			}
		}
	case "git":
		if len(argv) < 2 || !map[string]bool{"status": true, "diff": true, "log": true, "show": true, "branch": true, "rev-parse": true, "ls-files": true, "grep": true}[argv[1]] {
			return errors.New("run_command: allowed git subcommands are status, diff, log, show, branch, rev-parse, ls-files, grep")
		}
		for _, arg := range argv[2:] {
			if arg == "-c" || strings.HasPrefix(arg, "-c=") || strings.HasPrefix(arg, "--config") || strings.HasPrefix(arg, "--git-dir") || strings.HasPrefix(arg, "--work-tree") {
				return fmt.Errorf("run_command: unsafe git option %q", arg)
			}
		}
	default:
		return fmt.Errorf("run_command: executable %q is not allowlisted", argv[0])
	}
	return nil
}

func pathEscapesWorkspace(arg string) bool {
	if filepath.IsAbs(arg) || arg == ".." || strings.HasPrefix(arg, "../") || strings.Contains(arg, "/../") || strings.HasSuffix(arg, "/..") {
		return true
	}
	// Options such as -modfile=../outside and --git-dir=/tmp must be rejected
	// even though the option's complete spelling is not itself an absolute
	// path. The allowlist does not need any host paths.
	for _, part := range strings.FieldsFunc(arg, func(r rune) bool { return r == '=' || r == ',' }) {
		if filepath.IsAbs(part) || part == ".." || strings.HasPrefix(part, "../") || strings.Contains(part, "/../") {
			return true
		}
	}
	return false
}

func bubblewrapBinary() (string, error) {
	for _, path := range []string{"/usr/bin/bwrap", "/bin/bwrap"} {
		if executableFile(path) {
			return path, nil
		}
	}
	return "", errors.New("run_command: bubblewrap is unavailable; install bwrap to enable sandboxed commands")
}

// allowlistedBinary resolves only fixed host paths. User PATH and model input
// never select an executable. go is taken from the toolchain that built this
// process so a go.mod requiring a newer version cannot trigger a network
// download from inside the sandbox.
func allowlistedBinary(name string) (hostPath, sandboxPath string, err error) {
	var candidates []string
	switch name {
	case "go":
		candidates = append(candidates, filepath.Join(runtime.GOROOT(), "bin", "go"), "/usr/local/go/bin/go", "/usr/bin/go", "/bin/go")
	case "git":
		candidates = []string{"/usr/bin/git", "/bin/git"}
	default:
		return "", "", fmt.Errorf("run_command: executable %q is not allowlisted", name)
	}
	for _, candidate := range candidates {
		if executableFile(candidate) {
			if name == "go" {
				return candidate, "/toolchain/bin/go", nil
			}
			return candidate, "/usr/bin/git", nil
		}
	}
	return "", "", fmt.Errorf("run_command: allowlisted executable %q is unavailable", name)
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

func osPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sandboxArgs(root, hostBinary, sandboxBinary string, argv []string) ([]string, error) {
	goroot := filepath.Dir(filepath.Dir(hostBinary))
	if !filepath.IsAbs(goroot) || !executableFile(filepath.Join(goroot, "bin", "go")) && filepath.Base(hostBinary) == "go" {
		return nil, errors.New("run_command: go toolchain root is unavailable")
	}
	args := []string{
		"--die-with-parent", "--new-session", "--unshare-net", "--clearenv",
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--dir", "/etc",
		"--dir", "/workspace",
		"--bind", root, "/workspace",
		"--tmpfs", "/tmp",
		"--dir", "/tmp/selftui-home",
		"--dir", "/tmp/selftui-tmp",
		"--dir", "/tmp/selftui-gocache",
		"--dir", "/tmp/selftui-gopath",
		"--proc", "/proc",
		"--dev", "/dev",
		"--setenv", "PATH", "/toolchain/bin:/usr/bin:/bin",
		"--setenv", "HOME", "/tmp/selftui-home",
		"--setenv", "TMPDIR", "/tmp/selftui-tmp",
		"--setenv", "GOCACHE", "/tmp/selftui-gocache",
		"--setenv", "GOPATH", "/tmp/selftui-gopath",
		"--setenv", "GOMODCACHE", "/modcache",
		"--setenv", "GOTOOLCHAIN", "local",
		"--setenv", "GOPROXY", "off",
		"--setenv", "GOSUMDB", "off",
		"--setenv", "CGO_ENABLED", "0",
		"--setenv", "LANG", "C.UTF-8",
		"--setenv", "TERM", "dumb",
		"--setenv", "PWD", "/workspace",
		"--setenv", "GIT_CONFIG_GLOBAL", "/dev/null",
		"--setenv", "GIT_CONFIG_SYSTEM", "/dev/null",
		"--setenv", "GIT_CONFIG_NOSYSTEM", "1",
		"--chdir", "/workspace",
	}
	if osPathExists("/lib64") {
		// Dynamically linked allowlisted binaries (notably git) need the
		// loader path, while some Linux layouts do not have /lib64.
		args = append(args, "--ro-bind", "/lib64", "/lib64")
	}
	if hostBinary != "" && filepath.Base(hostBinary) == "go" {
		args = append(args, "--ro-bind", goroot, "/toolchain")
		if modcache := hostModuleCache(goroot); modcache != "" {
			args = append(args, "--ro-bind", modcache, "/modcache")
		} else {
			args = append(args, "--dir", "/modcache")
		}
	}
	args = append(args, sandboxBinary)
	args = append(args, argv...)
	return args, nil
}

func hostModuleCache(goroot string) string {
	// GOTOOLCHAIN-managed Go installations live at
	// <GOMODCACHE>/golang.org/toolchain@...; derive that cache from the
	// trusted toolchain path. Do not bind the caller's GOMODCACHE environment
	// value directly: it could name an arbitrary host directory, defeating
	// the selective-mount boundary.
	if filepath.Base(filepath.Dir(goroot)) == "golang.org" {
		cache := filepath.Dir(filepath.Dir(goroot))
		if info, err := os.Stat(cache); err == nil && info.IsDir() {
			return cache
		}
	}
	// Standard Go installations use the conventional per-user module cache.
	// This binds only the known `~/go/pkg/mod` subtree, never the home
	// directory itself. A custom cache is intentionally unavailable rather
	// than being trusted as a mount source.
	if home, err := os.UserHomeDir(); err == nil {
		cache := filepath.Join(home, "go", "pkg", "mod")
		if info, err := os.Stat(cache); err == nil && info.IsDir() {
			return cache
		}
	}
	return ""
}

func commandEnv() []string {
	return []string{
		"PATH=/toolchain/bin:/usr/bin:/bin", "HOME=/tmp/selftui-home", "TMPDIR=/tmp/selftui-tmp",
		"GOCACHE=/tmp/selftui-gocache", "GOPATH=/tmp/selftui-gopath", "GOMODCACHE=/modcache",
		"GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "CGO_ENABLED=0",
		"LANG=C.UTF-8", "TERM=dumb", "PWD=/workspace",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
	}
}

// terminateCommand sends the same process-group termination sequence for
// cancellation and timeout. It waits for cmd.Wait's single owner, escalating
// to SIGKILL after the grace interval so descendants cannot survive a turn.
func terminateCommand(cmd *exec.Cmd, wait <-chan error) error {
	if cmd.Process == nil {
		return <-wait
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-wait:
		return err
	case <-timer.C:
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return <-wait
	}
}

type commandOutput struct {
	mu        sync.Mutex
	stream    string
	emit      func(stream, text string)
	body      strings.Builder
	truncated bool
}

func (o *commandOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	remaining := maxCommandOutput - o.body.Len()
	accepted := p
	if remaining <= 0 {
		o.truncated = true
		return len(p), nil
	}
	if len(accepted) > remaining {
		accepted = accepted[:remaining]
		o.truncated = true
	}
	o.body.Write(accepted)
	if o.emit != nil && len(accepted) > 0 {
		o.emit(o.stream, string(accepted))
	}
	return len(p), nil
}

func (o *commandOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	text := o.body.String()
	if o.truncated {
		text += "\n[output truncated]"
	}
	return text
}

func commandResult(stdout, stderr *commandOutput) string {
	var out strings.Builder
	if text := stdout.String(); text != "" {
		out.WriteString("stdout:\n")
		out.WriteString(text)
	}
	if text := stderr.String(); text != "" {
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString("stderr:\n")
		out.WriteString(text)
	}
	return boundedResult(out.String())
}
