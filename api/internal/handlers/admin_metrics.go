package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// AdminMetricsHandler handles admin metrics API endpoints
type AdminMetricsHandler struct {
	repo   repository.ChatEventRepository
	logger *zap.Logger
}

// NewAdminMetricsHandler creates a new admin metrics handler
func NewAdminMetricsHandler(repo repository.ChatEventRepository, logger *zap.Logger) *AdminMetricsHandler {
	return &AdminMetricsHandler{repo: repo, logger: logger}
}

// parseTimeRange extracts from/to from query params, defaulting to last 24h.
func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now()
	from := now.Add(-24 * time.Hour)
	to := now

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}

// parseInterval extracts the timeline interval, defaulting to "1h".
func parseInterval(r *http.Request) string {
	if v := r.URL.Query().Get("interval"); v != "" {
		return v
	}
	return "1h"
}

// parseLimit extracts the limit param, defaulting to the given value.
func parseLimit(r *http.Request, defaultLimit int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultLimit
}

// parseSource extracts the source filter ("web", "slack", or "" for all).
func parseSource(r *http.Request) string {
	return r.URL.Query().Get("source")
}

// Overview handles GET /api/admin/metrics/overview
func (h *AdminMetricsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	from, to := parseTimeRange(r)
	source := parseSource(r)
	stats, err := h.repo.GlobalStats(r.Context(), from, to, source)
	if err != nil {
		h.logger.Error("failed to get global stats", zap.Error(err))
		writeJSONError(w, "failed to get stats", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// OverviewTimeline handles GET /api/admin/metrics/overview/timeline
func (h *AdminMetricsHandler) OverviewTimeline(w http.ResponseWriter, r *http.Request) {
	from, to := parseTimeRange(r)
	interval := parseInterval(r)
	buckets, err := h.repo.GlobalTimeline(r.Context(), from, to, interval, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get global timeline", zap.Error(err))
		writeJSONError(w, "failed to get timeline", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buckets)
}

// TopResources handles GET /api/admin/metrics/overview/top
func (h *AdminMetricsHandler) TopResources(w http.ResponseWriter, r *http.Request) {
	from, to := parseTimeRange(r)
	limit := parseLimit(r, 10)
	resources, err := h.repo.TopResources(r.Context(), from, to, limit, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get top resources", zap.Error(err))
		writeJSONError(w, "failed to get top resources", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resources)
}

// OverviewUsers handles GET /api/admin/metrics/overview/users
func (h *AdminMetricsHandler) OverviewUsers(w http.ResponseWriter, r *http.Request) {
	from, to := parseTimeRange(r)
	limit := parseLimit(r, 20)
	users, err := h.repo.GlobalUsers(r.Context(), from, to, limit, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get global users", zap.Error(err))
		writeJSONError(w, "failed to get users", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// ResourceStats handles GET /api/admin/metrics/{type}/{id}
func (h *AdminMetricsHandler) ResourceStats(resourceType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID := chi.URLParam(r, "id")
		from, to := parseTimeRange(r)
		stats, err := h.repo.ResourceStats(r.Context(), resourceType, resourceID, from, to, parseSource(r))
		if err != nil {
			h.logger.Error("failed to get resource stats", zap.String("type", resourceType), zap.String("id", resourceID), zap.Error(err))
			writeJSONError(w, "failed to get stats", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// ResourceTimeline handles GET /api/admin/metrics/{type}/{id}/timeline
func (h *AdminMetricsHandler) ResourceTimeline(resourceType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID := chi.URLParam(r, "id")
		from, to := parseTimeRange(r)
		interval := parseInterval(r)
		buckets, err := h.repo.ResourceTimeline(r.Context(), resourceType, resourceID, from, to, interval, parseSource(r))
		if err != nil {
			h.logger.Error("failed to get resource timeline", zap.Error(err))
			writeJSONError(w, "failed to get timeline", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buckets)
	}
}

// ResourceUsers handles GET /api/admin/metrics/{type}/{id}/users
func (h *AdminMetricsHandler) ResourceUsers(resourceType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID := chi.URLParam(r, "id")
		from, to := parseTimeRange(r)
		limit := parseLimit(r, 20)
		users, err := h.repo.ResourceUsers(r.Context(), resourceType, resourceID, from, to, limit, parseSource(r))
		if err != nil {
			h.logger.Error("failed to get resource users", zap.Error(err))
			writeJSONError(w, "failed to get users", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}
}

// ResourceErrors handles GET /api/admin/metrics/{type}/{id}/errors
func (h *AdminMetricsHandler) ResourceErrors(resourceType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID := chi.URLParam(r, "id")
		from, to := parseTimeRange(r)
		errors, err := h.repo.ResourceErrors(r.Context(), resourceType, resourceID, from, to, parseSource(r))
		if err != nil {
			h.logger.Error("failed to get resource errors", zap.Error(err))
			writeJSONError(w, "failed to get errors", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(errors)
	}
}

// ResourceErrorEvents handles GET /api/admin/metrics/{type}/{id}/error-events
func (h *AdminMetricsHandler) ResourceErrorEvents(resourceType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resourceID := chi.URLParam(r, "id")
		from, to := parseTimeRange(r)
		limit := parseLimit(r, 50)
		events, err := h.repo.ResourceErrorEvents(r.Context(), resourceType, resourceID, from, to, limit, parseSource(r))
		if err != nil {
			h.logger.Error("failed to get resource error events", zap.Error(err))
			writeJSONError(w, "failed to get error events", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}
}

// OverviewErrors handles GET /api/admin/metrics/overview/errors
func (h *AdminMetricsHandler) OverviewErrors(w http.ResponseWriter, r *http.Request) {
	from, to := parseTimeRange(r)
	errors, err := h.repo.GlobalErrors(r.Context(), from, to, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get global errors", zap.Error(err))
		writeJSONError(w, "failed to get errors", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(errors)
}

// OverviewErrorEvents handles GET /api/admin/metrics/overview/error-events
func (h *AdminMetricsHandler) OverviewErrorEvents(w http.ResponseWriter, r *http.Request) {
	from, to := parseTimeRange(r)
	limit := parseLimit(r, 50)
	events, err := h.repo.GlobalErrorEvents(r.Context(), from, to, limit, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get global error events", zap.Error(err))
		writeJSONError(w, "failed to get error events", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// apiTypeToDBType converts URL resource type to DB resource type
func apiTypeToDBType(t string) string {
	switch t {
	case "agents":
		return "agent"
	case "mcp":
		return "mcp"
	default:
		return t
	}
}

// UserStats handles GET /api/admin/metrics/users/{email}
func (h *AdminMetricsHandler) UserStats(w http.ResponseWriter, r *http.Request) {
	email := chi.URLParam(r, "email")
	from, to := parseTimeRange(r)

	// Check for combined user+resource filter
	resourceType := r.URL.Query().Get("resource_type")
	resourceID := r.URL.Query().Get("resource_id")

	if resourceType != "" && resourceID != "" {
		stats, err := h.repo.UserResourceStats(r.Context(), email, apiTypeToDBType(resourceType), resourceID, from, to, parseSource(r))
		if err != nil {
			h.logger.Error("failed to get user resource stats", zap.String("email", email), zap.Error(err))
			writeJSONError(w, "failed to get stats", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
		return
	}

	stats, err := h.repo.UserStats(r.Context(), email, from, to, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get user stats", zap.String("email", email), zap.Error(err))
		writeJSONError(w, "failed to get stats", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// UserTimeline handles GET /api/admin/metrics/users/{email}/timeline
func (h *AdminMetricsHandler) UserTimeline(w http.ResponseWriter, r *http.Request) {
	email := chi.URLParam(r, "email")
	from, to := parseTimeRange(r)
	interval := parseInterval(r)

	resourceType := r.URL.Query().Get("resource_type")
	resourceID := r.URL.Query().Get("resource_id")

	if resourceType != "" && resourceID != "" {
		buckets, err := h.repo.UserResourceTimeline(r.Context(), email, apiTypeToDBType(resourceType), resourceID, from, to, interval, parseSource(r))
		if err != nil {
			h.logger.Error("failed to get user resource timeline", zap.Error(err))
			writeJSONError(w, "failed to get timeline", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buckets)
		return
	}

	buckets, err := h.repo.UserTimeline(r.Context(), email, from, to, interval, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get user timeline", zap.Error(err))
		writeJSONError(w, "failed to get timeline", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buckets)
}

// UserTopResources handles GET /api/admin/metrics/users/{email}/resources
func (h *AdminMetricsHandler) UserTopResources(w http.ResponseWriter, r *http.Request) {
	email := chi.URLParam(r, "email")
	from, to := parseTimeRange(r)
	limit := parseLimit(r, 10)
	resources, err := h.repo.UserTopResources(r.Context(), email, from, to, limit, parseSource(r))
	if err != nil {
		h.logger.Error("failed to get user top resources", zap.String("email", email), zap.Error(err))
		writeJSONError(w, "failed to get resources", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resources)
}
