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
}

// New builds a client for an Ollama base URL (e.g. "http://localhost:11434").
// host is used verbatim modulo trailing slashes; token, when non-empty, is
// sent as a Bearer Authorization header.
func New(host, token string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(host, "/"),
		token:      token,
		http:       &http.Client{Timeout: requestTimeout},
		stream:     &http.Client{},
		streamIdle: streamIdleTimeout,
	}
}

// do performs one JSON request and returns the raw response body for the
// caller to unmarshal. Non-2xx responses produce an error that includes the
// Ollama error message when the body carries one ({ "error": "..." }),
// and the HTTP status otherwise.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
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
		return nil, fmt.Errorf("ollama %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama %s %s: read body: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
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
