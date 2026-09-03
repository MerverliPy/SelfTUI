package ollama

import "time"

// Model is one entry from GET /api/tags — the domain model of PLAN.md §5.
type Model struct {
	Name          string
	ParameterSize string // e.g. "8.2B"
	Quantization  string // e.g. "Q4_K_M"
	Family        string // e.g. "qwen3"
	SizeBytes     int64
	ModifiedAt    time.Time
}

// Details is the POST /api/show payload (M1a inspect pane). Fields that can
// be absent on some models come back empty (license/template/modelfile).
type Details struct {
	License       string
	Modelfile     string
	Parameters    string
	Template      string
	Capabilities  []string
	ModelInfo     map[string]any // mixed value types (numbers, strings, bools)
	ProjectorInfo map[string]any
	Details       detailsBlock
}

// detailsBlock mirrors the nested "details" object present in both /api/tags
// items and /api/show responses.
type detailsBlock struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}
