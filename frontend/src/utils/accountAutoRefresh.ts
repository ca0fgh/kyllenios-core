import type { Account } from '@/types'

export const shouldReplaceAutoRefreshRow = (current: Account, next: Account) => {
  return (
    current.updated_at !== next.updated_at ||
    current.current_concurrency !== next.current_concurrency ||
    current.current_window_cost !== next.current_window_cost ||
    current.active_sessions !== next.active_sessions ||
    current.schedulable !== next.schedulable ||
    current.status !== next.status ||
    current.rate_limit_reset_at !== next.rate_limit_reset_at ||
    current.overload_until !== next.overload_until ||
    current.temp_unschedulable_until !== next.temp_unschedulable_until ||
    current.quota_daily_used !== next.quota_daily_used ||
    current.quota_daily_limit !== next.quota_daily_limit ||
    current.quota_weekly_used !== next.quota_weekly_used ||
    current.quota_weekly_limit !== next.quota_weekly_limit ||
    current.quota_used !== next.quota_used ||
    current.quota_limit !== next.quota_limit
  )
}
