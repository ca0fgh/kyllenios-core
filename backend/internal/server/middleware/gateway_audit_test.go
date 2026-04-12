package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ca0fgh/hermes-proxy/internal/config"
	"github.com/ca0fgh/hermes-proxy/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubAuditRecorder struct {
	inputs []*service.GatewayAuditCaptureInput
}

func (s *stubAuditRecorder) RecordGatewayAudit(_ context.Context, input *service.GatewayAuditCaptureInput) error {
	s.inputs = append(s.inputs, input)
	return nil
}

func TestGatewayAuditMiddleware_CapturesGatewayRequestAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Gateway.Audit.Enabled = true
	cfg.Gateway.Audit.RequestCaptureLimitBytes = 4096
	cfg.Gateway.Audit.ResponseCaptureLimitBytes = 4096
	cfg.Gateway.Audit.CanaryHeaders = map[string]string{
		"X-Hermes-Canary-Key": "canary-value",
	}

	recorder := &stubAuditRecorder{}

	r := gin.New()
	r.Use(GatewayAudit(recorder, cfg))
	r.POST("/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"output":[{"type":"function_call","name":"Bash","arguments":"{\"command\":\"echo hi\"}"}]}`)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, recorder.inputs, 1)
	require.Equal(t, "/v1/responses", recorder.inputs[0].Path)
	require.Equal(t, "POST", recorder.inputs[0].Method)
	require.Equal(t, `{"model":"gpt-5.4"}`, strings.TrimSpace(string(recorder.inputs[0].RequestBody)))
	require.NotEmpty(t, recorder.inputs[0].ResponseHash)
	require.True(t, recorder.inputs[0].CanaryInjected)
	require.Contains(t, recorder.inputs[0].CanaryLabels, "X-Hermes-Canary-Key")
}
