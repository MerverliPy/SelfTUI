package ollama

import (
	"context"
	"net/http"
	"time"
)

// List fetches GET /api/tags — the installed models in Ollama's own order.
func (c *Client) List(ctx context.Context) ([]Model, error) {
	const path = "/api/tags"
	raw, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Models []struct {
			Name       string       `json:"name"`
			Size       int64        `json:"size"`
			ModifiedAt string       `json:"modified_at"`
			Details    detailsBlock `json:"details"`
		} `json:"models"`
	}
	if err := unmarshal(http.MethodGet, path, raw, &resp); err != nil {
		return nil, err
	}

	models := make([]Model, 0, len(resp.Models))
	for _, m := range resp.Models {
		// modified_at is RFC3339Nano; resist malformed timestamps from
		// other hosts rather than failing the whole list.
		mod, err := time.Parse(time.RFC3339Nano, m.ModifiedAt)
		if err != nil {
			mod = time.Time{}
		}
		models = append(models, Model{
			Name:          m.Name,
			ParameterSize: m.Details.ParameterSize,
			Quantization:  m.Details.QuantizationLevel,
			Family:        m.Details.Family,
			SizeBytes:     m.Size,
			ModifiedAt:    mod,
		})
	}
	return models, nil
}
