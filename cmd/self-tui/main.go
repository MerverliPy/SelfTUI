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
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/charmbracelet/log"

	"selftui/internal/config"
	"selftui/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "selftui:", err)
		os.Exit(1)
	}
}

func run() error {
	// --- flags (highest config priority) ---
	flagHost := flag.String("host", "", "Ollama base URL (overrides env + config file)")
	flagTheme := flag.String("theme", "", "theme: dark (default) or light")
	flagConfig := flag.String("config", "", "config file path (default: $XDG_CONFIG_HOME/selftui/config.toml)")
	flagVerbose := flag.Bool("verbose", false, "debug-level logging")
	flag.Parse()

	ov := config.Overrides{ConfigPath: flagConfig}
	if *flagHost != "" {
		ov.Host = flagHost
	}
	if *flagTheme != "" {
		ov.Theme = flagTheme
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
	rootLog.Info("starting", "host", cfg.Host, "theme", cfg.Theme, "config", cfg.ConfigPath())

	// --- cancellation plumbing: SIGINT/SIGTERM cancel a root context that
	// the program and (from M1+) background jobs share ---
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- bootstrap the program ---
	m := ui.New(&cfg, ui.NewStyles(cfg.Theme))
	p := tea.NewProgram(m, tea.WithContext(ctx))
	rootLog.Info("program running")
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	cancel()
	rootLog.Info("shutdown clean")
	return nil
}
