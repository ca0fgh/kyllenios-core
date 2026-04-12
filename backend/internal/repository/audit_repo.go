package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ca0fgh/hermes-proxy/internal/service"
)

type auditRepository struct {
	db *sql.DB
}

func NewAuditRepository(db *sql.DB) service.AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) InsertAuditEvent(ctx context.Context, event *service.AuditEvent) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("audit repository unavailable")
	}
	if event == nil {
		return 0, fmt.Errorf("audit event is nil")
	}

	toolCallsJSON, _ := json.Marshal(event.ToolCalls)
	toolHashesJSON, _ := json.Marshal(event.ToolHashes)
	riskFlagsJSON, _ := json.Marshal(event.RiskFlags)
	canaryLabelsJSON, _ := json.Marshal(event.CanaryLabels)

	var id int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO audit_events (
	created_at, request_id, client_request_id, user_id, api_key_id, account_id, group_id,
	platform, request_type, method, path, inbound_endpoint, upstream_endpoint, upstream_target,
	status_code, requested_model, effective_model, upstream_model, user_agent, request_hash,
	response_hash, request_bytes, response_bytes, request_truncated, response_truncated,
	has_tool_calls, tool_count, tool_calls_json, tool_hashes_json, risk_flags_json, risk_level,
	canary_injected, canary_labels_json, alert_sent_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14,
	$15, $16, $17, $18, $19, $20,
	$21, $22, $23, $24, $25,
	$26, $27, $28::jsonb, $29::jsonb, $30::jsonb, $31,
	$32, $33::jsonb, $34
) RETURNING id
`,
		event.CreatedAt, event.RequestID, event.ClientRequestID, event.UserID, event.APIKeyID, event.AccountID, event.GroupID,
		event.Platform, int16(event.RequestType.Normalize()), event.Method, event.Path, event.InboundEndpoint, event.UpstreamEndpoint, event.UpstreamTarget,
		event.StatusCode, event.RequestedModel, event.EffectiveModel, event.UpstreamModel, event.UserAgent, event.RequestHash,
		event.ResponseHash, event.RequestBytes, event.ResponseBytes, event.RequestTruncated, event.ResponseTruncated,
		event.HasToolCalls, event.ToolCount, string(toolCallsJSON), string(toolHashesJSON), string(riskFlagsJSON), event.RiskLevel,
		event.CanaryInjected, string(canaryLabelsJSON), event.AlertSentAt,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *auditRepository) ListAuditEvents(ctx context.Context, filter *service.AuditEventFilter) (*service.AuditEventList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("audit repository unavailable")
	}
	if filter == nil {
		filter = &service.AuditEventFilter{}
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	where, args := buildAuditEventsWhere(filter)
	countSQL := "SELECT COUNT(*) FROM audit_events a" + where
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, `
SELECT
	id, created_at, request_id, client_request_id, user_id, api_key_id, account_id, group_id,
	platform, request_type, method, path, inbound_endpoint, upstream_endpoint, upstream_target,
	status_code, requested_model, effective_model, upstream_model, user_agent, request_hash,
	response_hash, request_bytes, response_bytes, request_truncated, response_truncated,
	has_tool_calls, tool_count, tool_calls_json, tool_hashes_json, risk_flags_json, risk_level,
	canary_injected, canary_labels_json, alert_sent_at
FROM audit_events a`+where+`
ORDER BY created_at DESC
LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*service.AuditEvent, 0, filter.PageSize)
	for rows.Next() {
		event, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &service.AuditEventList{
		Events:   out,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func (r *auditRepository) GetAuditEventByID(ctx context.Context, id int64) (*service.AuditEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("audit repository unavailable")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT
	id, created_at, request_id, client_request_id, user_id, api_key_id, account_id, group_id,
	platform, request_type, method, path, inbound_endpoint, upstream_endpoint, upstream_target,
	status_code, requested_model, effective_model, upstream_model, user_agent, request_hash,
	response_hash, request_bytes, response_bytes, request_truncated, response_truncated,
	has_tool_calls, tool_count, tool_calls_json, tool_hashes_json, risk_flags_json, risk_level,
	canary_injected, canary_labels_json, alert_sent_at
FROM audit_events
WHERE id = $1
`, id)
	return scanAuditEvent(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAuditEvent(scanner rowScanner) (*service.AuditEvent, error) {
	var (
		event            service.AuditEvent
		requestType      int16
		toolCallsJSON    []byte
		toolHashesJSON   []byte
		riskFlagsJSON    []byte
		canaryLabelsJSON []byte
		userID           sql.NullInt64
		apiKeyID         sql.NullInt64
		accountID        sql.NullInt64
		groupID          sql.NullInt64
		alertSentAt      sql.NullTime
	)
	if err := scanner.Scan(
		&event.ID, &event.CreatedAt, &event.RequestID, &event.ClientRequestID, &userID, &apiKeyID, &accountID, &groupID,
		&event.Platform, &requestType, &event.Method, &event.Path, &event.InboundEndpoint, &event.UpstreamEndpoint, &event.UpstreamTarget,
		&event.StatusCode, &event.RequestedModel, &event.EffectiveModel, &event.UpstreamModel, &event.UserAgent, &event.RequestHash,
		&event.ResponseHash, &event.RequestBytes, &event.ResponseBytes, &event.RequestTruncated, &event.ResponseTruncated,
		&event.HasToolCalls, &event.ToolCount, &toolCallsJSON, &toolHashesJSON, &riskFlagsJSON, &event.RiskLevel,
		&event.CanaryInjected, &canaryLabelsJSON, &alertSentAt,
	); err != nil {
		return nil, err
	}
	event.RequestType = service.RequestTypeFromInt16(requestType)
	if userID.Valid {
		v := userID.Int64
		event.UserID = &v
	}
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		event.APIKeyID = &v
	}
	if accountID.Valid {
		v := accountID.Int64
		event.AccountID = &v
	}
	if groupID.Valid {
		v := groupID.Int64
		event.GroupID = &v
	}
	if alertSentAt.Valid {
		v := alertSentAt.Time
		event.AlertSentAt = &v
	}
	_ = json.Unmarshal(toolCallsJSON, &event.ToolCalls)
	_ = json.Unmarshal(toolHashesJSON, &event.ToolHashes)
	_ = json.Unmarshal(riskFlagsJSON, &event.RiskFlags)
	_ = json.Unmarshal(canaryLabelsJSON, &event.CanaryLabels)
	return &event, nil
}

func buildAuditEventsWhere(filter *service.AuditEventFilter) (string, []any) {
	if filter == nil {
		return "", nil
	}

	clauses := make([]string, 0, 12)
	args := make([]any, 0, 12)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, clause+" $"+itoa(len(args)))
	}

	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		add("a.created_at >=", filter.StartTime.UTC())
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		add("a.created_at <=", filter.EndTime.UTC())
	}
	if v := strings.TrimSpace(filter.RequestID); v != "" {
		add("a.request_id =", v)
	}
	if v := strings.TrimSpace(filter.ClientRequestID); v != "" {
		add("a.client_request_id =", v)
	}
	if v := strings.TrimSpace(filter.Platform); v != "" {
		add("a.platform =", v)
	}
	if v := strings.TrimSpace(filter.Method); v != "" {
		add("a.method =", strings.ToUpper(v))
	}
	if v := strings.TrimSpace(filter.Path); v != "" {
		add("a.path =", v)
	}
	if v := strings.TrimSpace(filter.InboundEndpoint); v != "" {
		add("a.inbound_endpoint =", v)
	}
	if v := strings.TrimSpace(filter.RiskLevel); v != "" {
		add("a.risk_level =", v)
	}
	if filter.UserID != nil {
		add("a.user_id =", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add("a.api_key_id =", *filter.APIKeyID)
	}
	if filter.AccountID != nil {
		add("a.account_id =", *filter.AccountID)
	}
	if filter.GroupID != nil {
		add("a.group_id =", *filter.GroupID)
	}
	if filter.HasToolCalls != nil {
		add("a.has_tool_calls =", *filter.HasToolCalls)
	}
	if filter.CanaryInjected != nil {
		add("a.canary_injected =", *filter.CanaryInjected)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		args = append(args, "%"+q+"%")
		likeIdx := itoa(len(args))
		clauses = append(clauses, "(a.request_id ILIKE $"+likeIdx+" OR a.client_request_id ILIKE $"+likeIdx+" OR a.requested_model ILIKE $"+likeIdx+" OR a.effective_model ILIKE $"+likeIdx+" OR a.upstream_model ILIKE $"+likeIdx+" OR a.path ILIKE $"+likeIdx+")")
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
