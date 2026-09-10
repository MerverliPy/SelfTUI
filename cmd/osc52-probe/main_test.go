package main

import (
	"strings"
	"testing"
)

func TestGeneratePayload(t *testing.T) {
	p, err := generatePayload()
	if err != nil {
		t.Fatalf("generatePayload: %v", err)
	}
	if !strings.HasPrefix(p, payloadPrefix) {
		t.Errorf("payload %q missing prefix %q", p, payloadPrefix)
	}
	suffix := strings.TrimPrefix(p, payloadPrefix)
	if len(suffix) != 8 {
		t.Errorf("payload suffix %q: want 8 hex chars, got %d", suffix, len(suffix))
	}
	for _, r := range suffix {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("payload suffix %q: non-hex rune %q", suffix, r)
		}
	}
	// Uniqueness is the false-PASS guard: a stale clipboard must not match a
	// fresh run's payload.
	q, err := generatePayload()
	if err != nil {
		t.Fatalf("generatePayload (second): %v", err)
	}
	if p == q {
		t.Errorf("two generated payloads identical: %q", p)
	}
}

func TestOSC52Sequence(t *testing.T) {
	// base64("selftui") = c2VsZnR1aQ==
	wantBEL := "\x1b]52;c;c2VsZnR1aQ==\a"
	if got := osc52Sequence("selftui", false); got != wantBEL {
		t.Errorf("BEL sequence:\n got %q\nwant %q", got, wantBEL)
	}
	wantST := "\x1b]52;c;c2VsZnR1aQ==\x1b\\"
	if got := osc52Sequence("selftui", true); got != wantST {
		t.Errorf("ST sequence:\n got %q\nwant %q", got, wantST)
	}
	if s := osc52Sequence("selftui", false); !strings.HasPrefix(s, "\x1b]52;c;") {
		t.Errorf("sequence %q: missing OSC 52 system-clipboard header", s)
	}
}

func TestSanitizePaste(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "SelfTUI-OSC52-deadbeef", "SelfTUI-OSC52-deadbeef"},
		{"trailing CR (iOS/SSH)", "SelfTUI-OSC52-deadbeef\r", "SelfTUI-OSC52-deadbeef"},
		{"bracketed paste", "\x1b[200~SelfTUI-OSC52-deadbeef\x1b[201~", "SelfTUI-OSC52-deadbeef"},
		{"bracketed + CR", "\x1b[200~SelfTUI-OSC52-deadbeef\x1b[201~\r", "SelfTUI-OSC52-deadbeef"},
		{"surrounding whitespace", "  SelfTUI-OSC52-deadbeef \n", "SelfTUI-OSC52-deadbeef"},
		{"empty", "", ""},
		{"CR only", "\r", ""},
	}
	for _, tc := range cases {
		if got := sanitizePaste(tc.in); got != tc.want {
			t.Errorf("%s: sanitizePaste(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestVerdict(t *testing.T) {
	cases := []struct {
		paste, payload, want string
	}{
		{"SelfTUI-OSC52-deadbeef", "SelfTUI-OSC52-deadbeef", "PASS"},
		{"SelfTUI-OSC52-cafebabe", "SelfTUI-OSC52-deadbeef", "FAIL"},
		{"stale clipboard content", "SelfTUI-OSC52-deadbeef", "FAIL"},
		{"", "SelfTUI-OSC52-deadbeef", "INCONCLUSIVE"},
	}
	for _, tc := range cases {
		if got := verdict(tc.paste, tc.payload); got != tc.want {
			t.Errorf("verdict(%q, %q) = %q, want %q", tc.paste, tc.payload, got, tc.want)
		}
	}
}
