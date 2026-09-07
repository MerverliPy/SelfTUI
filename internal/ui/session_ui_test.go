package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/session"
)

// waitForSession polls the transcript dir until the single per-run file
// contains every want substring. Transcript writes are now async (M-04: the
// recorder worker owns the file), so persistence assertions wait for the
// worker instead of reading the dir right after an update.
func waitForSession(t *testing.T, dir string, want ...string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 1 {
			body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
			if err == nil {
				text := string(body)
				ok := true
				for _, w := range want {
					if !strings.Contains(text, w) {
						ok = false
						break
					}
				}
				if ok {
					return text
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("transcript never contained %q within 3s (dir %s)", want, dir)
	return ""
}

// blockWriter is the M-04 filesystem seam for UI tests: its first Append call
// parks until release is closed (a wedged/slow transcript directory), letting
// tests prove AgentView.Update and the /export path never wait on it. Every
// call is recorded in order so tests can assert the recorder's FIFO ordering
// across a stalled write.
type blockWriter struct {
	entered chan struct{} // closed when the first Append is inside the stall
	release chan struct{} // close to let the stalled Append finish
	once    sync.Once

	mu      sync.Mutex
	order   []string
	flushes int
}

func newBlockWriter() *blockWriter {
	return &blockWriter{entered: make(chan struct{}), release: make(chan struct{})}
}

func (w *blockWriter) Append(role, model, content, meta string, at time.Time) error {
	w.mu.Lock()
	w.order = append(w.order, "append:"+role+":"+content)
	first := len(w.order) == 1
	w.mu.Unlock()
	if first {
		w.once.Do(func() { close(w.entered) })
		<-w.release
	}
	return nil
}

func (w *blockWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushes++
	w.order = append(w.order, "flush")
	return nil
}

func (w *blockWriter) Close() error { return nil }
func (w *blockWriter) Path() string { return "" }

func (w *blockWriter) saw() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.order...)
}

// failWriter errors on every Append and counts how often it was called, so
// tests can prove recording stops after the first failure.
type failWriter struct {
	err   error
	calls int
}

func (w *failWriter) Append(role, model, content, meta string, at time.Time) error {
	w.calls++
	return w.err
}

func (w *failWriter) Flush() error { return nil }
func (w *failWriter) Close() error { return nil }
func (w *failWriter) Path() string { return "" }

// TestAgentViewPersistsChatSession drives one turn with a session dir
// configured and asserts the per-process transcript file records both turns
// in readable markdown (async recorder; the file appears once the worker
// writes).
func TestAgentViewPersistsChatSession(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "http://fake-host:11434")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	defer v.recorder.Close() // drain + stop the worker before the test exits

	text := waitForSession(t, dir,
		"# SelfTUI chat session", "# host: http://fake-host:11434",
		"## user (qwen3:8b)", "hello",
		"## assistant (qwen3:8b)", "func main()")
	if strings.Index(text, "hello") > strings.Index(text, "func main()") {
		t.Errorf("user turn recorded after the assistant turn:\n%s", text)
	}
	// The in-memory transcript is untouched by persistence.
	if len(v.turns) != 2 {
		t.Errorf("turns = %d, want 2", len(v.turns))
	}
}

// TestAgentViewSessionTurnOrder drives user→assistant→user across two full
// turns through the update loop and proves the recorder preserved the commit
// order (enqueue order is the loop's order; one actor writes it exactly so).
func TestAgentViewSessionTurnOrder(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	for _, msg := range []string{"hello", "second"} {
		typeText(t, &v, msg)
		v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		drainChat(t, &v)
	}
	defer v.recorder.Close()

	text := waitForSession(t, dir, "hello", "second")
	// The second assistant reply lands after the second user turn; wait until
	// both assistant headers are durable before asserting order.
	deadline := time.Now().Add(3 * time.Second)
	for strings.Count(text, "## assistant") < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		if entries, err := os.ReadDir(dir); err == nil && len(entries) == 1 {
			if b, err := os.ReadFile(filepath.Join(dir, entries[0].Name())); err == nil {
				text = string(b)
			}
		}
	}
	if n := strings.Count(text, "## user"); n != 2 {
		t.Errorf("user headers = %d, want 2:\n%s", n, text)
	}
	if n := strings.Count(text, "## assistant"); n != 2 {
		t.Errorf("assistant headers = %d, want 2 (second reply never recorded):\n%s", n, text)
	}
	u1 := strings.Index(text, "hello")
	a1 := strings.Index(text[u1:], "func main()") + u1
	u2 := strings.Index(text[a1:], "second") + a1
	rest := text[a1+1:]
	a2 := strings.Index(rest, "func main()") + a1 + 1
	if u1 < 0 || a1 < 0 || u2 < 0 || a2 < 0 {
		t.Fatalf("order markers not found in transcript:\n%s", text)
	}
	// user1 < assistant1 < user2 < assistant2.
	if !(u1 < a1 && a1 < u2 && u2 < a2) {
		t.Errorf("turn order lost: u1=%d a1=%d u2=%d a2=%d:\n%s", u1, a1, u2, a2, text)
	}
}

func TestAgentViewExportCommandShowsPath(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	dir := t.TempDir()
	v := testAgent(t, client)
	v = v.WithSessionDir(dir, "")
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	defer v.recorder.Close()

	typeText(t, &v, "/export")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("export with a live session should arm a flush command")
	}
	// The flush runs on the recorder worker after every earlier enqueued
	// turn; the command delivers the completion message.
	v, _ = v.Update(cmd())
	if !strings.Contains(v.notice, "transcript: ") || !strings.Contains(v.notice, filepath.Base(dir)) {
		t.Errorf("notice = %q, want the transcript path", v.notice)
	}
	if _, err := os.Stat(strings.TrimPrefix(v.notice, "transcript: ")); err != nil {
		t.Errorf("notice path is not a real file: %v", err)
	}
	// The export reports the transcript file; it must never claim the
	// conversation itself can be resumed from it (chat stays in-memory).
	if strings.Contains(strings.ToLower(v.notice), "resume") {
		t.Errorf("notice = %q, must not claim the conversation can be resumed", v.notice)
	}
}

func TestAgentViewExportWithoutRecording(t *testing.T) {
	// No session dir configured: /export explains recording is off.
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "/export")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("export with recording off must not arm a command")
	}
	if !strings.Contains(v.notice, "recording is off") {
		t.Errorf("notice = %q, want the off-state hint", v.notice)
	}

	// Dir configured but no messages yet (no recorder was ever started).
	v2 := testAgent(t, nil)
	v2 = v2.WithSessionDir(t.TempDir(), "")
	v2, _ = v2.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v2, "/export")
	v2, cmd = v2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("export with nothing recorded must not arm a command")
	}
	if !strings.Contains(v2.notice, "nothing recorded yet") {
		t.Errorf("notice = %q, want the empty hint", v2.notice)
	}
}

func TestAgentViewWithoutSessionDirWritesNothing(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	v := testAgent(t, client) // default: no dir
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	if v.recorder != nil {
		t.Fatal("no recorder should be started without a dir")
	}
	if v.sessionErr || v.notice != "" {
		t.Errorf("unexpected session state: err=%v notice=%q", v.sessionErr, v.notice)
	}
}

// TestAgentViewSessionWriteNeverBlocksUpdate is the M-04 core proof: while
// the transcript writer is stalled inside a write, committing a user turn and
// further updates must return promptly (the recorder owns the I/O). On the
// pre-fix code this update would park inside the synchronous append and time
// out.
func TestAgentViewSessionWriteNeverBlocksUpdate(t *testing.T) {
	w := newBlockWriter()
	dir := t.TempDir()
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v = v.WithSessionDir(dir, "")
	v.recorder.Close() // stop the eager composition-root recorder; this test injects its own
	v.recorder = session.NewRecorderWithOpener(dir, "", func(string, string) (session.Writer, error) {
		return w, nil
	})
	released := false
	t.Cleanup(func() {
		if !released {
			close(w.release)
		}
		if v.recorder != nil {
			v.recorder.Close()
		}
	})

	typeText(t, &v, "hello")
	upd := make(chan AgentView, 1)
	go func() {
		vv, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		upd <- vv
	}()
	// The commit update must return even while the transcript write is
	// stalled (a wedged XDG dir / slow paste must never freeze the UI).
	select {
	case v = <-upd:
	case <-time.After(time.Second):
		t.Fatal("AgentView.Update blocked on a stalled transcript write (M-04)")
	}
	// The writer is provably parked mid-append right now.
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("transcript writer never entered the stalled append")
	}
	// The view stays fully responsive while the write is still parked.
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	if v.sessionErr {
		t.Fatalf("a stalled (not failed) write disabled recording: %q", v.notice)
	}
	// Un-stall: the committed turn lands in order, exactly once.
	close(w.release)
	released = true
	deadline := time.Now().Add(time.Second)
	for {
		if got := w.saw(); len(got) == 1 && got[0] == "append:user:hello" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("writer never received the committed turn, saw %v", w.saw())
		}
		time.Sleep(2 * time.Millisecond)
	}
	if len(v.turns) != 1 {
		t.Errorf("turns = %d, want 1 committed user turn", len(v.turns))
	}
}

// TestAgentViewSessionWriteNeverBlocksUpdateStalledOpen is the same proof for
// the lazy Open path: even when mkdir/open of the transcript itself is
// stalled, the commit update returns (Open is recorder work, M-04).
func TestAgentViewSessionWriteNeverBlocksUpdateStalledOpen(t *testing.T) {
	w := newBlockWriter()
	opened := make(chan struct{})
	openRelease := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			select {
			case <-openRelease:
			default:
				close(openRelease)
			}
			close(w.release)
		})
	}
	dir := t.TempDir()
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v = v.WithSessionDir(dir, "")
	v.recorder.Close() // stop the eager composition-root recorder; this test injects its own
	v.recorder = session.NewRecorderWithOpener(dir, "", func(string, string) (session.Writer, error) {
		close(opened)
		<-openRelease
		return w, nil
	})
	t.Cleanup(func() {
		release()
		if v.recorder != nil {
			v.recorder.Close()
		}
	})

	typeText(t, &v, "hello")
	upd := make(chan AgentView, 1)
	go func() {
		vv, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		upd <- vv
	}()
	select {
	case v = <-upd:
	case <-time.After(time.Second):
		t.Fatal("AgentView.Update blocked on a stalled transcript open (M-04)")
	}
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("recorder never attempted the transcript open")
	}
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40}) // responsive mid-open

	release() // un-stall the open and the append
	deadline := time.Now().Add(time.Second)
	for {
		if got := w.saw(); len(got) == 1 && got[0] == "append:user:hello" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("writer never received the committed turn after the open unstalled, saw %v", w.saw())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestAgentViewExportWaitsForStalledEarlierTurn proves the /export barrier:
// the flush command must not report before every earlier enqueued turn has
// been written, even when the writer is stalled (ordering under a wedged
// sink, M-04).
func TestAgentViewExportWaitsForStalledEarlierTurn(t *testing.T) {
	w := newBlockWriter()
	dir := t.TempDir()
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v = v.WithSessionDir(dir, "")
	v.recorder.Close() // stop the eager composition-root recorder; this test injects its own
	v.recorder = session.NewRecorderWithOpener(dir, "", func(string, string) (session.Writer, error) {
		return w, nil
	})
	released := false
	t.Cleanup(func() {
		if !released {
			close(w.release)
		}
		if v.recorder != nil {
			v.recorder.Close()
		}
	})

	// Enqueue a committed user turn whose write is stalled on the worker.
	if _, ack := v.enqueueSessionTurn("user", "qwen3:8b", "hello", "", time.Now()); ack == nil {
		t.Fatal("commit should arm an ack command")
	}
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("transcript writer never entered the stalled append")
	}

	// /export while the earlier turn is still stalled: the flush must be
	// queued behind it and must not complete until the turn is written.
	typeText(t, &v, "/export")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("export with a live session should arm a flush command")
	}
	exportDone := make(chan tea.Msg, 1)
	go func() { exportDone <- cmd() }()
	select {
	case msg := <-exportDone:
		t.Fatalf("export completed before its stalled earlier turn: %#v", msg)
	case <-time.After(150 * time.Millisecond):
	}
	if flushes := w.flushes; flushes != 0 {
		t.Fatalf("writer flushed %d time(s) before its stalled append completed", flushes)
	}

	close(w.release)
	released = true
	select {
	case msg := <-exportDone:
		v, _ = v.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("export never completed after the stalled turn was written")
	}
	got := w.saw()
	if len(got) < 2 || got[0] != "append:user:hello" || got[1] != "flush" {
		t.Errorf("writer order = %v, want append:user:hello then flush (export after its earlier turns)", got)
	}
	if v.sessionErr {
		t.Errorf("healthy export disabled recording: %q", v.notice)
	}
}

// TestAgentViewSessionFailureSurfacesOnce proves one-error-only reporting at
// the UI: the first failed append disables recording with a single notice,
// later commits enqueue nothing, and /export reports the disabled state
// instead of re-surfacing the error.
func TestAgentViewSessionFailureSurfacesOnce(t *testing.T) {
	dir := t.TempDir()
	fw := &failWriter{err: errors.New("simulated transcript write failure")}
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	v = v.WithSessionDir(dir, "")
	v.recorder.Close() // stop the eager composition-root recorder; this test injects its own
	v.recorder = session.NewRecorderWithOpener(dir, "", func(string, string) (session.Writer, error) {
		return fw, nil
	})
	t.Cleanup(func() {
		if v.recorder != nil {
			v.recorder.Close()
		}
	})

	v, cmd := v.enqueueSessionTurn("user", "qwen3:8b", "first", "", time.Now())
	if cmd == nil {
		t.Fatal("first commit should arm an ack command")
	}
	v, _ = v.Update(cmd()) // delivers sessionAppendMsg{err}
	if !v.sessionErr {
		t.Fatal("a failed append must disable recording")
	}
	if !strings.Contains(v.notice, "simulated transcript write failure") {
		t.Errorf("notice = %q, want the failure surfaced once", v.notice)
	}
	wantNotice := v.notice

	// A later commit (after the failure was surfaced) enqueues nothing.
	v2, cmd2 := v.enqueueSessionTurn("assistant", "qwen3:8b", "second", "", time.Now())
	if cmd2 != nil {
		t.Fatal("a disabled session must not arm ack commands")
	}
	if !v2.sessionErr || v2.notice != wantNotice {
		t.Errorf("disabled session state: err=%v notice=%q, want unchanged %q", v2.sessionErr, v2.notice, wantNotice)
	}
	if fw.calls != 1 {
		t.Errorf("writer saw %d appends, want exactly 1 (no writes after the failure)", fw.calls)
	}

	// /export now reports the disabled state synchronously (no second error),
	// echoing the real failure — not the dead-end "send a message first" hint
	// (recording is permanently off, so more messages would never help; P1-5).
	ve, cmd3 := v2.exportSession()
	if cmd3 != nil {
		t.Fatal("export after a failure must not arm a flush command")
	}
	if !strings.Contains(ve.notice, "simulated transcript write failure") {
		t.Errorf("export notice after failure = %q, want the recorded failure echoed", ve.notice)
	}
	if strings.Contains(ve.notice, "send a message first") {
		t.Errorf("export notice after failure = %q, want no dead-end send-message advice", ve.notice)
	}
}
