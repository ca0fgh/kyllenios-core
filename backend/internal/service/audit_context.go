package service

import "context"

type gatewayAuditHeadersKey struct{}

// WithGatewayAuditHeaders stores sensitive canary headers in request context for upstream injection.
func WithGatewayAuditHeaders(ctx context.Context, headers map[string]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	cloned := make(map[string]string, len(headers))
	for k, v := range headers {
		if k == "" || v == "" {
			continue
		}
		cloned[k] = v
	}
	if len(cloned) == 0 {
		return ctx
	}
	return context.WithValue(ctx, gatewayAuditHeadersKey{}, cloned)
}

// GatewayAuditHeadersFromContext returns canary headers to inject into upstream requests.
func GatewayAuditHeadersFromContext(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(gatewayAuditHeadersKey{}).(map[string]string)
	if len(v) == 0 {
		return nil
	}
	return v
}
