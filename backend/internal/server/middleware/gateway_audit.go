package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"sort"
	"strings"

	"github.com/ca0fgh/hermes-proxy/internal/config"
	"github.com/ca0fgh/hermes-proxy/internal/pkg/ctxkey"
	"github.com/ca0fgh/hermes-proxy/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	auditOpsAccountIDKey     = "ops_account_id"
	auditOpsRequestTypeKey   = "ops_request_type"
	auditOpsUpstreamModelKey = "ops_upstream_model"
	auditInboundEndpointKey  = "_gateway_inbound_endpoint"
)

type gatewayAuditWriter struct {
	gin.ResponseWriter
	maxBytes  int
	buffer    bytes.Buffer
	truncated bool
	hash      hashAccumulator
}

type hashAccumulator struct {
	hash  hash.Hash
	bytes int
}

func (h *hashAccumulator) Write(p []byte) {
	if h.hash == nil {
		h.hash = sha256.New()
	}
	_, _ = h.hash.Write(p)
	h.bytes += len(p)
}

func (h *hashAccumulator) Hash() string {
	if h.hash == nil {
		sum := sha256.Sum256(nil)
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	return "sha256:" + hex.EncodeToString(h.hash.Sum(nil))
}

func (w *gatewayAuditWriter) Write(p []byte) (int, error) {
	w.hash.Write(p)
	if w.maxBytes > 0 && w.buffer.Len() < w.maxBytes {
		remaining := w.maxBytes - w.buffer.Len()
		if len(p) > remaining {
			_, _ = w.buffer.Write(p[:remaining])
			w.truncated = true
		} else {
			_, _ = w.buffer.Write(p)
		}
	} else if w.maxBytes > 0 {
		w.truncated = true
	}
	return w.ResponseWriter.Write(p)
}

func (w *gatewayAuditWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// GatewayAudit captures request/response summaries for gateway routes and passes them to the audit recorder.
func GatewayAudit(recorder service.AuditRecorder, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recorder == nil || cfg == nil || !cfg.Gateway.Audit.Enabled || c.Request == nil {
			c.Next()
			return
		}

		reqBody, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(reqBody))

		canaryHeaders := cfg.Gateway.Audit.CanaryHeaders
		canaryLabels := resolveAuditCanaryLabels(&cfg.Gateway.Audit)
		canaryInjected := len(canaryHeaders) > 0
		if canaryInjected {
			c.Request = c.Request.WithContext(service.WithGatewayAuditHeaders(c.Request.Context(), canaryHeaders))
		}

		originalWriter := c.Writer
		writer := &gatewayAuditWriter{
			ResponseWriter: c.Writer,
			maxBytes:       cfg.Gateway.Audit.ResponseCaptureLimitBytes,
		}
		c.Writer = writer
		c.Next()
		if c.Writer == writer {
			c.Writer = originalWriter
		}

		input := &service.GatewayAuditCaptureInput{
			RequestID:         firstNonEmpty(strings.TrimSpace(c.Writer.Header().Get("X-Request-ID")), ctxString(c, ctxkey.RequestID)),
			ClientRequestID:   ctxString(c, ctxkey.ClientRequestID),
			UserID:            ctxInt64Ptr(c, string(ContextKeyUser)),
			APIKeyID:          apiKeyIDFromContext(c),
			AccountID:         ctxInt64Ptr(c, auditOpsAccountIDKey),
			GroupID:           apiKeyGroupIDFromContext(c),
			Platform:          apiKeyPlatformFromContext(c),
			RequestType:       requestTypeFromContext(c),
			Method:            strings.TrimSpace(c.Request.Method),
			Path:              strings.TrimSpace(c.Request.URL.Path),
			InboundEndpoint:   firstNonEmpty(ginString(c, auditInboundEndpointKey), strings.TrimSpace(c.Request.URL.Path)),
			UpstreamModel:     ginString(c, auditOpsUpstreamModelKey),
			RequestedModel:    strings.TrimSpace(gjson.GetBytes(reqBody, "model").String()),
			EffectiveModel:    ginString(c, "ops_model"),
			StatusCode:        c.Writer.Status(),
			UserAgent:         strings.TrimSpace(c.GetHeader("User-Agent")),
			RequestBody:       truncateBytes(reqBody, cfg.Gateway.Audit.RequestCaptureLimitBytes),
			ResponseBody:      writer.buffer.Bytes(),
			RequestHash:       hashBytes(reqBody),
			ResponseHash:      writer.hash.Hash(),
			RequestBytes:      len(reqBody),
			ResponseBytes:     writer.hash.bytes,
			RequestTruncated:  cfg.Gateway.Audit.RequestCaptureLimitBytes > 0 && len(reqBody) > cfg.Gateway.Audit.RequestCaptureLimitBytes,
			ResponseTruncated: writer.truncated,
			CanaryInjected:    canaryInjected,
			CanaryLabels:      canaryLabels,
		}
		_ = recorder.RecordGatewayAudit(c.Request.Context(), input)
	}
}

func resolveAuditCanaryLabels(cfg *config.GatewayAuditConfig) []string {
	if cfg == nil {
		return nil
	}
	if len(cfg.CanaryLabels) > 0 {
		return append([]string(nil), cfg.CanaryLabels...)
	}
	if len(cfg.CanaryHeaders) == 0 {
		return nil
	}
	labels := make([]string, 0, len(cfg.CanaryHeaders))
	for key := range cfg.CanaryHeaders {
		if strings.TrimSpace(key) != "" {
			labels = append(labels, key)
		}
	}
	sort.Strings(labels)
	return labels
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func truncateBytes(raw []byte, limit int) []byte {
	if limit <= 0 || len(raw) <= limit {
		return append([]byte(nil), raw...)
	}
	return append([]byte(nil), raw[:limit]...)
}

func ctxString(c *gin.Context, key any) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if v, ok := c.Request.Context().Value(key).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func ctxInt64Ptr(c *gin.Context, key string) *int64 {
	if c == nil {
		return nil
	}
	v, ok := c.Get(key)
	if !ok {
		return nil
	}
	switch value := v.(type) {
	case int64:
		if value <= 0 {
			return nil
		}
		out := value
		return &out
	case AuthSubject:
		if value.UserID <= 0 {
			return nil
		}
		out := value.UserID
		return &out
	default:
		return nil
	}
}

func ginString(c *gin.Context, key string) string {
	if c == nil {
		return ""
	}
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	if out, ok := v.(string); ok {
		return strings.TrimSpace(out)
	}
	return ""
}

func apiKeyIDFromContext(c *gin.Context) *int64 {
	apiKey, ok := GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.ID <= 0 {
		return nil
	}
	out := apiKey.ID
	return &out
}

func apiKeyGroupIDFromContext(c *gin.Context) *int64 {
	apiKey, ok := GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.GroupID == nil {
		return nil
	}
	out := *apiKey.GroupID
	return &out
}

func apiKeyPlatformFromContext(c *gin.Context) string {
	apiKey, ok := GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return strings.TrimSpace(apiKey.Group.Platform)
}

func requestTypeFromContext(c *gin.Context) service.RequestType {
	if c == nil {
		return service.RequestTypeUnknown
	}
	v, ok := c.Get(auditOpsRequestTypeKey)
	if !ok {
		return service.RequestTypeUnknown
	}
	switch value := v.(type) {
	case int16:
		return service.RequestTypeFromInt16(value)
	case service.RequestType:
		return value.Normalize()
	default:
		return service.RequestTypeUnknown
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
