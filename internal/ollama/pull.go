package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// PullProgress is one streamed event from POST /api/pull.
type PullProgress struct {
	Status    string // phase label, e.g. "pulling manifest", "verifying sha256 digest", "success"
	Digest    string // layer digest being downloaded ("" for non-layer phases)
	Total     int64  // current layer size in bytes (0 when the server doesn't report it)
	Completed int64  // bytes downloaded so far for the current layer
}

// Pull downloads a model via POST /api/pull. The response is a stream of
// JSON lines; each line becomes an onProgress callback (nil allowed) in
// arrival order. Ollama reports pull failures as {"error": "..."} lines with
// HTTP 200, so the stream itself is the error channel — a streamed error
// turns into the returned error.
//
// Pull is a long operation: unlike the other client methods it does not go
// through the shared request-timeout client. The caller's context is the only
// deadline (UI cancellation, Ctrl+C, or an explicit timeout), which matches
// how `do` already binds every request to its context.
func (c *Client) Pull(ctx context.Context, name string, onProgress func(PullProgress)) error {
	const path = "/api/pull"
	if name == "" {
		return fmt.Errorf("ollama POST %s: empty model name", path)
	}

	body, err := json.Marshal(map[string]any{"name": name, "stream": true})
	if err != nil {
		return fmt.Errorf("ollama POST %s: encode request: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ollama POST %s: build request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.stream.Do(req)
	if err != nil {
		return fmt.Errorf("ollama POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		return apiError(http.MethodPost, path, resp.StatusCode, raw)
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var ev struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("ollama POST %s: stream ended without success", path)
			}
			return fmt.Errorf("ollama POST %s: decode stream: %w", path, err)
		}
		if ev.Error != "" {
			return fmt.Errorf("ollama POST %s: %s", path, ev.Error)
		}
		if onProgress != nil {
			onProgress(PullProgress{
				Status:    ev.Status,
				Digest:    ev.Digest,
				Total:     ev.Total,
				Completed: ev.Completed,
			})
		}
		if ev.Status == "success" {
			return nil
		}
	}
}
