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

import { getSubscriptionUsageBreakdown } from '../subscription-usage.ts'

describe('subscription usage breakdown', () => {
  test('separates request usage and debt offset from occupied quota', () => {
    const usage = getSubscriptionUsageBreakdown({
      amount_total: 1_000_000,
      amount_used: 900_000,
      debt_offset: 400_000,
    })

    assert.deepEqual(usage, {
      total: 1_000_000,
      used: 900_000,
      debtOffset: 400_000,
      requestUsage: 500_000,
      available: 100_000,
      percent: 90,
    })
  })

  test('does not show invalid debt data as negative request usage', () => {
    const usage = getSubscriptionUsageBreakdown({
      amount_total: 1000,
      amount_used: 200,
      debt_offset: 500,
    })

    assert.equal(usage.debtOffset, 200)
    assert.equal(usage.requestUsage, 0)
    assert.equal(usage.available, 800)
  })
})
