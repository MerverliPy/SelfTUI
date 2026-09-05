// Shared streaming protections for POST /api/chat and POST /api/pull
// (v0.1 hardening, phase 5): every NDJSON event is capped at maxEventBytes on
// the wire, and a stream that stops delivering bytes for streamIdleTimeout is
// aborted. There is deliberately no total stream deadline — a pull that keeps
// making progress, or a chat that keeps emitting deltas, may run far longer
// than the idle window, and caller cancellation always wins.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// Stream budgets (phase 5). Tests inject a short idle window per client.
const (
	// maxEventBytes caps one NDJSON event (raw JSON text between newlines).
	maxEventBytes = 4 << 20
	// streamIdleTimeout is how long a stream may deliver no bytes before the
	// client aborts it.
	streamIdleTimeout = 90 * time.Second
	// maxChatStreamBytes caps the cumulative raw NDJSON bytes a chat stream may
	// deliver. The ceiling counts complete event bytes — JSON framing,
	// content, thinking, and tool calls — so tool arguments cannot escape the
	// 16 MiB budget the way decoded content-only accounting would (H-03).
	maxChatStreamBytes = 16 << 20
)

// Stream decoder errors. The chat/pull loops wrap them with the endpoint
// path; callers and tests match on the stable text.
var (
	errEventTooLarge      = errors.New("stream event exceeds 4194304 bytes")
	errIdleTimeout        = errors.New("stream idle timeout")
	errChatStreamTooLarge = errors.New("chat stream exceeds 16777216 bytes")
)

// postStream issues one streaming POST on the no-timeout stream client. The
// request context is a child of ctx so the idle watchdog can abort a stalled
// connection without canceling the caller's context; caller cancellation
// still propagates through the child. The caller must call the returned
// cancel (typically deferred) so the request is released when the stream is
// abandoned without reaching EOF.
func (c *Client) postStream(ctx context.Context, path string, body []byte) (*http.Response, context.CancelFunc, error) {
	reqCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("ollama POST %s: build request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("ollama POST %s: %w", path, err)
	}
	return resp, cancel, nil
}

// idleReader wraps a streaming response body so that a read delivering no
// bytes for idle aborts the stream via the request context. Bytes arriving at
// any cadence keep the stream alive — there is no total deadline. A blocked
// read cannot be interrupted directly, so the watchdog cancels the request
// context (which makes net/http tear down the transport read) and turns the
// resulting error into the idle error.
type idleReader struct {
	r        io.Reader
	cancel   context.CancelFunc
	idle     time.Duration
	timedOut atomic.Bool
	errIdle  error // stable per-stream idle error
}

func newIdleReader(r io.Reader, cancel context.CancelFunc, idle time.Duration) *idleReader {
	if idle <= 0 {
		idle = streamIdleTimeout
	}
	return &idleReader{
		r:       r,
		cancel:  cancel,
		idle:    idle,
		errIdle: fmt.Errorf("%w: no bytes received for %s", errIdleTimeout, idle),
	}
}

// readResult carries one underlying read back to the waiting caller.
type readResult struct {
	n   int
	err error
}

// Read bounds the underlying read by the idle window. Once the watchdog has
// fired, every later Read returns the idle error immediately.
func (r *idleReader) Read(p []byte) (int, error) {
	if r.timedOut.Load() {
		return 0, r.errIdle
	}
	ch := make(chan readResult, 1)
	go func() {
		n, err := r.r.Read(p)
		ch <- readResult{n, err}
	}()
	timer := time.NewTimer(r.idle)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-timer.C:
		r.timedOut.Store(true)
		r.cancel() // abort the request: the transport tears down the blocked read
		<-ch       // wait for the read goroutine to finish before returning
		return 0, r.errIdle
	}
}

// ndjsonStream frames newline-delimited JSON events off a streaming response
// body. It combines the idle watchdog with the per-event size cap so both
// chat and pull share one bounded decode path.
type ndjsonStream struct {
	path string        // endpoint path, used to prefix errors
	br   *bufio.Reader // over the idle-watched body
}

// newNDJSONStream builds a stream decoder over body. idle is the idle window
// (zero means the 90s default); cancel aborts the underlying request when the
// stream idles out.
func newNDJSONStream(body io.Reader, cancel context.CancelFunc, idle time.Duration, path string) *ndjsonStream {
	// 32 KiB fill chunks keep the per-read watchdog goroutine cheap even on
	// multi-GiB pulls (progress lines are served from the buffer).
	ir := newIdleReader(body, cancel, idle)
	return &ndjsonStream{path: path, br: bufio.NewReaderSize(ir, 32<<10)}
}

// next returns the raw bytes of the next JSON event (delimiter stripped,
// blank lines skipped, trailing whitespace tolerated). io.EOF means the
// stream ended cleanly at an event boundary; the caller decides what a clean
// end means (a terminal done/success event). Every other error is
// endpoint-prefixed and terminal: the stream must not be read again.
func (s *ndjsonStream) next() ([]byte, error) {
	ev := make([]byte, 0, 1024)
	for {
		frag, err := s.br.ReadSlice('\n')
		if len(ev)+len(frag) > maxEventBytes {
			return nil, fmt.Errorf("ollama POST %s: %w", s.path, errEventTooLarge)
		}
		ev = append(ev, frag...)
		switch {
		case err == nil:
			ev = ev[:len(ev)-1] // drop the '\n' delimiter
			if len(bytes.TrimSpace(ev)) == 0 {
				ev = ev[:0] // blank line: skip like json.Decoder skips whitespace
				continue
			}
			return ev, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue // event longer than the buffer; keep accumulating up to the cap
		case errors.Is(err, io.EOF):
			if len(bytes.TrimSpace(ev)) == 0 {
				return nil, io.EOF
			}
			return ev, nil // final event without a trailing newline
		case errors.Is(err, errIdleTimeout):
			return nil, fmt.Errorf("ollama POST %s: %w", s.path, err)
		default:
			return nil, fmt.Errorf("ollama POST %s: decode stream: %w", s.path, err)
		}
	}
}
