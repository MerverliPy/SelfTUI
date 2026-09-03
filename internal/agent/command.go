package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

var commandMu sync.Mutex

// RunCommand executes the deliberately small M3b allowlist. It accepts argv,
// never a shell string; callers must obtain user approval before calling it.
// A process group, scrubbed environment, bounded output, and context-backed
// timeout are guardrails only—not an OS sandbox.
func RunCommand(ctx context.Context, root string, argv []string, timeoutSeconds int, emit func(stream, text string)) (string, error) {
	rootReal, err := canonicalRoot(root)
	if err != nil {
		return "", err
	}
	if err := validateCommand(argv); err != nil {
		return "", err
	}
	commandMu.Lock()
	defer commandMu.Unlock()

	timeout := defaultCommandTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	if timeout > maxCommandTimeout {
		return "", fmt.Errorf("run_command: timeout exceeds %s", maxCommandTimeout)
	}
	if err := os.MkdirAll(filepath.Join(rootReal, ".selftui-home"), 0o700); err != nil {
		return "", fmt.Errorf("run_command: create jailed home: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(rootReal, ".selftui-tmp"), 0o700); err != nil {
		return "", fmt.Errorf("run_command: create jailed temp: %w", err)
	}

	binary, err := allowlistedBinary(argv[0])
	if err != nil {
		return "", err
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout := &commandOutput{stream: "stdout", emit: emit}
	stderr := &commandOutput{stream: "stderr", emit: emit}
	cmd := exec.Command(binary, argv[1:]...)
	cmd.Dir = rootReal
	cmd.Env = commandEnv(rootReal)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("run_command: start %q: %w", argv[0], err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-wait:
	case <-execCtx.Done():
		terminateCommand(cmd)
		waitErr = <-wait
		_ = waitErr
		return commandResult(stdout, stderr), execCtx.Err()
	}
	result := commandResult(stdout, stderr)
	if waitErr != nil {
		return result, fmt.Errorf("run_command %q: %w", argv[0], waitErr)
	}
	return result, nil
}

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
		if filepath.IsAbs(arg) || arg == ".." || strings.HasPrefix(arg, ".."+string(filepath.Separator)) {
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

func allowlistedBinary(name string) (string, error) {
	for _, dir := range strings.Split("/usr/local/bin:/usr/bin:/bin", ":") {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("run_command: allowlisted executable %q is unavailable", name)
}

func commandEnv(root string) []string {
	home := filepath.Join(root, ".selftui-home")
	tmp := filepath.Join(root, ".selftui-tmp")
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "TMPDIR=" + tmp,
		"GOCACHE=" + filepath.Join(root, ".selftui-gocache"), "GOPATH=" + filepath.Join(root, ".selftui-gopath"),
		"LANG=C.UTF-8", "TERM=dumb",
	}
}

func terminateCommand(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	case <-processExited(cmd):
	}
}

// processExited is only used as a grace-period timer hint. Wait remains the
// single owner of cmd.Wait, avoiding concurrent Wait calls.
func processExited(cmd *exec.Cmd) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for {
			if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
				close(ch)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	return ch
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
