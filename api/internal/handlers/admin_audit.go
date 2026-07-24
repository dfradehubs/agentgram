package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// AdminAuditHandler serves the admin audit log (detailed activity with content).
type AdminAuditHandler struct {
	repo   repository.AuditEventRepository
	logger *zap.Logger
}

// NewAdminAuditHandler creates a new admin audit handler.
func NewAdminAuditHandler(repo repository.AuditEventRepository, logger *zap.Logger) *AdminAuditHandler {
	return &AdminAuditHandler{repo: repo, logger: logger}
}

// ListAuditEvents handles GET /api/admin/audit with filters:
// from, to (RFC3339), user, group, resource_type, request_id, limit, offset.
func (h *AdminAuditHandler) ListAuditEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := models.AuditEventFilter{
		UserEmail:    q.Get("user"),
		Group:        q.Get("group"),
		ResourceType: q.Get("resource_type"),
		RequestID:    q.Get("request_id"),
		SessionID:    q.Get("session"),
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = &t
		}
	}
	f.Limit = 50
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		f.Limit = v
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 {
		f.Offset = v
	}

	events, total, err := h.repo.List(r.Context(), f)
	if err != nil {
		h.logger.Error("list audit events failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []*models.AuditEvent{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}
