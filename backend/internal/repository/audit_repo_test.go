package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/ca0fgh/hermes-proxy/internal/service"
)

func TestBuildAuditEventsWhere_WithMultipleFilters(t *testing.T) {
	start := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	userID := int64(11)
	accountID := int64(22)
	hasToolCalls := true
	canaryInjected := true

	where, args := buildAuditEventsWhere(&service.AuditEventFilter{
		StartTime:       &start,
		EndTime:         &end,
		RequestID:       "req-1",
		ClientRequestID: "creq-1",
		Platform:        "openai",
		Path:            "/v1/responses",
		InboundEndpoint: "/v1/responses",
		RiskLevel:       "high",
		UserID:          &userID,
		AccountID:       &accountID,
		HasToolCalls:    &hasToolCalls,
		CanaryInjected:  &canaryInjected,
		Query:           "bash",
	})

	if where == "" {
		t.Fatal("expected non-empty WHERE clause")
	}
	if len(args) != 13 {
		t.Fatalf("args len=%d, want 13", len(args))
	}
	for _, expected := range []string{
		"a.request_id =",
		"a.client_request_id =",
		"a.platform =",
		"a.path =",
		"a.inbound_endpoint =",
		"a.risk_level =",
		"a.user_id =",
		"a.account_id =",
		"a.has_tool_calls =",
		"a.canary_injected =",
		"ILIKE",
	} {
		if !strings.Contains(where, expected) {
			t.Fatalf("where clause missing %q: %s", expected, where)
		}
	}
}
