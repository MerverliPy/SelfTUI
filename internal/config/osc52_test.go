package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOSC52CopyDefaultsFalse pins the spike-gated opt-in: no OSC 52 emission
// unless the user explicitly turns it on (grill decision #3, LEDGER
// 2026-09-09). A file that never mentions the key must also leave it off.
func TestOSC52CopyDefaultsFalse(t *testing.T) {
	blankEnv(t)
	if c := Default(); c.OSC52Copy {
		t.Error("Default().OSC52Copy = true, want false (copy-to-phone is opt-in)")
	}
	p := writeFile(t, "host = \"http://localhost:11434\"\n")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if c.OSC52Copy {
		t.Error("Load with no osc52_copy key enabled OSC 52 copy")
	}
}

// TestOSC52CopySaveLoadRoundTrip proves the key survives Save → Load and
// lands in the TOML under the documented name.
func TestOSC52CopySaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")

	c := Default()
	c.filePath = p
	c.OSC52Copy = true

	if err := Save(c); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "osc52_copy = true") {
		t.Errorf("saved config missing `osc52_copy = true`:\n%s", b)
	}

	loaded, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatalf("Load after Save error: %v", err)
	}
	if !loaded.OSC52Copy {
		t.Error("loaded.OSC52Copy = false, want true")
	}
	assertNoTempFiles(t, dir)
}

// TestOSC52CopyEnvOverride proves SELFTUI_OSC52_COPY parses as a bool and
// beats the file, matching the tools_enabled precedence.
func TestOSC52CopyEnvOverride(t *testing.T) {
	p := writeFile(t, "osc52_copy = false\n")
	t.Setenv(envPrefix+"OSC52_COPY", "true")
	c, err := Load(Overrides{ConfigPath: &p})
	if err != nil {
		t.Fatal(err)
	}
	if !c.OSC52Copy {
		t.Error("OSC52Copy = false, want true (env beats file)")
	}

	p2 := writeFile(t, "osc52_copy = true\n")
	t.Setenv(envPrefix+"OSC52_COPY", "not-a-bool")
	if _, err := Load(Overrides{ConfigPath: &p2}); err == nil {
		t.Error("Load accepted a non-bool SELFTUI_OSC52_COPY, want parse error")
	}
}
