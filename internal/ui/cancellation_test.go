package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/config"
	"selftui/internal/ollama"
)

// blockingServer parks model-operation requests until the request context is
// done. For streaming endpoints (chat/pull) it first writes + flushes a
// valid first line — the client must be mid-stream when the test cancels,
// mirroring the repo's esc-cancel tests (a request canceled while the client
// still awaits response headers does not reliably close the connection, so
// the server handler would never observe it). entered closes when the first
// parked request arrives; released closes once that request observes
// cancellation. flushLine is written verbatim before parking; "" parks
// immediately (used for the body-less GET list fetch, whose no-body request
// arms the server's connection watch at once).
func blockingServer(t *testing.T, flushLine string) (*httptest.Server, chan struct{}, chan struct{}) {
	t.Helper()
	entered := make(chan struct{})
	released := make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags", "/api/chat", "/api/pull":
			enterOnce.Do(func() { close(entered) })
			if flushLine != "" {
				w.Header().Set("Content-Type", "application/x-ndjson")
				strings.NewReader(flushLine).WriteTo(w)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			<-r.Context().Done()
			releaseOnce.Do(func() { close(released) })
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, entered, released
}

// requireSettledGoroutines polls (up to 1s) for the live goroutine count to
// return to baseline, proving a canceled operation released its workers (the
// view's stream producer, the httptest handler, the HTTP transport) instead
// of leaking them.
func requireSettledGoroutines(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("goroutine count = %d, want ≤ baseline %d within 1s (leak?)",
		runtime.NumGoroutine(), baseline)
}

// TestAgentViewParentCancellationStopsChat: a chat stream mid-flight on the
// server must stop within one second of the parent context being canceled,
// the server handler must observe the cancellation, the turn must surface as
// an error (never a clean/fabricated success), and no goroutines may be left
// behind.
func TestAgentViewParentCancellationStopsChat(t *testing.T) {
	srv, entered, released := blockingServer(t,
		`{"message":{"role":"assistant","content":"one"},"done":false}`+"\n")
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Default()
	v := newAgentView(parentCtx, ollama.New(srv.URL, ""), NewStyles("dark"), "dark",
		"", cfg.WorkspaceRoot, cfg.Agent.SystemPrompt, cfg.Agent, false, srv.URL)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "hello")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.streaming {
		t.Fatal("enter: chat stream should start")
	}

	// Wait until the first delta has provably reached the client (the runner
	// decoded the flushed line and is now blocked reading the next event),
	// so the cancel lands mid-stream, exactly like the esc-cancel tests.
	select {
	case msg := <-v.chatCh:
		v, _ = v.Update(msg)
	case <-entered:
		select {
		case msg := <-v.chatCh:
			v, _ = v.Update(msg)
		case <-time.After(2 * time.Second):
			t.Fatal("first chat delta never arrived after the request reached the server")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("chat request never reached the server")
	}

	baseline := runtime.NumGoroutine()
	start := time.Now()
	cancel()

	deadline := time.After(time.Second)
	for v.streaming {
		select {
		case msg := <-v.chatCh:
			v, _ = v.Update(msg)
		case <-deadline:
			t.Fatalf("chat still streaming %s after parent cancel (ctx not propagated?)",
				time.Since(start).Round(time.Millisecond))
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("chat stopped after %s, want ≤ 1s", elapsed.Round(time.Millisecond))
	}

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("server never observed cancellation of the chat request within 1s")
	}

	// Cancellation surfaces as an error, not a clean (fabricated) success,
	// and only the deltas streamed before the cancel were committed.
	if !strings.Contains(v.chatErr, "canceled") {
		t.Errorf("chatErr = %q, want a context-canceled error (no fabricated success)", v.chatErr)
	}
	var assistant []string
	for _, m := range v.history {
		if m.Role == ollama.RoleAssistant {
			assistant = append(assistant, m.Content)
		}
	}
	if len(assistant) != 1 || assistant[0] != "one" {
		t.Errorf("assistant history = %q, want only the pre-cancel delta %q", assistant, "one")
	}
	requireSettledGoroutines(t, baseline)
}

// TestModelsViewParentCancellationStopsPull: a pull stream mid-flight must
// stop within one second of parent cancellation, surface the error (never
// "pulled <name>"), and release its goroutines.
func TestModelsViewParentCancellationStopsPull(t *testing.T) {
	srv, entered, released := blockingServer(t, `{"status":"pulling manifest"}`+"\n")
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	v := newModelsView(parentCtx, ollama.New(srv.URL, ""), NewStyles("dark"), "dark")
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	v, _ = v.Update(modelsLoadedMsg{list: sampleModels()})
	v, _ = v.Update(tea.KeyPressMsg{Text: "p"})
	v, _ = v.Update(tea.KeyPressMsg{Text: "qwen3:big"})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.pulling {
		t.Fatal("enter: pull should start")
	}

	// Wait until the first progress event has provably reached the client
	// (mid-stream), then cancel the parent.
	select {
	case msg := <-v.pullCh:
		v, _ = v.Update(msg)
	case <-entered:
		select {
		case msg := <-v.pullCh:
			v, _ = v.Update(msg)
		case <-time.After(2 * time.Second):
			t.Fatal("first pull event never arrived after the request reached the server")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pull request never reached the server")
	}

	baseline := runtime.NumGoroutine()
	start := time.Now()
	cancel()

	deadline := time.After(time.Second)
	for v.pulling {
		select {
		case msg := <-v.pullCh:
			v, _ = v.Update(msg)
		case <-deadline:
			t.Fatalf("pull still running %s after parent cancel (ctx not propagated?)",
				time.Since(start).Round(time.Millisecond))
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("pull stopped after %s, want ≤ 1s", elapsed.Round(time.Millisecond))
	}

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("server never observed cancellation of the pull request within 1s")
	}

	if !strings.Contains(v.pullErr, "canceled") {
		t.Errorf("pullErr = %q, want a surfaced context-canceled error", v.pullErr)
	}
	if strings.Contains(v.notice, "pulled ") {
		t.Errorf("notice = %q, want no fabricated success for a canceled pull", v.notice)
	}
	requireSettledGoroutines(t, baseline)
}

// TestModelsViewParentCancellationStopsList: a finite list fetch parked on
// the server must return within one second of parent cancellation as an
// error (never a modelsLoadedMsg), leaving the view in the error state.
func TestModelsViewParentCancellationStopsList(t *testing.T) {
	srv, entered, released := blockingServer(t, "")
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	v := newModelsView(parentCtx, ollama.New(srv.URL, ""), NewStyles("dark"), "dark")
	cmd := v.loadCmd()

	resCh := make(chan tea.Msg, 1)
	go func() { resCh <- cmd() }()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("list request never reached the server")
	}

	baseline := runtime.NumGoroutine()
	start := time.Now()
	cancel()

	select {
	case msg := <-resCh:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("list command returned after %s, want ≤ 1s", elapsed.Round(time.Millisecond))
		}
		ev, ok := msg.(modelsEventMsg)
		if !ok {
			t.Fatalf("cmd() = %T after parent cancel, want a modelsEventMsg envelope (no fabricated list)", msg)
		}
		errMsg, ok := ev.msg.(modelsLoadErrMsg)
		if !ok {
			t.Fatalf("envelope payload = %T after parent cancel, want modelsLoadErrMsg (no fabricated list)", ev.msg)
		}
		if !strings.Contains(errMsg.err, "canceled") {
			t.Errorf("err = %q, want a context-canceled error", errMsg.err)
		}
		after, _ := v.Update(msg)
		if after.loading || after.listErr == "" || len(after.models) != 0 {
			t.Errorf("state after canceled load: loading=%v listErr=%q models=%d (want surfaced error, empty list)",
				after.loading, after.listErr, len(after.models))
		}
	case <-time.After(time.Second):
		t.Fatal("list command did not return within 1s of parent cancellation")
	}

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("server never observed cancellation of the list request within 1s")
	}
	requireSettledGoroutines(t, baseline)
}
