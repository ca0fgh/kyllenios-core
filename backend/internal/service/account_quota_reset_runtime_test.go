package service

import (
	"testing"
	"time"
)

func TestFixedDailyQuotaPeriodExpiredHonorsResetAt(t *testing.T) {
	account := &Account{Extra: map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(9),
		"quota_reset_timezone":   "UTC",
		"quota_daily_reset_at":   time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
	}}
	periodStart := time.Now().Add(-1 * time.Minute)

	if !account.isFixedDailyPeriodExpired(periodStart) {
		t.Fatal("fixed daily quota period should expire when quota_daily_reset_at has passed")
	}
}

func TestFixedWeeklyQuotaPeriodExpiredHonorsResetAt(t *testing.T) {
	account := &Account{Extra: map[string]any{
		"quota_weekly_reset_mode": "fixed",
		"quota_weekly_reset_day":  float64(1),
		"quota_weekly_reset_hour": float64(9),
		"quota_reset_timezone":    "UTC",
		"quota_weekly_reset_at":   time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
	}}
	periodStart := time.Now().Add(-1 * time.Minute)

	if !account.isFixedWeeklyPeriodExpired(periodStart) {
		t.Fatal("fixed weekly quota period should expire when quota_weekly_reset_at has passed")
	}
}
