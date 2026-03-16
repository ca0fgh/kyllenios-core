//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ca0fgh/kyllenios-core/internal/service"
)

func TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-" + uuid.NewString(),
		Name:   "billing",
		Quota:  1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:           requestID,
		APIKeyID:            apiKey.ID,
		UserID:              user.ID,
		AccountID:           account.ID,
		AccountType:         service.AccountTypeAPIKey,
		BalanceCost:         1.25,
		APIKeyQuotaCost:     1.25,
		APIKeyRateLimitCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.True(t, result1.Applied)
	require.True(t, result1.APIKeyQuotaExhausted)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = $1", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 1.25, quotaUsed, 0.000001)

	var usage5h float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT usage_5h FROM api_keys WHERE id = $1", apiKey.ID).Scan(&usage5h))
	require.InDelta(t, 1.25, usage5h, 0.000001)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM api_keys WHERE id = $1", apiKey.ID).Scan(&status))
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, status)

	var dedupCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&dedupCount))
	require.Equal(t, 1, dedupCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesSubscriptionBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-sub-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "usage-billing-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-usage-billing-sub-" + uuid.NewString(),
		Name:    "billing-sub",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:  user.ID,
		GroupID: group.ID,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        0,
		SubscriptionID:   &subscription.ID,
		SubscriptionCost: 2.5,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var dailyUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&dailyUsage))
	require.InDelta(t, 2.5, dailyUsage, 0.000001)
}

func TestUsageBillingRepositoryApply_RequestFingerprintConflict(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-conflict-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-conflict-" + uuid.NewString(),
		Name:   "billing-conflict",
	})

	requestID := uuid.NewString()
	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	})
	require.NoError(t, err)

	_, err = repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 2.50,
	})
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
}

func TestUsageBillingRepositoryApply_UpdatesAccountQuota(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-account-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-account-" + uuid.NewString(),
		Name:   "billing-account",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-quota-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 100.0,
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 3.5,
	})
	require.NoError(t, err)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE((extra->>'quota_used')::numeric, 0) FROM accounts WHERE id = $1", account.ID).Scan(&quotaUsed))
	require.InDelta(t, 3.5, quotaUsed, 0.000001)
}

func TestUsageBillingRepositoryApply_ResetsFixedDailyQuotaWhenBoundaryPassed(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-fixed-daily-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-fixed-daily-" + uuid.NewString(),
		Name:   "billing-fixed-daily",
	})

	now := time.Now().UTC().Truncate(time.Second)
	resetHour := float64((now.Hour() + 23) % 24)
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-fixed-daily-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_daily_limit":      120.0,
			"quota_daily_used":       3.03,
			"quota_daily_start":      now.Add(-2 * time.Hour).Format(time.RFC3339),
			"quota_daily_reset_mode": "fixed",
			"quota_daily_reset_hour": resetHour,
			"quota_reset_timezone":   "UTC",
			"quota_daily_reset_at":   now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 0.94,
	})
	require.NoError(t, err)

	var quotaDailyUsed float64
	var quotaDailyStart string
	var quotaDailyResetAt string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT
			COALESCE((extra->>'quota_daily_used')::numeric, 0),
			COALESCE(extra->>'quota_daily_start', ''),
			COALESCE(extra->>'quota_daily_reset_at', '')
		FROM accounts
		WHERE id = $1
	`, account.ID).Scan(&quotaDailyUsed, &quotaDailyStart, &quotaDailyResetAt))

	require.InDelta(t, 0.94, quotaDailyUsed, 0.000001)

	startAt, err := time.Parse(time.RFC3339, quotaDailyStart)
	require.NoError(t, err)
	require.WithinDuration(t, now, startAt, 5*time.Second)

	resetAt, err := time.Parse(time.RFC3339, quotaDailyResetAt)
	require.NoError(t, err)
	require.True(t, resetAt.After(now))
}

func TestUsageBillingRepositoryApply_RepairsStaleFixedDailyWindowEvenWhenResetAtIsFuture(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-stale-fixed-daily-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-stale-fixed-daily-" + uuid.NewString(),
		Name:   "billing-stale-fixed-daily",
	})

	now := time.Now().UTC().Truncate(time.Second)
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-stale-fixed-daily-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_daily_limit":      120.0,
			"quota_daily_used":       8.88,
			"quota_daily_start":      now.Add(-48 * time.Hour).Format(time.RFC3339),
			"quota_daily_reset_mode": "fixed",
			"quota_daily_reset_hour": 0.0,
			"quota_reset_timezone":   "UTC",
			"quota_daily_reset_at":   now.Add(12 * time.Hour).Format(time.RFC3339),
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 0.66,
	})
	require.NoError(t, err)

	var quotaDailyUsed float64
	var quotaDailyStart string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT
			COALESCE((extra->>'quota_daily_used')::numeric, 0),
			COALESCE(extra->>'quota_daily_start', '')
		FROM accounts
		WHERE id = $1
	`, account.ID).Scan(&quotaDailyUsed, &quotaDailyStart))

	require.InDelta(t, 0.66, quotaDailyUsed, 0.000001)
	expectedDailyStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	require.Equal(t, expectedDailyStart, quotaDailyStart)
}

func TestUsageBillingRepositoryApply_ResetsFixedWeeklyQuotaWhenBoundaryPassed(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-fixed-weekly-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-fixed-weekly-" + uuid.NewString(),
		Name:   "billing-fixed-weekly",
	})

	now := time.Now().UTC().Truncate(time.Second)
	resetHour := float64((now.Hour() + 23) % 24)
	resetDay := float64(now.Weekday())
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-fixed-weekly-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_weekly_limit":      300.0,
			"quota_weekly_used":       9.12,
			"quota_weekly_start":      now.Add(-2 * time.Hour).Format(time.RFC3339),
			"quota_weekly_reset_mode": "fixed",
			"quota_weekly_reset_day":  resetDay,
			"quota_weekly_reset_hour": resetHour,
			"quota_reset_timezone":    "UTC",
			"quota_weekly_reset_at":   now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 1.23,
	})
	require.NoError(t, err)

	var quotaWeeklyUsed float64
	var quotaWeeklyStart string
	var quotaWeeklyResetAt string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT
			COALESCE((extra->>'quota_weekly_used')::numeric, 0),
			COALESCE(extra->>'quota_weekly_start', ''),
			COALESCE(extra->>'quota_weekly_reset_at', '')
		FROM accounts
		WHERE id = $1
	`, account.ID).Scan(&quotaWeeklyUsed, &quotaWeeklyStart, &quotaWeeklyResetAt))

	require.InDelta(t, 1.23, quotaWeeklyUsed, 0.000001)

	startAt, err := time.Parse(time.RFC3339, quotaWeeklyStart)
	require.NoError(t, err)
	require.WithinDuration(t, now, startAt, 5*time.Second)

	resetAt, err := time.Parse(time.RFC3339, quotaWeeklyResetAt)
	require.NoError(t, err)
	require.True(t, resetAt.After(now))
}

func TestUsageBillingRepositoryApply_RepairsStaleFixedWeeklyWindowEvenWhenResetAtIsFuture(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-stale-fixed-weekly-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-stale-fixed-weekly-" + uuid.NewString(),
		Name:   "billing-stale-fixed-weekly",
	})

	now := time.Now().UTC().Truncate(time.Second)
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-stale-fixed-weekly-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_weekly_limit":      300.0,
			"quota_weekly_used":       18.18,
			"quota_weekly_start":      now.Add(-14 * 24 * time.Hour).Format(time.RFC3339),
			"quota_weekly_reset_mode": "fixed",
			"quota_weekly_reset_day":  1.0,
			"quota_weekly_reset_hour": 0.0,
			"quota_reset_timezone":    "UTC",
			"quota_weekly_reset_at":   now.Add(6 * 24 * time.Hour).Format(time.RFC3339),
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 1.11,
	})
	require.NoError(t, err)

	var quotaWeeklyUsed float64
	var quotaWeeklyStart string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT
			COALESCE((extra->>'quota_weekly_used')::numeric, 0),
			COALESCE(extra->>'quota_weekly_start', '')
		FROM accounts
		WHERE id = $1
	`, account.ID).Scan(&quotaWeeklyUsed, &quotaWeeklyStart))

	require.InDelta(t, 1.11, quotaWeeklyUsed, 0.000001)
	daysBack := (int(now.Weekday()) - 1 + 7) % 7
	expectedWeeklyStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -daysBack).
		Format(time.RFC3339)
	require.Equal(t, expectedWeeklyStart, quotaWeeklyStart)
}

func TestDashboardAggregationRepositoryCleanupUsageBillingDedup_BatchDeletesOldRows(t *testing.T) {
	ctx := context.Background()
	repo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	oldRequestID := "dedup-old-" + uuid.NewString()
	newRequestID := "dedup-new-" + uuid.NewString()
	oldCreatedAt := time.Now().UTC().AddDate(0, 0, -400)
	newCreatedAt := time.Now().UTC().Add(-time.Hour)

	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint, created_at)
		VALUES ($1, 1, $2, $3), ($4, 1, $5, $6)
	`,
		oldRequestID, strings.Repeat("a", 64), oldCreatedAt,
		newRequestID, strings.Repeat("b", 64), newCreatedAt,
	)
	require.NoError(t, err)

	require.NoError(t, repo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	var oldCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", oldRequestID).Scan(&oldCount))
	require.Equal(t, 0, oldCount)

	var newCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", newRequestID).Scan(&newCount))
	require.Equal(t, 1, newCount)

	var archivedCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup_archive WHERE request_id = $1", oldRequestID).Scan(&archivedCount))
	require.Equal(t, 1, archivedCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesAgainstArchivedKey(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	aggRepo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-archive-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-archive-" + uuid.NewString(),
		Name:   "billing-archive",
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE usage_billing_dedup
		SET created_at = $1
		WHERE request_id = $2 AND api_key_id = $3
	`, time.Now().UTC().AddDate(0, 0, -400), requestID, apiKey.ID)
	require.NoError(t, err)
	require.NoError(t, aggRepo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)
}
