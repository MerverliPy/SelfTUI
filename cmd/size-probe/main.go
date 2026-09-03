// Command size-probe measures the real terminal geometry the app receives —
// width x height, resize events, key delivery, and color depth — on any
// terminal, including SSH clients (Blink/Termius) from a phone.
//
// Usage:
//
//	bin/size-probe                 interactive probe (alt screen, Bubble Tea);
//	                               every event is also appended to -log
//	bin/size-probe -mode raw       pure CSV reporter (TIOCGWINSZ + SIGWINCH,
//	                               no Bubble Tea): one line per event:
//	                               ts kind width height breakpoint extra
//	bin/size-probe -mode raw -once print the first negotiated size, then exit
//	bin/size-probe -dur 20         auto-quit after N seconds (both modes)
//	bin/size-probe -session live-1 tag this session in the event log (default:
//	                               auto "pid-<pid>"; each fresh launch writes a
//	                               session header, so reconnect runs are
//	                               attributable across SSH re-connects)
//
// Checkpoints: press c in tui mode, or send SIGUSR1 in raw mode, to append a
// checkpoint event line — a human marker ("about to drop", "just
// reconnected") that makes probe.txt read like a timeline.
//
// The two modes are deliberately independent reporters of the same pty:
// tui mode receives Bubble Tea's WindowSizeMsg, raw mode reads TIOCGWINSZ
// directly — they must agree, which is itself a measurement check.
//
// -log defaults to $XDG_STATE_HOME/selftui/probe.txt and doubles as the
// durable evidence file: every event lands there in both modes.
//
// M0a gate evidence: run this over the target SSH client(s) and record the
// output; see docs/m0a-gate-evidence.md. M6 reconnect evidence (fresh SSH
// re-connect semantics) uses the session tags + checkpoints; see
// docs/reconnect.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/muesli/termenv"
	"golang.org/x/sys/unix"
)

// ---- shared event line -----------------------------------------------------

var logPath string

// session names this probe launch in the event log. A fresh SSH session is a
// fresh process, so the default (pid) already separates reconnect runs; an
// explicit -session makes a live run legible (e.g. -session m6-live-1a).
var sessionName string

// sessionHeader is written once per process, before the first event line, so
// probe.txt is a sequence of attributable session blocks.
func sessionHeader(termEnv, colorEnv, profile string) string {
	return fmt.Sprintf("# size-probe start session=%s pid=%d term=%s colorterm=%s profile=%s",
		sessionName, os.Getpid(), termEnv, colorEnv, profile)
}

// line renders one CSV-ish measurement line, shared by both modes.
func line(kind string, w, h int, extra string) string {
	return fmt.Sprintf("%s\t%s\t%d\t%d\t%s\t%s",
		time.Now().Format("2006-01-02T15:04:05"), kind, w, h, breakpointName(w), extra)
}

// breakpointName mirrors ui.BreakpointFor without importing the app package.
func breakpointName(w int) string {
	switch {
	case w <= 79:
		return "compact"
	case w <= 119:
		return "medium"
	default:
		return "wide"
	}
}

// ---- raw mode: standalone TIOCGWINSZ reporter (no Bubble Tea) --------------

// rawTTY returns an fd for size queries: the controlling terminal when
// available, else stdin. Background jobs get stdin redirected to /dev/null
// (POSIX), so ioctl'ing stdin alone would fail in the harness's `&`-started
// probe; /dev/tty is the controlling terminal either way.
func rawTTY() (fd int, closeFn func()) {
	f, err := os.Open("/dev/tty")
	if err == nil {
		return int(f.Fd()), func() { _ = f.Close() }
	}
	return int(os.Stdin.Fd()), func() {}
}

// runRaw prints the initial size, then every resize, as pure CSV on stdout
// (and appends everything to the log). Works over plain SSH pipes too.
func runRaw(once bool, dur time.Duration, logFile *os.File) error {
	log := func(s string) {
		if logFile != nil {
			fmt.Fprintln(logFile, s) // nolint:errcheck — best effort
		}
		fmt.Println(s)
	}

	fd, closeTTY := rawTTY()
	defer closeTTY()

	size, err := sizeAt(fd)
	if err != nil {
		return fmt.Errorf("no terminal size available (raw mode needs a tty): %w", err)
	}
	log(line("initial", size.w, size.h, ""))
	if once {
		return nil
	}

	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	defer signal.Stop(sigwinch)

	// SIGUSR1 = human checkpoint marker ("about to drop", "reconnected").
	sigusr1 := make(chan os.Signal, 1)
	signal.Notify(sigusr1, syscall.SIGUSR1)
	defer signal.Stop(sigusr1)

	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var deadline <-chan time.Time
	if dur > 0 {
		d := time.NewTimer(dur)
		defer d.Stop()
		deadline = d.C
	}

	prev := size
	checkpoint := false
	for {
		select {
		case <-sigwinch:
		case <-sigusr1:
			checkpoint = true
		case <-tick.C:
		case <-deadline:
			return nil
		}
		cur, err := sizeAt(fd)
		if err != nil {
			return err
		}
		if checkpoint {
			log(line("checkpoint", cur.w, cur.h, "SIGUSR1"))
			checkpoint = false
		}
		if cur != prev {
			log(line("resize", cur.w, cur.h, ""))
			prev = cur
		}
	}
}

type size struct{ w, h int }

func sizeAt(fd int) (size, error) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return size{}, err
	}
	return size{w: int(ws.Col), h: int(ws.Row)}, nil
}

// ---- tui mode: Bubble Tea WindowSizeMsg viewer -----------------------------

// probe models the interactive probe.
type probe struct {
	w, h     int // last known geometry (0 = none yet)
	logFile  *os.File
	events   []string // ring buffer for the screen
	keys     int
	termEnv  string
	colorEnv string
	profile  string
	darkBg   bool
	dur      time.Duration
}

type timedOut struct{}

func (m probe) Init() tea.Cmd {
	if m.dur > 0 {
		return tea.Tick(m.dur, func(time.Time) tea.Msg { return timedOut{} })
	}
	return nil
}

func (m *probe) note(kind string, extra string) {
	l := line(kind, m.w, m.h, extra)
	if m.logFile != nil {
		fmt.Fprintln(m.logFile, l) // nolint:errcheck — best effort
	}
	m.events = append(m.events, l)
	if len(m.events) > 9 {
		m.events = m.events[len(m.events)-9:]
	}
}

func (m probe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		first := m.w == 0
		m.w, m.h = msg.Width, msg.Height
		// A 0x0 WindowSizeMsg means the terminal hasn't negotiated a size
		// yet (observed under script without a parent tty). Record only real
		// geometries; the app must tolerate the zero-size frame regardless.
		if m.w == 0 || m.h == 0 {
			break
		}
		kind := "resize"
		if first {
			kind = "initial"
		}
		m.note(kind, "")

	case tea.KeyPressMsg:
		m.keys++
		k := msg.Key()
		var mods []string
		if k.Mod.Contains(tea.ModCtrl) {
			mods = append(mods, "ctrl")
		}
		if k.Mod.Contains(tea.ModShift) {
			mods = append(mods, "shift")
		}
		if k.Mod.Contains(tea.ModAlt) {
			mods = append(mods, "alt")
		}
		extra := fmt.Sprintf("code=%d text=%q mod=%s",
			k.Code, k.Text, strings.Join(mods, "+"))
		m.note("key", extra)
		if k.Text == "q" {
			return m, quit()
		}
		if k.Text == "c" {
			// human checkpoint marker for reconnect runs: press c just
			// before dropping the SSH session and again after reconnecting.
			m.note("checkpoint", "user marker")
			return m, nil
		}

	case tea.KeyMsg:
		// other key messages (release etc.): display only
		m.keys++
		m.note("key", fmt.Sprintf("%T", msg))

	case timedOut:
		return m, quit()
	}
	return m, nil
}

func (m probe) View() tea.View {
	lines := []string{
		"size-probe — M0a measurement (q / ctrl+c quits, c = checkpoint)",
		"",
		fmt.Sprintf("  session    %s", sessionName),
		fmt.Sprintf("  geometry   %dx%d  → %s", m.w, m.h, breakpointName(m.w)),
		fmt.Sprintf("  keys seen  %d", m.keys),
		fmt.Sprintf("  color      TERM=%s COLORTERM=%s profile=%s darkBg=%v",
			m.termEnv, m.colorEnv, m.profile, m.darkBg),
		fmt.Sprintf("  log        %s", logPath),
		"",
		"  resize the window / rotate the phone to record sizes;",
		"  press keys (arrows, tab, ctrl+c, letters) to test delivery:",
		"",
	}
	for _, e := range m.events {
		lines = append(lines, "  "+e)
	}
	// pad the event window so the layout doesn't jump
	for i := len(m.events); i < 8; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, "", "  live session appended to the log file above.")
	return tea.View{Content: strings.Join(lines, "\n"), AltScreen: true}
}

func quit() tea.Cmd {
	return func() tea.Msg { return tea.Quit() }
}

// ---- entrypoint ------------------------------------------------------------

func main() {
	mode := flag.String("mode", "tui", "tui (alt screen) or raw (CSV lines)")
	once := flag.Bool("once", false, "raw: print the first size, then exit")
	dur := flag.Int("dur", 0, "auto-quit after N seconds (0 = run until quit)")
	session := flag.String("session", "", "session tag for the event log (default: pid-<pid>)")
	flag.StringVar(&logPath, "log", "", "event log file (default: $XDG_STATE_HOME/selftui/probe.txt)")
	flag.Parse()

	if *session != "" {
		sessionName = *session
	} else {
		sessionName = fmt.Sprintf("pid-%d", os.Getpid())
	}

	if *mode != "tui" && *mode != "raw" {
		fmt.Fprintln(os.Stderr, "size-probe: -mode must be tui or raw")
		os.Exit(2)
	}

	if logPath == "" {
		var err error
		logPath, err = xdg.StateFile(filepath.Join("selftui", "probe.txt"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "size-probe:", err)
			os.Exit(1)
		}
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "size-probe: open log:", err)
		os.Exit(1)
	}
	defer logFile.Close()

	termEnv, colorEnv := os.Getenv("TERM"), os.Getenv("COLORTERM")
	profile := termenv.ColorProfile().Name()
	darkBg := termenv.HasDarkBackground()
	fmt.Fprintln(logFile, sessionHeader(termEnv, colorEnv, profile))

	if *mode == "raw" {
		if err := runRaw(*once, time.Duration(*dur)*time.Second, logFile); err != nil {
			fmt.Fprintln(os.Stderr, "size-probe:", err)
			os.Exit(1)
		}
		return
	}

	m := probe{
		logFile:  logFile,
		termEnv:  termEnv,
		colorEnv: colorEnv,
		profile:  profile,
		darkBg:   darkBg,
		dur:      time.Duration(*dur) * time.Second,
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "size-probe:", err)
		os.Exit(1)
	}
}
