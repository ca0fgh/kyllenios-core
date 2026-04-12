package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ca0fgh/hermes-proxy/internal/config"
	"github.com/ca0fgh/hermes-proxy/internal/service"
	"github.com/gin-gonic/gin"
)

func newOpsAuditTestRouter(handler *OpsHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/audit-events", handler.ListAuditEvents)
	r.GET("/audit-events/:id", handler.GetAuditEvent)
	return r
}

func TestOpsAuditHandler_ListUnavailable(t *testing.T) {
	h := NewOpsHandler(nil, nil)
	r := newOpsAuditTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/audit-events", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", w.Code)
	}
}

func TestOpsAuditHandler_ListDisabled(t *testing.T) {
	h := NewOpsHandler(nil, service.NewAuditService(nil, &config.Config{}))
	r := newOpsAuditTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/audit-events", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestOpsAuditHandler_ListInvalidBool(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.Audit.Enabled = true
	h := NewOpsHandler(nil, service.NewAuditService(stubAuditRepository{}, cfg))
	r := newOpsAuditTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/audit-events?has_tool_calls=bad", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestOpsAuditHandler_GetSuccess(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.Audit.Enabled = true
	h := NewOpsHandler(nil, service.NewAuditService(stubAuditRepository{}, cfg))
	r := newOpsAuditTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/audit-events/1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200, body=%s", w.Code, w.Body.String())
	}

	var resp responseEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("unexpected response payload: %+v", resp)
	}
}

type stubAuditRepository struct{}

func (stubAuditRepository) InsertAuditEvent(_ context.Context, _ *service.AuditEvent) (int64, error) {
	return 1, nil
}

func (stubAuditRepository) ListAuditEvents(_ context.Context, _ *service.AuditEventFilter) (*service.AuditEventList, error) {
	return &service.AuditEventList{
		Events:   []*service.AuditEvent{},
		Total:    0,
		Page:     1,
		PageSize: 50,
	}, nil
}

func (stubAuditRepository) GetAuditEventByID(_ context.Context, _ int64) (*service.AuditEvent, error) {
	return &service.AuditEvent{ID: 1, RiskLevel: "low"}, nil
}
