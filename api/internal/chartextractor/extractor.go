package chartextractor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// cleanJSONResponse strips markdown code fences (```json ... ```) that LLMs
// commonly wrap around JSON output.
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// ChartDataset represents a single dataset in a chart
type ChartDataset struct {
	Label string    `json:"label"`
	Data  []float64 `json:"data"`
	Color string    `json:"color,omitempty"`
}

// ChartOptions represents optional chart configuration
type ChartOptions struct {
	Stacked    bool `json:"stacked,omitempty"`
	Horizontal bool `json:"horizontal,omitempty"`
	ShowLegend bool `json:"showLegend,omitempty"`
}

// ChartData represents the chart schema returned by the extractor
type ChartData struct {
	ChartType   string         `json:"chartType"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	XAxisLabel  string         `json:"xAxisLabel,omitempty"`
	YAxisLabel  string         `json:"yAxisLabel,omitempty"`
	Labels      []string       `json:"labels"`
	Datasets    []ChartDataset `json:"datasets"`
	Options     *ChartOptions  `json:"options,omitempty"`
}

// Extractor extracts chart data from arbitrary content using an LLM
type Extractor struct {
	provider llm.Provider
	logger   *zap.Logger
}

// New creates a new Extractor if model is valid, otherwise returns nil
func New(model *models.LLMModel, logger *zap.Logger) *Extractor {
	if model == nil || !model.Enabled || model.APIKey == "" {
		return nil
	}
	provider, err := llm.NewProvider(model)
	if err != nil {
		logger.Warn("failed to create LLM provider for chart extractor", zap.Error(err))
		return nil
	}
	return &Extractor{
		provider: provider,
		logger:   logger,
	}
}

// NewWithProvider creates an Extractor with a pre-configured provider (e.g. traced).
func NewWithProvider(provider llm.Provider, logger *zap.Logger) *Extractor {
	if provider == nil {
		return nil
	}
	return &Extractor{
		provider: provider,
		logger:   logger,
	}
}

const maxDataLen = 50000 // ~12k tokens, generous for structured data

const extractPrompt = `Analyze the following data and extract information suitable for a chart visualization.

Rules:
1. Return a valid JSON object with the chart schema, or null if the data is not suitable for visualization.
2. Choose the best chartType: "bar" for categorical comparisons, "line" for time series or trends, "pie" for proportions/percentages, "area" for cumulative data.
3. Extract meaningful labels from the data.
4. Group numeric values into datasets with descriptive labels.
5. Keep the response concise — only the JSON object, no explanation.

Expected JSON schema:
{
  "chartType": "bar" | "line" | "pie" | "area",
  "title": "optional title",
  "labels": ["label1", "label2", ...],
  "datasets": [{"label": "Dataset Name", "data": [1, 2, 3, ...]}],
  "options": {"showLegend": true}
}

<data>
%s
</data>

JSON response:`

var validChartTypes = map[string]bool{"bar": true, "line": true, "pie": true, "area": true}

// Extract analyzes arbitrary data and returns chart schema or nil
func (e *Extractor) Extract(ctx context.Context, data string) (*ChartData, error) {
	if data == "" {
		return nil, nil
	}

	// Truncate oversized data to control cost and latency
	if len(data) > maxDataLen {
		data = data[:maxDataLen]
	}

	// Apply a timeout to the LLM call
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(extractPrompt, data)

	resp, err := e.provider.GenerateContent(ctx, &llm.Request{
		Messages:  []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens: 1024,
	})
	if err != nil {
		e.logger.Warn("chart extractor failed", zap.Error(err))
		return nil, err
	}

	text := cleanJSONResponse(resp.Text)
	if text == "" || text == "null" {
		return nil, nil
	}

	var chart ChartData
	if err := json.Unmarshal([]byte(text), &chart); err != nil {
		e.logger.Debug("chart extractor returned non-JSON", zap.String("text", text))
		return nil, nil
	}

	// Validate chartType
	if !validChartTypes[chart.ChartType] {
		e.logger.Debug("chart extractor returned invalid chartType", zap.String("chartType", chart.ChartType))
		return nil, nil
	}

	// Validate minimum requirements
	if len(chart.Labels) == 0 || len(chart.Datasets) == 0 {
		return nil, nil
	}

	return &chart, nil
}
