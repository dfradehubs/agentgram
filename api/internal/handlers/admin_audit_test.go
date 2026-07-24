package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

type fakeAuditEventRepo struct {
	events    []*models.AuditEvent
	total     int
	gotFilter models.AuditEventFilter
}

func (f *fakeAuditEventRepo) Insert(context.Context, *models.AuditEvent) error { return nil }
func (f *fakeAuditEventRepo) List(_ context.Context, filter models.AuditEventFilter) ([]*models.AuditEvent, int, error) {
	f.gotFilter = filter
	return f.events, f.total, nil
}
func (f *fakeAuditEventRepo) Cleanup(context.Context, int) (int64, error) { return 0, nil }

func TestListAuditEventsParsesFiltersAndResponds(t *testing.T) {
	repo := &fakeAuditEventRepo{
		events: []*models.AuditEvent{{ID: "1", UserEmail: "a@b.com", ResourceType: "agent", Status: "ok"}},
		total:  1,
	}
	h := NewAdminAuditHandler(repo, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet,
		"/api/admin/audit?user=a@b.com&resource_type=agent&session=s1&group=g1&limit=25&offset=50&from=2026-01-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.ListAuditEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Events []*models.AuditEvent `json:"events"`
		Total  int                  `json:"total"`
		Limit  int                  `json:"limit"`
		Offset int                  `json:"offset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Total != 1 || len(body.Events) != 1 || body.Limit != 25 || body.Offset != 50 {
		t.Errorf("unexpected body: %+v", body)
	}

	f := repo.gotFilter
	if f.UserEmail != "a@b.com" || f.ResourceType != "agent" || f.SessionID != "s1" || f.Group != "g1" {
		t.Errorf("filter not parsed: %+v", f)
	}
	if f.Limit != 25 || f.Offset != 50 {
		t.Errorf("paging not parsed: limit=%d offset=%d", f.Limit, f.Offset)
	}
	if f.From == nil {
		t.Error("from timestamp not parsed")
	}
}

func TestListAuditEventsEmptyResult(t *testing.T) {
	h := NewAdminAuditHandler(&fakeAuditEventRepo{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	rec := httptest.NewRecorder()
	h.ListAuditEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// events must serialize as [] not null
	if !json.Valid(rec.Body.Bytes()) || !contains(rec.Body.String(), `"events":[]`) {
		t.Errorf("expected empty events array, got: %s", rec.Body.String())
	}
}
