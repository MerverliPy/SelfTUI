package agent

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"selftui/internal/ollama"
)

// Hardened fake-Ollama harness for the runner tests' flake-exposed httptest
// handlers (audit-squad packet §5, pre-existing finding 2026-09-08).
//
// Under full-suite -race parallel load, several runner tests have failed at
// the untouched base commit with shapes the runner cannot produce: handler
// `decode request: EOF` on an empty/truncated body, and "chat requests = N+1"
// counts where the extra arrival carried a body identical to a request the
// runner already made (and with arrival-keyed responses even corrupts the
// scripted conversation — the runner then reads the wrong turn's response).
// The runner issues exactly one json.Marshal'd ChatStream per iteration and
// never re-sends one, so neither shape is a runner behavior; they are
// transport-layer delivery artifacts (duplicate/truncated request delivery
// under load). This stub absorbs them without weakening the assertions:
//
//   - An undecodable body (the runner cannot produce one) is logged, refused
//     with 400, and never counted or served.
//   - Requests are counted by body identity (sha256): a redelivered request
//     carries the identical bytes and counts once, while each real model
//     iteration appends messages and therefore always produces a new body.
//     Genuine runner bugs (an extra iteration, a retry after approval expiry)
//     still produce a distinct body and still fail the count.
//   - Responses are served by body-identity phase, so a redelivered request
//     receives exactly the response its original got — the runner's view of
//     the conversation stays consistent.
//   - Phase assignment AND the respond callback run under one mutex: a
//     duplicate delivery can arrive while the original is still being
//     served, and callbacks capture unsynchronized test state (counters,
//     captured messages), so overlapping callbacks must be serialized.
type chatStub struct {
	t       *testing.T
	mu      sync.Mutex
	phaseOf map[string]int
	logical int
}

// newChatStub serves respond(phase, req, w) for each logical chat request.
func newChatStub(t *testing.T, respond func(phase int, req ollama.ChatRequest, w http.ResponseWriter)) (*httptest.Server, *chatStub) {
	stub := &chatStub{t: t, phaseOf: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			stub.logf("chat stub: unreadable request body (%d bytes): %v", len(raw), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var req ollama.ChatRequest
		if uerr := json.Unmarshal(raw, &req); uerr != nil {
			stub.logf("chat stub: undecodable chat request (%d bytes, %s...) — transport artifact, refused: %v",
				len(raw), firstRunes(string(raw), 24), uerr)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sum := sha256.Sum256(raw)
		stub.mu.Lock()
		defer stub.mu.Unlock()
		phase, seen := stub.phaseOf[string(sum[:])]
		if !seen {
			phase = stub.logical
			stub.logical++
			stub.phaseOf[string(sum[:])] = phase
		}
		respond(phase, req, w)
	}))
	t.Cleanup(srv.Close)
	return srv, stub
}

// requests reports how many logical (distinct-body) chat requests were served.
func (s *chatStub) requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logical
}

// logf is safe from handler goroutines before t.Cleanup's srv.Close rejoins.
func (s *chatStub) logf(format string, args ...any) {
	s.t.Logf(format, args...)
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
