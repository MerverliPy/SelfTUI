package ollama

// N5 debug-drawer transport traces (PLAN.md §12 N5): the client logs one
// DEBUG line per request/response and WARN lines for connection failures —
// shape only (method, path, status, size, duration), never bodies, never
// headers. These tests pin the traces and the nil-logger silence.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

// newBufLogger returns a charmbracelet logger writing plain entries to buf
// (debug level on, no caller report so assertions stay substring-simple).
func newBufLogger() (*log.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	l := log.NewWithOptions(&buf, log.Options{ReportCaller: false, Level: log.DebugLevel})
	return l, &buf
}

func TestRequestLoggingTracesShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(srv.Close)

	l, buf := newBufLogger()
	c := New(srv.URL, "sekrit")
	c.SetLogger(l)
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ollama request", `method=GET`, `path=/api/tags`, `status=200`} {
		if !strings.Contains(out, want) {
			t.Errorf("request trace missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sekrit") {
		t.Errorf("request trace leaked the token:\n%s", out)
	}
}

func TestRequestLoggingErrorStatusWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	t.Cleanup(srv.Close)

	l, buf := newBufLogger()
	c := New(srv.URL, "")
	c.SetLogger(l)
	if _, err := c.List(context.Background()); err == nil {
		t.Fatal("List: want an error for HTTP 500")
	}
	out := buf.String()
	if !strings.Contains(out, "ollama request error") || !strings.Contains(out, `status=500`) {
		t.Errorf("error-status warn missing:\n%s", out)
	}
}

func TestRequestLoggingConnectionFailureWarns(t *testing.T) {
	// A closed server's port is dead: the transport error must WARN.
	l, buf := newBufLogger()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := New(url, "")
	c.SetLogger(l)
	if _, err := c.List(context.Background()); err == nil {
		t.Fatal("List: want a transport error against a dead host")
	}
	if !strings.Contains(buf.String(), "ollama request failed") {
		t.Errorf("connection-failure warn missing:\n%s", buf.String())
	}
}

func TestStreamOpenTraceAndFailureWarn(t *testing.T) {
	// Success: one stream-open trace; failure (dead host): one warn.
	l, buf := newBufLogger()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Write([]byte(`{"message":{"content":"hi"},"done":true}` + "\n"))
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "")
	c.SetLogger(l)
	err := c.ChatStream(context.Background(), ChatRequest{Model: "m", Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}}}, func(ev ChatEvent) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if !strings.Contains(buf.String(), "ollama stream open") || !strings.Contains(buf.String(), `path=/api/chat`) {
		t.Errorf("stream-open trace missing:\n%s", buf.String())
	}

	l2, buf2 := newBufLogger()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := dead.URL
	dead.Close()
	c2 := New(url, "")
	c2.SetLogger(l2)
	if err := c2.ChatStream(context.Background(), ChatRequest{Model: "m", Messages: []ChatMessage{{Role: RoleUser, Content: "hi"}}}, func(ev ChatEvent) {}); err == nil {
		t.Fatal("ChatStream: want a transport error against a dead host")
	}
	if !strings.Contains(buf2.String(), "ollama stream failed") {
		t.Errorf("stream-failure warn missing:\n%s", buf2.String())
	}
}

func TestNilLoggerStaysSilentAndSafe(t *testing.T) {
	// The default (no SetLogger) and an explicit nil must both be no-ops —
	// every pre-N5 caller and test relies on that silence.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "")
	c.SetLogger(nil)
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List with nil logger: %v", err)
	}
}
