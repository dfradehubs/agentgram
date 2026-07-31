package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/audit"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"github.com/dfradehubs/agentgram-api/internal/settings"
	"go.uber.org/zap"
)

// SurfaceMCPContextKey marks a request as coming from the MCP admin tools rather
// than the web admin. It travels in the request context, not a header, so an
// external client can't forge the surface recorded in the audit log.
const SurfaceMCPContextKey ContextKey = "surfaceMCP"

// WithSurfaceMCP tags ctx as an MCP-originated admin request.
func WithSurfaceMCP(ctx context.Context) context.Context {
	return context.WithValue(ctx, SurfaceMCPContextKey, true)
}

// adminAuditBodyLimit bounds how much of a request body is buffered for auditing.
// Admin payloads are small; the global BodyLimit already caps the request itself.
const adminAuditBodyLimit = 64 << 10

// AdminAudit records every mutating admin request in audit_events, so
// configuration changes show up in the audit panel next to chat activity.
// Mounted on the admin router, it covers the web admin and the MCP admin tools
// with one piece, including endpoints added later.
type AdminAudit struct {
	repo     repository.AuditEventRepository
	settings *settings.Service
	logger   *zap.Logger
}

// NewAdminAudit creates the middleware. A nil repo makes it a pass-through.
func NewAdminAudit(repo repository.AuditEventRepository, settingsService *settings.Service, logger *zap.Logger) *AdminAudit {
	return &AdminAudit{repo: repo, settings: settingsService, logger: logger}
}

// Handler returns the HTTP middleware.
func (a *AdminAudit) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.repo == nil || !isMutation(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Buffer the request body so the handler still gets to read it.
		reqBody, err := io.ReadAll(io.LimitReader(r.Body, adminAuditBodyLimit))
		if err != nil {
			reqBody = nil
		}
		r.Body = io.NopCloser(bytes.NewReader(reqBody))

		rec := &auditResponseRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)

		a.record(r, reqBody, rec, time.Since(start))
	})
}

func (a *AdminAudit) record(r *http.Request, reqBody []byte, rec *auditResponseRecorder, elapsed time.Duration) {
	source := models.AuditSourceWeb
	if mcp, ok := r.Context().Value(SurfaceMCPContextKey).(bool); ok && mcp {
		source = models.AuditSourceMCP
	}

	var email string
	var groups []string
	if claims := GetUserFromContext(r.Context()); claims != nil {
		email = claims.GetEmail()
		groups = claims.GetGroups()
	}

	respBody := rec.body.Bytes()
	status := "ok"
	var errMsg, errType string
	if rec.status < 200 || rec.status >= 300 {
		status = "error"
		errType = http.StatusText(rec.status)
		errMsg = strings.TrimSpace(string(security.RedactJSON(respBody)))
	}

	ev := &models.AuditEvent{
		UserEmail:    email,
		UserGroups:   groups,
		ResourceType: models.AuditResourceAdmin,
		ResourceID:   adminResourceID(r, reqBody, respBody),
		ResourceName: r.Method + " " + adminRoutePattern(r),
		Source:       source,
		Client:       r.UserAgent(),
		Action:       adminAction(r.Method),
		Prompt:       string(security.RedactJSON(reqBody)),
		Response:     string(security.RedactJSON(respBody)),
		Status:       status,
		ErrorType:    errType,
		ErrorMsg:     errMsg,
		DurationMs:   int(elapsed.Milliseconds()),
		CreatedAt:    time.Now(),
	}

	maxChars := 0
	if a.settings != nil {
		maxChars = a.settings.Int(settings.KeyAuditMaxContentChars)
	}
	audit.RecordEvent(a.repo, ev, maxChars, a.logger)
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func adminAction(method string) string {
	switch method {
	case http.MethodPost:
		return models.AuditActionAdminCreate
	case http.MethodDelete:
		return models.AuditActionAdminDelete
	default:
		return models.AuditActionAdminUpdate
	}
}

// adminResourceID builds "<section>[:<id>]" from the admin path, e.g.
// "agents:kube-agent", "mcp:audit-mcp", "settings". Section is the first path
// segment, so any admin endpoint added later is labelled without extra wiring.
// A create has no ID in its path, so it is read from the payload instead —
// otherwise the audit entry wouldn't say which resource was created.
func adminResourceID(r *http.Request, reqBody, respBody []byte) string {
	section := "unknown"
	if parts := strings.SplitN(strings.Trim(adminPath(r), "/"), "/", 2); parts[0] != "" {
		section = parts[0]
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		id = chi.URLParam(r, "email")
	}
	if id == "" {
		id = jsonID(respBody)
	}
	if id == "" {
		id = jsonID(reqBody)
	}
	if id == "" {
		return section
	}
	return section + ":" + id
}

// jsonID reads a top-level "id" string from a JSON document, if there is one.
func jsonID(raw []byte) string {
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return payload.ID
}

// adminRoutePattern returns the matched chi pattern when available (stable across
// IDs), falling back to the raw path.
func adminRoutePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return adminPath(r)
}

func adminPath(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePath != "" {
		return rctx.RoutePath
	}
	return r.URL.Path
}

// auditResponseRecorder captures the status and body while still writing through
// to the client. Admin responses are small JSON documents, so buffering is fine.
type auditResponseRecorder struct {
	http.ResponseWriter
	status      int
	body        bytes.Buffer
	wroteHeader bool
}

func (w *auditResponseRecorder) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseRecorder) Write(b []byte) (int, error) {
	if w.body.Len() < adminAuditBodyLimit {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}
