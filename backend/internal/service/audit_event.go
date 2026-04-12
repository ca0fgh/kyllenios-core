package service

import (
	"context"
	"time"
)

type AuditToolCall struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments,omitempty"`
}

type AuditEvent struct {
	ID                int64           `json:"id"`
	CreatedAt         time.Time       `json:"created_at"`
	RequestID         string          `json:"request_id"`
	ClientRequestID   string          `json:"client_request_id"`
	UserID            *int64          `json:"user_id,omitempty"`
	APIKeyID          *int64          `json:"api_key_id,omitempty"`
	AccountID         *int64          `json:"account_id,omitempty"`
	GroupID           *int64          `json:"group_id,omitempty"`
	Platform          string          `json:"platform"`
	RequestType       RequestType     `json:"request_type"`
	Method            string          `json:"method"`
	Path              string          `json:"path"`
	InboundEndpoint   string          `json:"inbound_endpoint,omitempty"`
	UpstreamEndpoint  string          `json:"upstream_endpoint,omitempty"`
	UpstreamTarget    string          `json:"upstream_target,omitempty"`
	StatusCode        int             `json:"status_code"`
	RequestedModel    string          `json:"requested_model,omitempty"`
	EffectiveModel    string          `json:"effective_model,omitempty"`
	UpstreamModel     string          `json:"upstream_model,omitempty"`
	UserAgent         string          `json:"user_agent,omitempty"`
	RequestHash       string          `json:"request_hash,omitempty"`
	ResponseHash      string          `json:"response_hash,omitempty"`
	RequestBytes      int             `json:"request_bytes"`
	ResponseBytes     int             `json:"response_bytes"`
	RequestTruncated  bool            `json:"request_truncated"`
	ResponseTruncated bool            `json:"response_truncated"`
	HasToolCalls      bool            `json:"has_tool_calls"`
	ToolCount         int             `json:"tool_count"`
	ToolCalls         []AuditToolCall `json:"tool_calls,omitempty"`
	ToolHashes        []string        `json:"tool_hashes,omitempty"`
	RiskFlags         []string        `json:"risk_flags,omitempty"`
	RiskLevel         string          `json:"risk_level"`
	CanaryInjected    bool            `json:"canary_injected"`
	CanaryLabels      []string        `json:"canary_labels,omitempty"`
	AlertSentAt       *time.Time      `json:"alert_sent_at,omitempty"`
}

type AuditEventFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	RequestID       string
	ClientRequestID string
	Platform        string
	Method          string
	Path            string
	InboundEndpoint string
	RiskLevel       string
	Query           string

	UserID         *int64
	APIKeyID       *int64
	AccountID      *int64
	GroupID        *int64
	HasToolCalls   *bool
	CanaryInjected *bool

	Page     int
	PageSize int
}

type AuditEventList struct {
	Events   []*AuditEvent
	Total    int
	Page     int
	PageSize int
}

type AuditRepository interface {
	InsertAuditEvent(ctx context.Context, event *AuditEvent) (int64, error)
	ListAuditEvents(ctx context.Context, filter *AuditEventFilter) (*AuditEventList, error)
	GetAuditEventByID(ctx context.Context, id int64) (*AuditEvent, error)
}

type AuditRecorder interface {
	RecordGatewayAudit(ctx context.Context, input *GatewayAuditCaptureInput) error
}

type GatewayAuditCaptureInput struct {
	RequestID         string
	ClientRequestID   string
	UserID            *int64
	APIKeyID          *int64
	AccountID         *int64
	GroupID           *int64
	Platform          string
	RequestType       RequestType
	Method            string
	Path              string
	InboundEndpoint   string
	UpstreamEndpoint  string
	UpstreamTarget    string
	StatusCode        int
	RequestedModel    string
	EffectiveModel    string
	UpstreamModel     string
	UserAgent         string
	RequestBody       []byte
	ResponseBody      []byte
	RequestHash       string
	ResponseHash      string
	RequestBytes      int
	ResponseBytes     int
	RequestTruncated  bool
	ResponseTruncated bool
	CanaryInjected    bool
	CanaryLabels      []string
}
