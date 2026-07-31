/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { UsageAnalyticsSummary } from '../../types'
import { getUsageAnalyticsSummary } from '../usage-analytics.ts'

const EMPTY_SUMMARY: UsageAnalyticsSummary = {
  request_count: 0,
  error_count: 0,
  refund_count: 0,
  token_count: 0,
  quota: 0,
  exact: 0,
  estimated: 0,
  pending: 0,
  failed: 0,
}

describe('getUsageAnalyticsSummary', () => {
  test('returns empty rates when the selected range has no requests', () => {
    assert.deepEqual(getUsageAnalyticsSummary(EMPTY_SUMMARY), {
      requestTotal: 0,
      trackedCount: 0,
      exactCoverage: null,
      successRate: null,
    })
  })

  test('derives success and exact billing rates from their own populations', () => {
    const summary: UsageAnalyticsSummary = {
      ...EMPTY_SUMMARY,
      request_count: 8,
      error_count: 2,
      exact: 6,
      estimated: 1,
      pending: 1,
    }

    assert.deepEqual(getUsageAnalyticsSummary(summary), {
      requestTotal: 10,
      trackedCount: 8,
      exactCoverage: 75,
      successRate: 80,
    })
  })
})
