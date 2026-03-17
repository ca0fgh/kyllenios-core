package dto

import (
	"testing"

	"github.com/ca0fgh/hermes-proxy/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogFromService_IncludesOpenAIWSMode(t *testing.T) {
	t.Parallel()

	wsLog := &service.UsageLog{
		RequestID:    "req_1",
		Model:        "gpt-5.3-codex",
		OpenAIWSMode: true,
	}
	httpLog := &service.UsageLog{
		RequestID:    "resp_1",
		Model:        "gpt-5.3-codex",
		OpenAIWSMode: false,
	}

	require.True(t, UsageLogFromService(wsLog).OpenAIWSMode)
	require.False(t, UsageLogFromService(httpLog).OpenAIWSMode)
	require.True(t, UsageLogFromServiceAdmin(wsLog).OpenAIWSMode)
	require.False(t, UsageLogFromServiceAdmin(httpLog).OpenAIWSMode)
}

func TestUsageLogFromService_PrefersRequestTypeForLegacyFields(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		RequestID:    "req_2",
		Model:        "gpt-5.3-codex",
		RequestType:  service.RequestTypeWSV2,
		Stream:       false,
		OpenAIWSMode: false,
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.Equal(t, "ws_v2", userDTO.RequestType)
	require.True(t, userDTO.Stream)
	require.True(t, userDTO.OpenAIWSMode)
	require.Equal(t, "ws_v2", adminDTO.RequestType)
	require.True(t, adminDTO.Stream)
	require.True(t, adminDTO.OpenAIWSMode)
}

func TestUsageCleanupTaskFromService_RequestTypeMapping(t *testing.T) {
	t.Parallel()

	requestType := int16(service.RequestTypeStream)
	task := &service.UsageCleanupTask{
		ID:     1,
		Status: service.UsageCleanupStatusPending,
		Filters: service.UsageCleanupFilters{
			RequestType: &requestType,
		},
	}

	dtoTask := UsageCleanupTaskFromService(task)
	require.NotNil(t, dtoTask)
	require.NotNil(t, dtoTask.Filters.RequestType)
	require.Equal(t, "stream", *dtoTask.Filters.RequestType)
}

func TestRequestTypeStringPtrNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, requestTypeStringPtr(nil))
}

func TestUsageLogFromService_IncludesServiceTierForUserAndAdmin(t *testing.T) {
	t.Parallel()

	serviceTier := "priority"
	inboundEndpoint := "/v1/chat/completions"
	upstreamEndpoint := "/v1/responses"
	log := &service.UsageLog{
		RequestID:             "req_3",
		Model:                 "gpt-5.4",
		ServiceTier:           &serviceTier,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		AccountRateMultiplier: f64Ptr(1.5),
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.NotNil(t, userDTO.ServiceTier)
	require.Equal(t, serviceTier, *userDTO.ServiceTier)
	require.NotNil(t, userDTO.InboundEndpoint)
	require.Equal(t, inboundEndpoint, *userDTO.InboundEndpoint)
	require.NotNil(t, userDTO.UpstreamEndpoint)
	require.Equal(t, upstreamEndpoint, *userDTO.UpstreamEndpoint)
	require.NotNil(t, adminDTO.ServiceTier)
	require.Equal(t, serviceTier, *adminDTO.ServiceTier)
	require.NotNil(t, adminDTO.InboundEndpoint)
	require.Equal(t, inboundEndpoint, *adminDTO.InboundEndpoint)
	require.NotNil(t, adminDTO.UpstreamEndpoint)
	require.Equal(t, upstreamEndpoint, *adminDTO.UpstreamEndpoint)
	require.NotNil(t, adminDTO.AccountRateMultiplier)
	require.InDelta(t, 1.5, *adminDTO.AccountRateMultiplier, 1e-12)
}

func TestAccountFromServiceShallow_UsesStoredQuotaWindowValues(t *testing.T) {
	t.Parallel()

	account := &service.Account{
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_daily_limit":       120.0,
			"quota_daily_used":        3.97,
			"quota_daily_start":       "2026-03-16T00:00:00Z",
			"quota_daily_reset_mode":  "fixed",
			"quota_daily_reset_hour":  0.0,
			"quota_reset_timezone":    "UTC",
			"quota_daily_reset_at":    "2026-03-17T00:00:00Z",
			"quota_weekly_limit":      840.0,
			"quota_weekly_used":       12.34,
			"quota_weekly_start":      "2026-03-16T00:00:00Z",
			"quota_weekly_reset_mode": "fixed",
			"quota_weekly_reset_day":  1.0,
			"quota_weekly_reset_hour": 0.0,
			"quota_weekly_reset_at":   "2026-03-23T00:00:00Z",
		},
	}

	dtoAccount := AccountFromServiceShallow(account)
	require.NotNil(t, dtoAccount)
	require.NotNil(t, dtoAccount.QuotaDailyUsed)
	require.InDelta(t, 3.97, *dtoAccount.QuotaDailyUsed, 1e-12)
	require.NotNil(t, dtoAccount.QuotaDailyResetAt)
	require.Equal(t, "2026-03-17T00:00:00Z", *dtoAccount.QuotaDailyResetAt)
	require.NotNil(t, dtoAccount.QuotaWeeklyUsed)
	require.InDelta(t, 12.34, *dtoAccount.QuotaWeeklyUsed, 1e-12)
	require.NotNil(t, dtoAccount.QuotaWeeklyResetAt)
	require.Equal(t, "2026-03-23T00:00:00Z", *dtoAccount.QuotaWeeklyResetAt)
}

func f64Ptr(value float64) *float64 {
	return &value
}
