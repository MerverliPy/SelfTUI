package ollama

import (
	"context"
	"fmt"
	"net/http"
)

// Delete removes a model via DELETE /api/delete. The 200 response body is a
// trivial {"status":"success"} we ignore; failures (HTTP status, or the
// Ollama {"error": "..."} payload) surface as usual via the shared client
// (delete is quick, so the 30s request timeout applies).
func (c *Client) Delete(ctx context.Context, name string) error {
	const path = "/api/delete"
	if name == "" {
		return fmt.Errorf("ollama DELETE %s: empty model name", path)
	}
	_, err := c.do(ctx, http.MethodDelete, path, map[string]string{"name": name})
	return err
}
