// Package ollama is a thin typed wrapper over the Ollama REST API
// (PLAN.md §5). M1a ships the read-only surface: GET /api/tags (model list)
// and POST /api/show (model inspection). Mutation endpoints land with M1b.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// requestTimeout bounds every request. Show on a remote host can be slow;
// 30s is generous but still fails fast on a dead connection.
const requestTimeout = 30 * time.Second

// maxBodyBytes caps how much of a response we read, so a misbehaving host
// cannot balloon memory.
const maxBodyBytes = 64 << 20

// Client talks to one Ollama host. It is safe for concurrent use.
type Client struct {
	baseURL string // no trailing slash
	token   string // optional bearer token
	http    *http.Client
	stream  *http.Client // no request timeout: long pulls (context governs)

	// streamIdle is how long a streaming response may deliver no bytes
	// before the client aborts it (phase 5). Set to the 90s default by New;
	// tests inject short windows. Zero is treated as the default.
	streamIdle time.Duration

	// logger, when non-nil, receives the N5 debug-drawer transport traces:
	// one DEBUG line per request/response (method, path, status, duration,
	// body size — never request or response bodies, never headers) and WARN
	// lines for connection failures and error statuses. Nil (the default)
	// logs nothing, so every existing caller and test stays silent.
	logger *log.Logger
}

// New builds a client for an Ollama base URL (e.g. "http://localhost:11434").
// host is used verbatim modulo trailing slashes; token, when non-empty, is
// sent as a Bearer Authorization header. Both the finite and the streaming
// client refuse every HTTP redirect (M-08): a redirect would either point at
// a different origin that must not receive the bearer token or downgrade the
// scheme, and the initial non-loopback https/token validation in
// internal/config cannot see either case.
func New(host, token string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(host, "/"),
		token:      token,
		http:       &http.Client{Timeout: requestTimeout, CheckRedirect: redirectPolicy},
		stream:     &http.Client{CheckRedirect: redirectPolicy},
		streamIdle: streamIdleTimeout,
	}
}

// SetLogger attaches the shared debug logger (PLAN.md §12 N5). Call it once
// during wiring before the client serves traffic; a nil logger (the default)
// keeps the client silent.
func (c *Client) SetLogger(l *log.Logger) {
	c.logger = l
}

// logDebug emits one drawer trace line; a nil logger is a no-op.
func (c *Client) logDebug(msg string, kv ...any) {
	if c.logger != nil {
		c.logger.Debug(msg, kv...)
	}
}

// logWarn emits one connection-error line; a nil logger is a no-op.
func (c *Client) logWarn(msg string, kv ...any) {
	if c.logger != nil {
		c.logger.Warn(msg, kv...)
	}
}

// redirectPolicy is the CheckRedirect policy shared by the finite and the
// streaming client. Ollama serves its API from one origin, so a redirect
// means either a different origin (which must never receive the bearer token)
// or a scheme/port change on the same hostname — and Go 1.27 forwards the
// Authorization header whenever the redirect hostname equals the original or
// is a subdomain of it, regardless of scheme (so an https endpoint can leak
// the token to a plain http target). Returning an error makes http.Client
// abort the request before the redirect target is contacted and close the
// previous response body; the stable "refusing redirect" text lets callers
// and tests recognize the refusal. M-08.
func redirectPolicy(req *http.Request, _ []*http.Request) error {
	return fmt.Errorf("refusing redirect to %s: ollama API calls must not follow redirects", req.URL)
}

// do performs one JSON request and returns the raw response body for the
// caller to unmarshal. Non-2xx responses produce an error that includes the
// Ollama error message when the body carries one ({ "error": "..." }),
// and the HTTP status otherwise.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	start := time.Now()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		rd = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// N5 drawer: connection failures are the reconnect/error events —
		// the error string carries the transport cause (never the headers).
		c.logWarn("ollama request failed", "method", method, "path", path,
			"err", err.Error())
		return nil, fmt.Errorf("ollama %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		c.logWarn("ollama request body read failed", "method", method, "path", path,
			"err", err.Error())
		return nil, fmt.Errorf("ollama %s %s: read body: %w", method, path, err)
	}

	// N5 drawer: one trace line per completed request/response. Only the
	// shape (method, path, status, size, duration) is logged — bodies and
	// headers never are, and the shared sink redacts regardless.
	c.logDebug("ollama request", "method", method, "path", path,
		"status", resp.StatusCode, "bytes", len(raw),
		"duration_ms", time.Since(start).Milliseconds())

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		c.logWarn("ollama request error", "method", method, "path", path,
			"status", resp.StatusCode)
		return nil, apiError(method, path, resp.StatusCode, raw)
	}
	return raw, nil
}

// apiError builds an error for a non-2xx response, preferring the Ollama
// {"error": "..."} payload when present.
func apiError(method, path string, status int, raw []byte) error {
	var body struct {
		Error string `json:"error"`
	}
	msg := fmt.Sprintf("HTTP %d", status)
	if err := json.Unmarshal(raw, &body); err == nil && body.Error != "" {
		msg = body.Error
	}
	return fmt.Errorf("ollama %s %s: %s", method, path, msg)
}

// unmarshal decodes a response body into out, wrapping decode errors with
// endpoint context.
func unmarshal(method, path string, raw []byte, out any) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("ollama %s %s: decode response: %w", method, path, err)
	}
	return nil
}
