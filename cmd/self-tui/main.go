// Command self-tui boots the SelfTUI TUI: flags → config load → file
// logger → cancellable Bubble Tea program.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/charmbracelet/log"

	"selftui/internal/config"
	"selftui/internal/ollama"
	"selftui/internal/ui"
)

// Version identifies this build; it is logged at startup and printed by
// selftui -version so release/smoke evidence is attributable (M6).
const Version = "0.6.0-m6"

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
	flagAuthToken := flag.String("auth-token", "", "auth token (overrides env + config file)")
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
		fmt.Printf("selftui %s\n", Version)
		return nil
	}

	ov := config.Overrides{ConfigPath: flagConfig}
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
	// the program and (from M1+) background jobs share ---
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- bootstrap the program ---
	client := ollama.New(cfg.Host, cfg.AuthToken)
	m := ui.New(&cfg, ui.NewStyles(cfg.Theme), client)
	m = m.WithSessionDir(sessionDirForRun(), cfg.Host)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	rootLog.Info("program running")
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	cancel()
	rootLog.Info("shutdown clean")
	return nil
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
