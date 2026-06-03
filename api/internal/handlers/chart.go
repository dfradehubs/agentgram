package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/chartextractor"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// ChartHandler handles chart extraction requests.
// It resolves the LLM model dynamically on each request so that
// models can be added/changed via admin UI without restarting the API.
type ChartHandler struct {
	llmRepo        repository.LLMModelRepository
	langfuseTracer *lf.Tracer
	logger         *zap.Logger
}

// NewChartHandler creates a new chart handler
func NewChartHandler(llmRepo repository.LLMModelRepository, lfTracer *lf.Tracer, logger *zap.Logger) *ChartHandler {
	return &ChartHandler{
		llmRepo:        llmRepo,
		langfuseTracer: lfTracer,
		logger:         logger,
	}
}

type chartExtractRequest struct {
	Data string `json:"data"`
}

type chartExtractResponse struct {
	Chart *chartextractor.ChartData `json:"chart"`
}

// Extract handles POST /api/chart/extract
func (h *ChartHandler) Extract(w http.ResponseWriter, r *http.Request) {
	// Resolve chart_extractor model dynamically from DB
	extractor := h.resolveExtractor(r)
	if extractor == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "chart extraction not configured"})
		return
	}

	var req chartExtractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.Data == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "data field is required"})
		return
	}

	const maxChartDataLen = 50_000
	if len(req.Data) > maxChartDataLen {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(map[string]string{"error": "data too large"})
		return
	}

	// Start Langfuse trace for chart extraction
	ctx := r.Context()
	var lfTrace *lf.Trace
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		userEmail := ""
		if claims := middleware.GetUserFromContext(ctx); claims != nil {
			userEmail = claims.GetEmail()
		}
		lfTrace = h.langfuseTracer.StartTrace(ctx, "chart-extract", userEmail, "", map[string]interface{}{
			"data_length": len(req.Data),
		})
		lfTrace.SetInput(truncate(req.Data, 500))
		ctx = lf.ContextWithTrace(ctx, lfTrace)
	}

	chart, err := extractor.Extract(ctx, req.Data)

	// End Langfuse trace
	if lfTrace != nil {
		if err != nil {
			lfTrace.End(false, err.Error())
		} else {
			lfTrace.End(true, chart)
		}
	}

	if err != nil {
		h.logger.Warn("chart extraction failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "extraction failed"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chartExtractResponse{Chart: chart})
}

// resolveExtractor looks up the chart_extractor model from the DB and creates
// an Extractor on the fly. This allows hot-reconfiguration via admin UI.
func (h *ChartHandler) resolveExtractor(r *http.Request) *chartextractor.Extractor {
	llmModels, err := h.llmRepo.ListByRole(r.Context(), "chart_extractor")
	if err != nil || len(llmModels) == 0 {
		return nil
	}
	model := llmModels[0]

	// Wrap with TracedProvider for Langfuse generation tracking
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		provider, provErr := llm.NewProvider(model)
		if provErr == nil {
			traced := lf.WrapProvider(provider, "chart-extractor", model.Model)
			return chartextractor.NewWithProvider(traced, h.logger)
		}
	}
	return chartextractor.New(model, h.logger)
}
