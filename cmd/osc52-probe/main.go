// Command osc52-probe is the P1 spike instrument: it proves or disproves
// terminal OSC 52 clipboard capture — does a terminal that receives
//
//	ESC ] 52 ; c ; <base64 payload> BEL
//
// write that payload into the system clipboard? This is the unproven link in
// copy-to-phone: SelfTUI runs on a host over SSH, so "copy" can only mean an
// OSC 52 sequence the *client* terminal must interpret. Pre-registered under
// LEDGER 2026-09-09 grill decision #2 (conclave item P1):
//
//	spike passes → copy-to-phone ships as an opt-in feature (config key
//	               default-OFF + Settings toggle; palette action
//	               "Copy last reply" — grill decisions #3/#4);
//	spike fails  → negative result recorded, item closed, P2 promoted.
//
// Usage:
//
//	bin/osc52-probe              emit a generated payload, paste to verify
//	bin/osc52-probe -payload X   verify with a fixed payload instead
//	bin/osc52-probe -st          use the ST (ESC \) terminator instead of BEL
//
// The probe prints its environment (TERM, SSH, tmux) first — tmux matters
// because tmux intercepts OSC 52 unless `set-clipboard` allows it through —
// then emits the sequence, then reads one pasted line from stdin and compares
// it to the payload. Paste artifacts (bracketed-paste markers, trailing CR)
// are stripped before the comparison so a captured paste is never scored as
// a failure.
//
// Verdicts (also the exit code and the evidence-log line):
//
//	PASS          paste == payload (exit 0)
//	FAIL          paste != payload (exit 1)
//	INCONCLUSIVE  nothing pasted (exit 2)
//
// Every run appends one evidence line to $XDG_STATE_HOME/selftui/
// osc52-probe.txt (same durable-evidence pattern as size-probe); run this in
// each target client — Moshi first, it is the gate — and record the verdicts
// per docs/ and the LEDGER handoff.
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
)

const payloadPrefix = "SelfTUI-OSC52-"

// generatePayload returns the paste-compare token: an ASCII-safe prefix plus
// 8 hex chars of crypto/rand, unique per run so a stale clipboard can never
// score a false PASS.
func generatePayload() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return payloadPrefix + hex.EncodeToString(b[:]), nil
}

// osc52Sequence builds the exact byte sequence under test: OSC 52 targeting
// the system clipboard (c) carrying the base64-encoded payload, terminated
// with BEL — or with ST when st is set.
func osc52Sequence(payload string, st bool) string {
	enc := base64.StdEncoding.EncodeToString([]byte(payload))
	term := "\a"
	if st {
		term = "\x1b\\"
	}
	return "\x1b]52;c;" + enc + term
}

// sanitizePaste removes the two artifacts a terminal may wrap a paste in —
// bracketed-paste markers and the CR that iOS/SSH clients append — plus any
// surrounding whitespace, so only real content differences produce FAIL.
func sanitizePaste(s string) string {
	s = strings.ReplaceAll(s, "\x1b[200~", "")
	s = strings.ReplaceAll(s, "\x1b[201~", "")
	return strings.TrimSpace(s)
}

// verdict compares a sanitized paste to the payload.
func verdict(paste, payload string) string {
	switch {
	case paste == "":
		return "INCONCLUSIVE"
	case paste == payload:
		return "PASS"
	default:
		return "FAIL"
	}
}

func main() {
	var (
		payloadFlag string
		st          bool
		logFlag     string
	)
	flag.StringVar(&payloadFlag, "payload", "", "verify with this fixed payload (default: generated per run)")
	flag.BoolVar(&st, "st", false, "terminate the sequence with ST (ESC \\) instead of BEL")
	flag.StringVar(&logFlag, "log", "", "evidence log file (default: $XDG_STATE_HOME/selftui/osc52-probe.txt)")
	flag.Parse()

	payload := payloadFlag
	if payload == "" {
		p, err := generatePayload()
		if err != nil {
			fmt.Fprintf(os.Stderr, "osc52-probe: generating payload: %v\n", err)
			os.Exit(1)
		}
		payload = p
	}

	env := map[string]string{
		"TERM":         os.Getenv("TERM"),
		"TERM_PROGRAM": os.Getenv("TERM_PROGRAM"),
	}
	inSSH := os.Getenv("SSH_CONNECTION") != ""
	inTMUX := os.Getenv("TMUX") != ""

	fmt.Println("OSC 52 capture spike — LEDGER 2026-09-09 grill decision #2 (conclave P1)")
	fmt.Println("-----------------------------------------------------------------------")
	fmt.Printf("  TERM          %s\n", orUnset(env["TERM"]))
	fmt.Printf("  TERM_PROGRAM  %s\n", orUnset(env["TERM_PROGRAM"]))
	fmt.Printf("  SSH           %t\n", inSSH)
	fmt.Printf("  tmux          %t%s\n", inTMUX, tmuxHint(inTMUX))
	fmt.Println()
	fmt.Printf("Payload: %s\n", payload)
	if _, err := fmt.Fprint(os.Stdout, osc52Sequence(payload, st)); err != nil {
		fmt.Fprintf(os.Stderr, "osc52-probe: emitting sequence: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Emitted ESC]52;c;<%d base64 bytes>%s — the clipboard should now hold the payload.\n",
		len(base64.StdEncoding.EncodeToString([]byte(payload))),
		map[bool]string{false: " + BEL", true: " + ST"}[st])
	fmt.Println()
	fmt.Println("Paste below now (long-press → Paste in Moshi), then press Enter.")
	fmt.Print("paste> ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintf(os.Stderr, "osc52-probe: reading paste: %v\n", err)
		os.Exit(2)
	}

	paste := sanitizePaste(line)
	v := verdict(paste, payload)
	fmt.Println()
	switch v {
	case "PASS":
		fmt.Printf("VERDICT: PASS — paste matched the payload; this terminal captured OSC 52.\n")
	case "FAIL":
		fmt.Printf("VERDICT: FAIL — paste differed from the payload.\n  want: %s\n  got:  %s\n", payload, paste)
	case "INCONCLUSIVE":
		fmt.Printf("VERDICT: INCONCLUSIVE — nothing pasted; no evidence either way. Re-run and paste.\n")
	}

	logVerdict(logFlag, v, payload, env, inSSH, inTMUX, st)
	os.Exit(map[string]int{"PASS": 0, "FAIL": 1, "INCONCLUSIVE": 2}[v])
}

func orUnset(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

// tmuxHint names the one configuration knob known to block capture through
// tmux, so a FAIL inside tmux is actionable instead of terminal.
func tmuxHint(inTMUX bool) string {
	if !inTMUX {
		return ""
	}
	return "  (tmux may intercept OSC 52 — `set -g set-clipboard on` or run outside tmux)"
}

// logVerdict appends the run's evidence line, best effort: a logging failure
// must not corrupt the verdict the operator already read.
func logVerdict(logFlag, v, payload string, env map[string]string, inSSH, inTMUX, st bool) {
	path := logFlag
	if path == "" {
		p, err := xdg.StateFile(filepath.Join("selftui", "osc52-probe.txt"))
		if err != nil {
			return
		}
		path = p
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s verdict=%s payload=%s term=%s ssh=%t tmux=%t st=%t\n",
		time.Now().UTC().Format(time.RFC3339), v, payload,
		env["TERM"], inSSH, inTMUX, st)
}
