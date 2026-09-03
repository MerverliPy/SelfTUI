package ollama

import (
	"context"
	"fmt"
	"net/http"
)

// Show fetches POST /api/show — full per-model inspection. This is a POST in
// the real API (PLAN.md's §5 table says GET; the evidence is the API itself
// and its docs — recorded in LEDGER).
func (c *Client) Show(ctx context.Context, name string) (Details, error) {
	const path = "/api/show"
	if name == "" {
		return Details{}, fmt.Errorf("ollama POST /api/show: empty model name")
	}
	raw, err := c.do(ctx, http.MethodPost, path, map[string]string{"name": name})
	if err != nil {
		return Details{}, err
	}

	var resp struct {
		License       string         `json:"license"`
		Modelfile     string         `json:"modelfile"`
		Parameters    string         `json:"parameters"`
		Template      string         `json:"template"`
		Capabilities  []string       `json:"capabilities"`
		ModelInfo     map[string]any `json:"model_info"`
		ProjectorInfo map[string]any `json:"projector_info"`
		Details       detailsBlock   `json:"details"`
	}
	if err := unmarshal(http.MethodPost, path, raw, &resp); err != nil {
		return Details{}, err
	}
	return Details{
		License:       resp.License,
		Modelfile:     resp.Modelfile,
		Parameters:    resp.Parameters,
		Template:      resp.Template,
		Capabilities:  resp.Capabilities,
		ModelInfo:     resp.ModelInfo,
		ProjectorInfo: resp.ProjectorInfo,
		Details:       resp.Details,
	}, nil
}
