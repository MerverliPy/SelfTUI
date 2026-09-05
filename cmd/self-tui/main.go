// Command self-tui boots the SelfTUI TUI: flags → config load → file
// logger → cancellable Bubble Tea program.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/charmbracelet/log"

	"selftui/internal/config"
	"selftui/internal/ollama"
	"selftui/internal/ui"
)

// Version identifies this build; it is printed by `selftui -version` and
// logged at startup so release/smoke evidence is attributable (M6). It is a
// var so a build can stamp a concrete version (default "dev").
var Version = "dev"

// printVersion writes the `selftui -version` output. Kept as one function so
// the formatting contract ("selftui <Version>") is testable against a
// temporarily-set Version (see main_test.go).
func printVersion(w io.Writer) {
	fmt.Fprintf(w, "selftui %s\n", Version)
}

// configPathOverride maps the parsed -config flag pointer onto the
// config.Overrides.ConfigPath field. flag.String returns a non-nil *string
// even when the flag is omitted (value ""); forwarding that pointer made
// config.Load treat "" as the config file path, silently skip the default XDG
// file, and leave ConfigPath() empty (audit H-01). An empty flag value
// therefore maps to nil — which Load interprets as "use
// $XDG_CONFIG_HOME/selftui/config.toml" — while a non-empty explicit path
// stays a non-nil override so -config keeps its documented precedence. Kept
// pure so the entrypoint boundary is deterministically testable without
// re-parsing the process-global flag set that run() owns.
func configPathOverride(flagConfig *string) *string {
	if flagConfig != nil && *flagConfig == "" {
		return nil
	}
	return flagConfig
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "selftui:", err)
		os.Exit(1)
	}
}

func run() error {
	// --- flags (highest config priority) ---
	flagConfig := flag.String("config", "", "config file path (default: $XDG_CONFIG_HOME/selftui/config.toml)")
	flagHost := flag.String("host", "", "Ollama base URL (overrides env + config file)")
	flagAuthToken := flag.String("auth-token", "", "auth token (compatibility only — prefer SELFTUI_AUTH_TOKEN or the 0600 config file; argv secrets appear in process listings and shell history)")
	flagTheme := flag.String("theme", "", "theme: dark (default) or light")
	flagDefaultModel := flag.String("default-model", "", "default model for new sessions")
	flagWorkspaceRoot := flag.String("workspace-root", "", "agent workspace root")
	flagTemperature := flag.String("temperature", "", "agent temperature")
	flagTopP := flag.String("top-p", "", "agent top_p")
	flagNumCtx := flag.String("num-ctx", "", "agent num_ctx")
	flagMaxToolIterations := flag.String("max-tool-iterations", "", "max tool iterations")
	flagSystemPrompt := flag.String("system-prompt", "", "agent system prompt")
	flagVerbose := flag.Bool("verbose", false, "debug-level logging")
	flagVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *flagVersion {
		printVersion(os.Stdout)
		return nil
	}

	ov := config.Overrides{ConfigPath: configPathOverride(flagConfig)}
	if *flagHost != "" {
		ov.Host = flagHost
	}
	if *flagAuthToken != "" {
		ov.AuthToken = flagAuthToken
	}
	if *flagTheme != "" {
		ov.Theme = flagTheme
	}
	if *flagDefaultModel != "" {
		ov.DefaultModel = flagDefaultModel
	}
	if *flagWorkspaceRoot != "" {
		ov.WorkspaceRoot = flagWorkspaceRoot
	}
	if *flagTemperature != "" {
		tv, err := strconv.ParseFloat(*flagTemperature, 64)
		if err != nil {
			return fmt.Errorf("parse flag temperature: %w", err)
		}
		ov.Temperature = &tv
	}
	if *flagTopP != "" {
		tv, err := strconv.ParseFloat(*flagTopP, 64)
		if err != nil {
			return fmt.Errorf("parse flag top-p: %w", err)
		}
		ov.TopP = &tv
	}
	if *flagNumCtx != "" {
		nv, err := strconv.Atoi(*flagNumCtx)
		if err != nil {
			return fmt.Errorf("parse flag num-ctx: %w", err)
		}
		ov.NumCtx = &nv
	}
	if *flagMaxToolIterations != "" {
		nv, err := strconv.Atoi(*flagMaxToolIterations)
		if err != nil {
			return fmt.Errorf("parse flag max-tool-iterations: %w", err)
		}
		ov.MaxToolIterations = &nv
	}
	if *flagSystemPrompt != "" {
		ov.SystemPrompt = flagSystemPrompt
	}

	cfg, err := config.Load(ov)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- file logger (stderr stays clean for the alt-screen over SSH) ---
	logPath, err := xdg.StateFile(filepath.Join("selftui", "log.txt"))
	if err != nil {
		return fmt.Errorf("resolve log path: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer logFile.Close()
	rootLog := log.NewWithOptions(logFile, log.Options{Prefix: "selftui", ReportCaller: true})
	if *flagVerbose {
		rootLog.SetLevel(log.DebugLevel)
	}
	rootLog.Info("starting", "version", Version, "host", cfg.Host, "theme", cfg.Theme, "config", cfg.ConfigPath())

	// --- chat-session transcript dir (owner feature): every committed turn
	// is appended to a per-process file here, so a conversation survives the
	// process. SELFTUI_NO_SESSION=1 disables; SELFTUI_SESSION_DIR overrides.
	rootLog.Info("session dir", "path", sessionDirForRun())

	// --- cancellation plumbing: SIGINT/SIGTERM cancel a root context that
	// the tea runtime (tea.WithContext below) and every Models/Agent
	// operation — list/show/delete fetches, chat and pull streams — derive
	// from, so a Ctrl+C aborts in-flight background work instead of
	// stranding it. Constructed before the App so NewWithContext can bind it.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- bootstrap the program ---
	client := ollama.New(cfg.Host, cfg.AuthToken)
	m := ui.NewWithContext(ctx, &cfg, ui.NewStyles(cfg.Theme), client)
	m = m.WithSessionDir(sessionDirForRun(), cfg.Host)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	rootLog.Info("program running")
	final, err := p.Run()
	// Stop intercepting SIGINT/SIGTERM and cancel the root context BEFORE the
	// recorder flush below: a recorder worker stuck on a wedged sink must not
	// leave the process unable to die on a second Ctrl+C (NotifyContext would
	// keep swallowing it). cancel() also releases in-flight chat/pull producers
	// so the shutdown wait is not competing with live work (P1-1).
	cancel()
	// Normal-shutdown lifecycle: flush every committed turn to the transcript
	// and stop the recorder worker before the process returns, so the last
	// turns survive and no goroutine is stranded (M-04). Best-effort on the
	// error paths too — a killed program still wants its transcript flushed.
	// The close is time-bounded so a stalled filesystem cannot hang the
	// process (P1-1).
	if cerr := closeSessionRecorder(final); cerr != nil {
		rootLog.Warn("session recorder close", "err", cerr)
	}
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	rootLog.Info("shutdown clean")
	return nil
}

// sessionCloseTimeout bounds how long the entrypoint waits for the transcript
// recorder to flush at shutdown (P1-1). A recorder worker stuck on a wedged
// filesystem sink must not hang the process forever; once the budget is spent
// the flush is abandoned (the file may be short its last turn) and the
// process exits, which is the actual escape hatch.
const sessionCloseTimeout = 3 * time.Second

// closeSessionRecorder closes the chat-transcript recorder on the final tea
// model when it exposes one (ui.App), within the shutdown budget. Kept as a
// small interface-assertion so the entrypoint stays decoupled from the
// concrete model and a nil or foreign final model is a harmless no-op.
func closeSessionRecorder(m any) error {
	return closeSessionRecorderWithin(m, sessionCloseTimeout)
}

// closeSessionRecorderWithin runs the recorder shutdown in a goroutine so a
// wedged sink can never block the process past the budget. The goroutine is
// abandoned on timeout — the process is exiting, so nothing is stranded that
// matters, and the recorder's own failure discipline already surfaced.
func closeSessionRecorderWithin(m any, budget time.Duration) error {
	c, ok := m.(interface{ CloseSession() error })
	if !ok {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- c.CloseSession() }()
	select {
	case err := <-done:
		return err
	case <-time.After(budget):
		return fmt.Errorf("session recorder close timed out after %s", budget)
	}
}

// sessionDirForRun resolves the chat-transcript directory: SELFTUI_NO_SESSION=1
// disables recording, SELFTUI_SESSION_DIR overrides the default (the XDG state
// dir, next to log.txt). An empty result leaves chat in-memory only.
func sessionDirForRun() string {
	if os.Getenv("SELFTUI_NO_SESSION") == "1" {
		return ""
	}
	if v := os.Getenv("SELFTUI_SESSION_DIR"); v != "" {
		return v
	}
	if xdg.StateHome == "" {
		return ""
	}
	return filepath.Join(xdg.StateHome, "selftui", "sessions")
}
