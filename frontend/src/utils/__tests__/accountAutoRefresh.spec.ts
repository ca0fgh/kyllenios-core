import { describe, expect, it } from 'vitest'

import type { Account } from '@/types'
import { shouldReplaceAutoRefreshRow } from '@/utils/accountAutoRefresh'

const makeAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 1,
  name: 'acc',
  platform: 'openai',
  type: 'apikey',
  credentials: {},
  extra: {},
  concurrency: 10,
  priority: 50,
  rate_multiplier: 1,
  status: 'active',
  schedulable: true,
  created_at: '2026-03-16T00:00:00Z',
  updated_at: '2026-03-16T00:00:00Z',
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null,
  ...overrides
})

describe('shouldReplaceAutoRefreshRow', () => {
  it('returns true when daily quota usage changes', () => {
    const current = makeAccount({ quota_daily_limit: 120, quota_daily_used: 0 })
    const next = makeAccount({ quota_daily_limit: 120, quota_daily_used: 25.6133565 })

    expect(shouldReplaceAutoRefreshRow(current, next)).toBe(true)
  })

  it('returns true when weekly quota usage changes', () => {
    const current = makeAccount({ quota_weekly_limit: 300, quota_weekly_used: 0 })
    const next = makeAccount({ quota_weekly_limit: 300, quota_weekly_used: 88.123456 })

    expect(shouldReplaceAutoRefreshRow(current, next)).toBe(true)
  })

  it('returns true when total quota usage changes', () => {
    const current = makeAccount({ quota_limit: 120, quota_used: 0 })
    const next = makeAccount({ quota_limit: 120, quota_used: 52.203348 })

    expect(shouldReplaceAutoRefreshRow(current, next)).toBe(true)
  })
})
