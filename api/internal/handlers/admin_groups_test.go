package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func TestUpdateGroupRequiresNameAndTwoAgents(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing name", body: `{"agent_ids":["a","b"]}`},
		{name: "one agent", body: `{"name":"ops","agent_ids":["a"]}`},
		{name: "empty roster", body: `{"name":"ops","agent_ids":[]}`},
	}

	h := NewAdminGroupsHandler(nil, nil, zap.NewNop())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/admin/groups/g1", strings.NewReader(tt.body))
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", "g1")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			rec := httptest.NewRecorder()

			h.UpdateGroup(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
