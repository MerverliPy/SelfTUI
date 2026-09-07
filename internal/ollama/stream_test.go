package ollama

// Phase-5 (v0.1 hardening) stream tests: the shared NDJSON decoder must cap a
// single event at 4 MiB, cap cumulative chat content+thinking at 16 MiB, and
// abort a stream that delivers no bytes for the idle window — while never
// imposing a total stream deadline and while caller cancellation always wins
// over the idle timer. Idle is injected per client via c.streamIdle; nothing
// here waits anywhere near the 90s production default.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const miB = 1 << 20

// stallServer serves prefix (optional first NDJSON line) then holds the
// handler open without writing another byte until the client disconnects.
func stallServer(t *testing.T, prefix string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain the request body before parking: only after the body reaches
		// EOF does net/http arm its client-close-detection background read, so
		// a handler that never reads r.Body can park forever once the client
		// disconnects (rare race -> 10m package timeout in CI under 2 vCPU).
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		if prefix != "" {
			io.WriteString(w, prefix)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// chatReq is the canonical minimal chat request used by these tests.
func chatReq() ChatRequest {
	return ChatRequest{
		Model:    "qwen3:8b",
		Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}},
	}
}

func TestChatOversizedEventRejected(t *testing.T) {
	// A single chat event whose raw NDJSON line exceeds the 4 MiB per-event
	// cap must be rejected with the stable per-event message before anything
	// is delivered.
	content := strings.Repeat("a", 4*miB) // raw line is 4 MiB + JSON framing
	body := fmt.Sprintf(`{"message":{"role":"assistant","content":"%s"},"done":false}`+"\n", content)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This host's localhost port prober sends stray GET /; only the
		// real POST matters, so ignore everything else without failing.
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")

	err := c.ChatStream(context.Background(), chatReq(), nil)
	if err == nil {
		t.Fatal("want per-event size error, got nil")
	}
	if !strings.Contains(err.Error(), "stream event exceeds 4194304 bytes") {
		t.Errorf("error = %v, want per-event cap message", err)
	}
}

func TestChatCumulativeToolBytesOverflowRejected(t *testing.T) {
	// H-03: tool calls must count toward the cumulative 16 MiB chat ceiling.
	// Each event carries one native tool call whose arguments hold ~2.5 MiB
	// of text: every event is well under the 4 MiB per-event raw cap, but the
	// raw NDJSON bytes (JSON framing and tool calls included) cross the
	// cumulative cap on the seventh event. Before the fix the ceiling counted
	// only decoded content/thinking, so tool-argument bytes escaped it and
	// every event was delivered.
	chunk := strings.Repeat("a", 2_600_000)
	event := fmt.Sprintf(`{"message":{"role":"assistant","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"%s"}}}]},"done":false}`+"\n", chunk)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tolerate this host's localhost port prober (stray GET /); only the
		// real POST matters.
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		for i := 0; i < 7; i++ {
			io.WriteString(w, event)
		}
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")

	delivered := 0
	err := c.ChatStream(context.Background(), chatReq(), func(ChatEvent) { delivered++ })
	if err == nil {
		t.Fatal("want cumulative cap error, got nil")
	}
	if !strings.Contains(err.Error(), "chat stream exceeds 16777216 bytes") {
		t.Errorf("error = %v, want chat cumulative cap message", err)
	}
	if delivered != 6 {
		t.Errorf("events delivered = %d, want 6 (crossing event rejected before delivery)", delivered)
	}
}

func TestChatCumulativeOverflowRejected(t *testing.T) {
	// Four content events of ~4 MiB each stay under both caps; a fifth event
	// carrying thinking bytes crosses the 16 MiB cumulative chat ceiling.
	// The cap counts complete raw NDJSON event bytes (content, thinking,
	// framing, and any tool calls), so the crossing event is rejected before
	// it is delivered.
	chunk := strings.Repeat("a", 4*miB-8*1024) // each event stays under 4 MiB raw
	var b strings.Builder
	for i := 0; i < 4; i++ {
		fmt.Fprintf(&b, `{"message":{"role":"assistant","content":"%s"},"done":false}`+"\n", chunk)
	}
	fmt.Fprintf(&b, `{"message":{"role":"assistant","thinking":"%s"},"done":false}`+"\n", chunk)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tolerate this host's localhost port prober (stray GET /); only the
		// real POST matters. A genuine client bug still fails: the POST gets a
		// 404 and ChatStream reports the HTTP error instead of the cap.
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, b.String())
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")

	events := 0
	err := c.ChatStream(context.Background(), chatReq(), func(ChatEvent) { events++ })
	if err == nil {
		t.Fatal("want cumulative cap error, got nil")
	}
	if !strings.Contains(err.Error(), "chat stream exceeds 16777216 bytes") {
		t.Errorf("error = %v, want chat cumulative cap message", err)
	}
	if events != 4 {
		t.Errorf("events delivered = %d, want 4 (overflow event rejected before delivery)", events)
	}
}

func TestPullOversizedEventRejected(t *testing.T) {
	// Pull events get the same per-event cap: one status line larger than
	// 4 MiB must fail with the stable per-event message.
	body := fmt.Sprintf(`{"status":"%s"}`+"\n", strings.Repeat("p", 4*miB+1))
	c, _ := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, body)
	}))

	err := c.Pull(context.Background(), "big-model", nil)
	if err == nil {
		t.Fatal("want per-event size error, got nil")
	}
	if !strings.Contains(err.Error(), "stream event exceeds 4194304 bytes") {
		t.Errorf("error = %v, want per-event cap message", err)
	}
}

func TestChatStalledBodyTimesOut(t *testing.T) {
	// One delta arrives, then the body goes silent: the idle watchdog must
	// abort with the idle message, not wait for any caller deadline.
	srv := stallServer(t, `{"message":{"role":"assistant","content":"hi"},"done":false}`+"\n")
	c := New(srv.URL, "")
	c.streamIdle = 60 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.ChatStream(ctx, chatReq(), nil)
	if err == nil {
		t.Fatal("want idle timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "stream idle") {
		t.Errorf("error = %v, want idle-timeout message", err)
	}
}

func TestPullStalledBodyTimesOut(t *testing.T) {
	// Same idle protection on the pull side: a manifest line then silence.
	srv := stallServer(t, `{"status":"pulling manifest"}`+"\n")
	c := New(srv.URL, "")
	c.streamIdle = 60 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.Pull(ctx, "big-model", nil)
	if err == nil {
		t.Fatal("want idle timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "stream idle") {
		t.Errorf("error = %v, want idle-timeout message", err)
	}
}

func TestPullSteadyProgressOutlivesIdle(t *testing.T) {
	// Progress lines keep arriving well inside the idle window: the pull must
	// span several idle windows and still complete — the timeout is per idle
	// gap, never a total request deadline (pulls can run minutes).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fl, _ := w.(http.Flusher)
		for i := 0; i < 25; i++ {
			fmt.Fprintf(w, `{"status":"pulling layer-%d","completed":%d}`+"\n", i, i*1000)
			fl.Flush()
			time.Sleep(10 * time.Millisecond)
		}
		io.WriteString(w, `{"status":"success"}`+"\n")
		fl.Flush()
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")
	c.streamIdle = 150 * time.Millisecond

	start := time.Now()
	count := 0
	if err := c.Pull(context.Background(), "big-model", func(PullProgress) { count++ }); err != nil {
		t.Fatalf("Pull with steady progress: %v", err)
	}
	if count != 26 {
		t.Errorf("progress events = %d, want 26 (25 layer lines + success)", count)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("pull finished in %v; want it to span more than one idle window", elapsed)
	}
}

func TestChatCancelBeatsIdle(t *testing.T) {
	// A caller cancel arriving long before the idle window must win: the
	// stream fails fast with the context error, never the idle error.
	srv := stallServer(t, "")
	c := New(srv.URL, "")
	c.streamIdle = 5 * time.Second // idle far away; only cancellation can end this

	ctx, cancelCtx := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- c.ChatStream(ctx, chatReq(), nil)
	}()
	time.Sleep(40 * time.Millisecond)
	start := time.Now()
	cancelCtx()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want error after cancel, got nil")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("error = %v, want context-canceled message", err)
		}
		if strings.Contains(err.Error(), "stream idle") {
			t.Errorf("error = %v: idle won over caller cancellation", err)
		}
		if elapsed := time.Since(start); elapsed > 700*time.Millisecond {
			t.Errorf("cancel took %v, want prompt return well before the idle window", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ChatStream did not return after context cancel")
	}
}

func TestPullCancelBeatsIdle(t *testing.T) {
	srv := stallServer(t, "")
	c := New(srv.URL, "")
	c.streamIdle = 5 * time.Second

	ctx, cancelCtx := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- c.Pull(ctx, "big-model", nil)
	}()
	time.Sleep(40 * time.Millisecond)
	start := time.Now()
	cancelCtx()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want error after cancel, got nil")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("error = %v, want context-canceled message", err)
		}
		if strings.Contains(err.Error(), "stream idle") {
			t.Errorf("error = %v: idle won over caller cancellation", err)
		}
		if elapsed := time.Since(start); elapsed > 700*time.Millisecond {
			t.Errorf("cancel took %v, want prompt return well before the idle window", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Pull did not return after context cancel")
	}
}

func TestPullMalformedNDJSONRejected(t *testing.T) {
	// A non-JSON line in the middle of a pull stream is a decode error with
	// the endpoint context.
	c, _ := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"status":"pulling manifest"}`+"\nnot-json\n")
	}))

	err := c.Pull(context.Background(), "big-model", nil)
	if err == nil {
		t.Fatal("want decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode stream") {
		t.Errorf("error = %v, want decode-stream context", err)
	}
}

func TestPullEOFWithoutSuccessRejected(t *testing.T) {
	// Hitting EOF before the terminal "success" event is a hard error.
	c, _ := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"status":"pulling manifest"}`+"\n")
	}))

	err := c.Pull(context.Background(), "big-model", nil)
	if err == nil {
		t.Fatal("want error when the stream ends without success, got nil")
	}
	if !strings.Contains(err.Error(), "stream ended without success") {
		t.Errorf("error = %v, want ended-without-success message", err)
	}
}

// errorStallServer responds non-2xx (500), flushes the status, then holds the
// error body open without writing a single body byte until the client
// disconnects — the exact stalled-error-body shape P1-2 targets. The request
// body is drained first so client-close detection arms (same discipline as
// stallServer).
func errorStallServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestChatStalledErrorBodyBoundedByIdle proves a non-2xx /api/chat response
// whose error body delivers no bytes is aborted by the idle watchdog, not
// pinned until the caller's deadline: the phase-5 "streams are bounded"
// guarantee must cover the error branch too (P1-2). RED before the fix: the
// error body was read with io.ReadAll and no idle bound on the no-timeout
// stream client, so the producer hung until caller cancellation.
func TestChatStalledErrorBodyBoundedByIdle(t *testing.T) {
	srv := errorStallServer(t)
	c := New(srv.URL, "")
	c.streamIdle = 60 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	err := c.ChatStream(ctx, chatReq(), nil)
	if err == nil {
		t.Fatal("want idle timeout error on a stalled error body, got nil")
	}
	if !strings.Contains(err.Error(), "stream idle") {
		t.Errorf("error = %v, want idle-timeout message naming the abort", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("stalled error body took %v to abort; want the ~60ms idle window", elapsed)
	}
}

// TestPullStalledErrorBodyBoundedByIdle is the pull-side twin: a 500 response
// to POST /api/pull with a silent error body must abort on the idle window.
func TestPullStalledErrorBodyBoundedByIdle(t *testing.T) {
	srv := errorStallServer(t)
	c := New(srv.URL, "")
	c.streamIdle = 60 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	err := c.Pull(ctx, "big-model", nil)
	if err == nil {
		t.Fatal("want idle timeout error on a stalled error body, got nil")
	}
	if !strings.Contains(err.Error(), "stream idle") {
		t.Errorf("error = %v, want idle-timeout message naming the abort", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("stalled error body took %v to abort; want the ~60ms idle window", elapsed)
	}
}

// TestNon2xxErrorBodyStillSurfaced ensures the error-body read still reports
// a genuinely received error payload (fast path unchanged): a 500 with a
// complete JSON error body must surface the API error, not the idle error.
func TestNon2xxErrorBodyStillSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"boom from host"}`)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")
	c.streamIdle = 5 * time.Second // far away; the complete body must win

	err := c.ChatStream(context.Background(), chatReq(), nil)
	if err == nil {
		t.Fatal("want API error, got nil")
	}
	if !strings.Contains(err.Error(), "boom from host") {
		t.Errorf("error = %v, want the host's error payload surfaced", err)
	}
	if strings.Contains(err.Error(), "stream idle") {
		t.Errorf("error = %v: a complete error body must not read as idle", err)
	}
}
