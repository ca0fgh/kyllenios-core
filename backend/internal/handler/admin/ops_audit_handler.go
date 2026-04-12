package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ca0fgh/hermes-proxy/internal/pkg/response"
	"github.com/ca0fgh/hermes-proxy/internal/service"
	"github.com/gin-gonic/gin"
)

// ListAuditEvents returns gateway audit events.
// GET /api/v1/admin/ops/audit-events
func (h *OpsHandler) ListAuditEvents(c *gin.Context) {
	if h.auditService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Audit service not available")
		return
	}
	if err := h.auditService.RequireEnabled(); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	page, pageSize := response.ParsePagination(c)
	filter := &service.AuditEventFilter{
		Page:            page,
		PageSize:        pageSize,
		RequestID:       strings.TrimSpace(c.Query("request_id")),
		ClientRequestID: strings.TrimSpace(c.Query("client_request_id")),
		Platform:        strings.TrimSpace(c.Query("platform")),
		Method:          strings.TrimSpace(c.Query("method")),
		Path:            strings.TrimSpace(c.Query("path")),
		InboundEndpoint: strings.TrimSpace(c.Query("inbound_endpoint")),
		RiskLevel:       strings.TrimSpace(c.Query("risk_level")),
		Query:           strings.TrimSpace(c.Query("q")),
	}

	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if v := strings.TrimSpace(c.Query("api_key_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		filter.APIKeyID = &id
	}
	if v := strings.TrimSpace(c.Query("account_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
		}
		filter.AccountID = &id
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}
	if v := strings.TrimSpace(c.Query("has_tool_calls")); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			response.BadRequest(c, "Invalid has_tool_calls")
			return
		}
		filter.HasToolCalls = &parsed
	}
	if v := strings.TrimSpace(c.Query("canary_injected")); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			response.BadRequest(c, "Invalid canary_injected")
			return
		}
		filter.CanaryInjected = &parsed
	}

	result, err := h.auditService.ListAuditEvents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, result.Events, int64(result.Total), result.Page, result.PageSize)
}

// GetAuditEvent returns one audit event by id.
// GET /api/v1/admin/ops/audit-events/:id
func (h *OpsHandler) GetAuditEvent(c *gin.Context) {
	if h.auditService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Audit service not available")
		return
	}
	if err := h.auditService.RequireEnabled(); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid audit event id")
		return
	}
	event, err := h.auditService.GetAuditEventByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, event)
}
