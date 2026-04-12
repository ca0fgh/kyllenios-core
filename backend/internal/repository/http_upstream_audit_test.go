package repository

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ca0fgh/hermes-proxy/internal/service"
	"github.com/stretchr/testify/require"
)

func TestApplyGatewayAuditHeaders_AddsCanaryHeadersFromContext(t *testing.T) {
	req := httptest.NewRequest("POST", "https://api.example.com/v1/responses", nil)
	ctx := service.WithGatewayAuditHeaders(context.Background(), map[string]string{
		"X-Hermes-Canary-Key": "canary-value",
	})
	req = req.WithContext(ctx)

	applyGatewayAuditHeaders(req)

	require.Equal(t, "canary-value", req.Header.Get("X-Hermes-Canary-Key"))
}
