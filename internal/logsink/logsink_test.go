package logsink

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestRedactRegisteredSecret(t *testing.T) {
	got := Redact("Authorization: Bearer sekrit (host=ok)", []string{"sekrit"})
	if got != "Authorization: Bearer [redacted] (host=ok)" {
		t.Errorf("Redact = %q, want the configured token scrubbed", got)
	}
}

func TestRedactUnregisteredBearerPattern(t *testing.T) {
	// A token that was never registered must still be scrubbed by the
	// generic Bearer pattern (defense in depth against future header traces).
	got := Redact("http.Authorization = Bearer abc123XYZ-/._~", nil)
	if got != "http.Authorization = Bearer [redacted]" {
		t.Errorf("Redact = %q, want generic Bearer credential scrubbed", got)
	}
}

func TestRedactKeepsProseBearer(t *testing.T) {
	// The 4-char floor keeps ordinary English intact ("Bearer of good news").
	in := "Bearer of good news and Bearer o"
	if got := Redact(in, nil); got != in {
		t.Errorf("Redact = %q, want prose untouched", got)
	}
}

func TestRedactCaseInsensitiveAndMultiple(t *testing.T) {
	got := Redact("bearer AAAA and BEARER bbbb", []string{"bbbb"})
	want := "Bearer [redacted] and BEARER [redacted]"
	if got != want {
		t.Errorf("Redact = %q, want %q", got, want)
	}
}

func TestRedactEmptySecretIsSkipped(t *testing.T) {
	in := "anything goes"
	if got := Redact(in, []string{"", "x"}); got != "anything goes" {
		t.Errorf("Redact = %q, want input kept (empty secret must not replace everything)", got)
	}
}

func TestRingKeepsNewestAndCaps(t *testing.T) {
	r := NewRing(3)
	for i := 0; i < 5; i++ {
		if _, err := fmt.Fprintf(r, "line-%d\n", i); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	got := r.Lines()
	if len(got) != 3 {
		t.Fatalf("Lines = %d entries, want 3", len(got))
	}
	want := []string{"line-2", "line-3", "line-4"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Lines[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestRingSplitsMultilineWrite(t *testing.T) {
	r := NewRing(10)
	r.Write([]byte("one\ntwo\n"))
	got := r.Lines()
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("Lines = %q, want [one two]", got)
	}
}

func TestRingLinesIsACopy(t *testing.T) {
	r := NewRing(2)
	r.Write([]byte("a\n"))
	got := r.Lines()
	got[0] = "mutated"
	if r.Lines()[0] != "a" {
		t.Error("Lines() returned a live view; the ring was mutated")
	}
}

func TestRingClampsTinyCap(t *testing.T) {
	r := NewRing(0)
	r.Write([]byte("only\n"))
	if got := r.Lines(); len(got) != 1 || got[0] != "only" {
		t.Errorf("Lines = %q, want [only]", got)
	}
}

func TestSinkRedactsFileAndRing(t *testing.T) {
	var file bytes.Buffer
	ring := NewRing(10)
	s := New(&file, ring, "sekrit")
	s.Write([]byte("INF ollama request method=GET Authorization=\"Bearer sekrit\"\n"))

	if strings.Contains(file.String(), "sekrit") {
		t.Errorf("file sink leaked the token: %q", file.String())
	}
	for _, line := range ring.Lines() {
		if strings.Contains(line, "sekrit") {
			t.Errorf("ring leaked the token: %q", line)
		}
	}
	if !strings.Contains(file.String(), "Bearer [redacted]") {
		t.Errorf("file = %q, want the redacted form", file.String())
	}
}

func TestSinkSetSecretRedactsNewToken(t *testing.T) {
	var file bytes.Buffer
	s := New(&file, NewRing(10), "old-token")
	s.SetSecret("new-token")
	s.Write([]byte("INF x old-token new-token\n"))
	if strings.Contains(file.String(), "old-token") || strings.Contains(file.String(), "new-token") {
		t.Errorf("file = %q, want both tokens redacted", file.String())
	}
	s.SetSecret("new-token") // duplicate registration is a no-op
	s.Write([]byte("INF x new-token\n"))
	if n := strings.Count(file.String(), "[redacted]"); n != 3 {
		t.Errorf("redacted count = %d, want 3 (duplicate secret not re-registered)", n)
	}
}

func TestSinkNilFileStillFeedsRing(t *testing.T) {
	ring := NewRing(10)
	s := New(nil, ring, "")
	s.Write([]byte("INF ring-only\n"))
	if got := ring.Lines(); len(got) != 1 || got[0] != "INF ring-only" {
		t.Errorf("Lines = %q, want the entry despite a nil file", got)
	}
}

func TestSinkConcurrentWritesAreIntact(t *testing.T) {
	var file safeBuffer
	ring := NewRing(1024)
	s := New(&file, ring, "sekrit")
	const n, workers = 200, 8
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < n/workers; i++ {
				s.Write([]byte(fmt.Sprintf("INF worker-%d item-%d token=sekrit\n", w, i)))
			}
		}(w)
	}
	wg.Wait()
	if strings.Contains(file.String(), "sekrit") {
		t.Error("file leaked the token under concurrency")
	}
	if got := len(ring.Lines()); got != n {
		t.Errorf("ring holds %d entries, want %d", got, n)
	}
}

// safeBuffer is a minimal mutex-guarded io.Writer so the concurrency test
// can assert on the file stream without racing.
type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *safeBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *safeBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// Sink satisfies io.Writer so log.NewWithOptions can wrap it directly.
var _ io.Writer = (*Sink)(nil)
