package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fake sinks for the recorder seam (NewRecorderWithOpener) -------------

// stallWriter is a Writer whose Append parks until release is closed, so a
// test can hold the recorder mid-write (a wedged/slow transcript directory)
// and prove callers never wait on it. entered closes when the first Append
// attempt is inside the stall.
type stallWriter struct {
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
	mu       sync.Mutex
	order    []string
	calls    int
	flushes  int
	closed   bool
	blockAll bool // when true every Append stalls (not just the first)
}

func newStallWriter() *stallWriter {
	return &stallWriter{entered: make(chan struct{}), release: make(chan struct{})}
}

func (w *stallWriter) Append(role, model, content, meta string, at time.Time) error {
	w.mu.Lock()
	w.calls++
	w.order = append(w.order, role+":"+content)
	first := w.calls == 1
	w.mu.Unlock()
	if first || w.blockAll {
		w.once.Do(func() { close(w.entered) })
		<-w.release
	}
	return nil
}

func (w *stallWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushes++
	return nil
}

func (w *stallWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *stallWriter) Path() string { return "" }

func (w *stallWriter) saw() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.order...)
}

// errorWriter fails exactly one Append attempt (failAt, 1-based; 0 = never)
// and counts every Append call it actually received.
type errorWriter struct {
	mu      sync.Mutex
	appends int
	flushes int
	closed  bool
	failAt  int
	failErr error
}

func newErrorWriter(failAt int) *errorWriter {
	return &errorWriter{failAt: failAt, failErr: errors.New("simulated write failure")}
}

func (w *errorWriter) Append(role, model, content, meta string, at time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appends++
	if w.appends == w.failAt {
		return w.failErr
	}
	return nil
}

func (w *errorWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushes++
	return w.failErr // the sink is already broken once it fails
}

func (w *errorWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *errorWriter) Path() string { return "" }

// recAppend enqueues one turn and waits for the recorder's ordered ack.
func recAppend(t *testing.T, r *Recorder, role, content string, at time.Time) Result {
	t.Helper()
	done, err := r.Append(role, "qwen3:8b", content, "", at)
	if err != nil {
		t.Fatalf("Append(%q): %v", content, err)
	}
	res, ok := <-done
	if !ok {
		t.Fatalf("Append(%q): ack channel closed without a result", content)
	}
	return res
}

// waitClosed polls ch for up to 2s and reports whether it closed.
func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not happen within 2s", what)
	}
}

// --- ordering and durability -------------------------------------------------

// TestRecorderOrdersCommittedTurns drives user→assistant→user turns through
// the actor and proves the transcript file preserves exactly that order with
// the documented 0700/0600 permissions and readable markdown blocks.
func TestRecorderOrdersCommittedTurns(t *testing.T) {
	// Nested dir: MkdirAll creates it (0700) inside the temp root, so the
	// permission contract below is the recorder's own, not TempDir's.
	dir := filepath.Join(t.TempDir(), "sessions")
	r := NewRecorder(dir, "http://localhost:11434")
	at := time.Date(2026, 9, 3, 21, 31, 0, 0, time.UTC)
	want := []string{"first user turn", "first assistant turn", "second user turn"}
	for i, content := range want {
		role := "user"
		if i == 1 {
			role = "assistant"
		}
		if res := recAppend(t, r, role, content, at.Add(time.Duration(i)*time.Second)); res.Err != nil {
			t.Fatalf("turn %q: %v", content, res.Err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("session dir entries = %v (%v), want exactly one transcript", entries, err)
	}
	fi, err := os.Stat(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("transcript perms = %o, want 600", fi.Mode().Perm())
	}
	if di, err := os.Stat(dir); err == nil && di.Mode().Perm() != 0o700 {
		t.Errorf("session dir perms = %o, want 700", di.Mode().Perm())
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"# SelfTUI chat session", "# host: http://localhost:11434"} {
		if !strings.Contains(text, want) {
			t.Errorf("transcript missing header %q:\n%s", want, text)
		}
	}
	// Deterministic order: each turn appears strictly after the previous one.
	last := -1
	for _, content := range want {
		idx := strings.Index(text, content)
		if idx < 0 {
			t.Fatalf("transcript missing turn %q:\n%s", content, text)
		}
		if idx <= last {
			t.Errorf("turn %q appears before an earlier turn (order lost):\n%s", content, text)
		}
		last = idx
	}
}

// TestRecorderFlushDrainsEarlierAppends proves /export semantics at the actor
// level: a flush job is processed strictly after every earlier enqueued turn,
// reports the real transcript path, and makes those turns durable.
func TestRecorderFlushDrainsEarlierAppends(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, "")
	recAppend(t, r, "user", "turn one", time.Now())
	recAppend(t, r, "assistant", "turn two", time.Now())

	done, err := r.Flush()
	if err != nil {
		t.Fatalf("Flush enqueue: %v", err)
	}
	res := <-done
	if res.Err != nil {
		t.Fatalf("Flush: %v", res.Err)
	}
	if res.Path == "" {
		t.Fatal("Flush with recorded turns must report the transcript path")
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatalf("flush reported path is not a file: %v", err)
	}
	body, _ := os.ReadFile(res.Path)
	for _, want := range []string{"turn one", "turn two"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("flush result missing %q", want)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

// --- the actor never blocks its callers ------------------------------------

// TestRecorderAppendDoesNotBlockCallerOnStalledWriter holds the writer inside
// a blocked Append and proves the caller can keep enqueueing and closing work
// the whole time: the recorder is a background actor, not an inline writer.
func TestRecorderAppendDoesNotBlockCallerOnStalledWriter(t *testing.T) {
	w := newStallWriter()
	r := NewRecorderWithOpener(t.TempDir(), "", func(string, string) (Writer, error) { return w, nil })

	done1, err := r.Append("user", "m", "hello", "", time.Now())
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}
	waitClosed(t, w.entered, "writer entering the stalled Append")

	// The writer is now parked mid-write; the caller must still be able to
	// enqueue (and the recorder must accept) further ordered work.
	done2, err := r.Append("user", "m", "second", "", time.Now())
	if err != nil {
		t.Fatalf("second Append while the writer is stalled: %v", err)
	}
	done3, err := r.Append("assistant", "m", "third", "", time.Now())
	if err != nil {
		t.Fatalf("third Append while the writer is stalled: %v", err)
	}

	close(w.release) // the wedged write finally completes
	for _, d := range []<-chan Result{done1, done2, done3} {
		if res := <-d; res.Err != nil {
			t.Fatalf("append ack: %v", res.Err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// The writer observed exactly the enqueue order.
	if got := w.saw(); strings.Join(got, "|") != "user:hello|user:second|assistant:third" {
		t.Errorf("writer append order = %q, want user:hello|user:second|assistant:third", got)
	}
	waitClosed(t, r.done, "recorder worker exit after Close")
}

// TestRecorderFlushWaitsForStalledEarlierTurn proves a flush job cannot report
// a path before an earlier stalled append has been written: ordering is held
// even under a wedged writer.
func TestRecorderFlushWaitsForStalledEarlierTurn(t *testing.T) {
	w := newStallWriter()
	r := NewRecorderWithOpener(t.TempDir(), "", func(string, string) (Writer, error) { return w, nil })

	done1, err := r.Append("user", "m", "hello", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	waitClosed(t, w.entered, "writer entering the stalled Append")

	doneF, err := r.Flush()
	if err != nil {
		t.Fatalf("Flush enqueue: %v", err)
	}
	// The flush must NOT complete while the earlier append is still stalled.
	select {
	case res := <-doneF:
		t.Fatalf("flush completed before its earlier stalled append: %+v", res)
	case <-time.After(100 * time.Millisecond):
	}
	close(w.release)
	res1 := <-done1
	if res1.Err != nil {
		t.Fatal(res1.Err)
	}
	resF := <-doneF
	if resF.Err != nil {
		t.Fatal(resF.Err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestRecorderOpenRunsOffCaller stalls the opener itself (a slow mkdir/open)
// and proves Append still returns immediately: even lazy Open is actor work.
func TestRecorderOpenRunsOffCaller(t *testing.T) {
	opened := make(chan struct{})
	release := make(chan struct{})
	r := NewRecorderWithOpener(t.TempDir(), "", func(string, string) (Writer, error) {
		close(opened)
		<-release
		return newErrorWriter(0), nil // a healthy sink once Open itself unstalls
	})
	done, err := r.Append("user", "m", "hello", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	waitClosed(t, opened, "opener entering the stalled open")
	// Caller is free while Open is parked.
	close(release)
	if res := <-done; res.Err != nil {
		t.Fatal(res.Err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

// --- failure discipline -----------------------------------------------------

// TestRecorderStopsWritingAfterFirstFailure proves one-error semantics at the
// actor: the first failed Append stops all later writes (the writer is never
// called again), every later job acks the same stable failure (so no waiter
// hangs), and Close still terminates the worker cleanly.
func TestRecorderStopsWritingAfterFirstFailure(t *testing.T) {
	w := newErrorWriter(2) // second Append attempt fails
	r := NewRecorderWithOpener(t.TempDir(), "", func(string, string) (Writer, error) { return w, nil })

	if res := recAppend(t, r, "user", "first", time.Now()); res.Err != nil {
		t.Fatalf("first append: %v", res.Err)
	}
	var failed []error
	for i := 0; i < 3; i++ {
		done, err := r.Append("user", "m", "after-failure", "", time.Now())
		if err != nil {
			t.Fatalf("append %d enqueue: %v", i, err)
		}
		res := <-done
		if res.Err == nil {
			t.Fatalf("append %d after the failure succeeded, want the stable failure", i)
		}
		failed = append(failed, res.Err)
	}
	for i := 1; i < len(failed); i++ {
		if failed[i] != failed[0] {
			t.Errorf("later failures = %v, want the same stable error every time", failed)
		}
	}
	w.mu.Lock()
	appends := w.appends
	closed := w.closed
	w.mu.Unlock()
	if appends != 2 {
		t.Errorf("writer saw %d Append calls, want exactly 2 (no writes after the failure)", appends)
	}
	if !closed {
		t.Error("recorder did not close the broken writer after the failure")
	}
	// A flush after the failure acks the same stable failure (never a path).
	doneF, err := r.Flush()
	if err != nil {
		t.Fatal(err)
	}
	if res := <-doneF; res.Err != failed[0] {
		t.Errorf("flush after failure = %v, want the stable failure %v", res.Err, failed[0])
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close after failure: %v", err)
	}
	waitClosed(t, r.done, "recorder worker exit after Close")
}

// TestRecorderOpenFailureIsReportedOnceAndStops proves an Open failure (mkdir/
// create) disables recording with one stable error and Close stays clean.
func TestRecorderOpenFailureIsReportedOnceAndStops(t *testing.T) {
	r := NewRecorderWithOpener(t.TempDir(), "", func(string, string) (Writer, error) {
		return nil, errors.New("simulated mkdir failure")
	})
	done, err := r.Append("user", "m", "hello", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res := <-done; res.Err == nil || res.Err.Error() != "simulated mkdir failure" {
		t.Fatalf("open-failure ack = %v, want the opener error", res.Err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close after open failure: %v", err)
	}
	waitClosed(t, r.done, "recorder worker exit after Close")
}

// TestRecorderCloseFlushesPendingTurnsAndIsIdempotent proves normal shutdown
// drains every earlier enqueued turn, exits the worker, and can be called
// again safely.
func TestRecorderCloseFlushesPendingTurnsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, "")
	// Enqueue turns without waiting for their acks: Close must drain them.
	acks := make([]<-chan Result, 0, 3)
	for _, turn := range []string{"one", "two", "three"} {
		done, err := r.Append("user", "m", turn, "", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		acks = append(acks, done)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for _, done := range acks {
		if res := <-done; res.Err != nil {
			t.Errorf("pending append after Close: %v", res.Err)
		}
	}
	waitClosed(t, r.done, "recorder worker exit after Close")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("session dir entries = %d, want one transcript after Close", len(entries))
	}
	body, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	for _, want := range []string{"one", "two", "three"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("Close dropped turn %q:\n%s", want, body)
		}
	}
	if err := r.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestRecorderBacklogFullRejectsWithoutBlocking pins the bounded-queue
// contract: when a wedged writer leaves the queue full, further enqueues fail
// immediately (never blocking Update) with the stable backlog error.
func TestRecorderBacklogFullRejectsWithoutBlocking(t *testing.T) {
	w := newStallWriter()
	r := &Recorder{
		dir:  t.TempDir(),
		host: "",
		open: func(string, string) (Writer, error) { return w, nil },
		jobs: make(chan job, 4),
		done: make(chan struct{}),
	}
	go r.run()

	// First append is consumed by the worker and parks inside the stalled
	// Append; the worker will not read again until it is released, so the
	// four-slot queue then holds exactly the next four appends.
	done1, err := r.Append("user", "m", "turn", "", time.Now())
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	waitClosed(t, w.entered, "writer entering the stalled Append")
	enqueued := []<-chan Result{done1}
	for i := 0; i < 4; i++ {
		done, err := r.Append("user", "m", "turn", "", time.Now())
		if err != nil {
			t.Fatalf("append %d within capacity: %v", i, err)
		}
		enqueued = append(enqueued, done)
	}
	if _, err := r.Append("user", "m", "overflow", "", time.Now()); err == nil {
		t.Fatal("append beyond the backlog should fail immediately")
	} else if !errors.Is(err, ErrRecorderBacklog) {
		t.Fatalf("backlog error = %v, want ErrRecorderBacklog", err)
	}
	close(w.release)
	for _, done := range enqueued {
		if res := <-done; res.Err != nil {
			t.Fatal(res.Err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
