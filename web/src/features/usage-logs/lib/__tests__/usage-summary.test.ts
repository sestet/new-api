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

import { getUsageLogSummary } from '../usage-summary.ts'

describe('usage log reconciliation summary', () => {
  test('calculates coverage only from requests tracked by precise billing', () => {
    const summary = getUsageLogSummary({
      quota: 1200,
      rpm: 0,
      tpm: 0,
      request_count: 8,
      exact: 6,
      estimated: 1,
      pending: 1,
      failed: 0,
    })

    assert.equal(summary.tracked, 8)
    assert.equal(summary.waiting, 2)
    assert.equal(summary.coveragePercent, 75)
  })

  test('returns no coverage when precise billing is not enabled for the range', () => {
    const summary = getUsageLogSummary({
      quota: 500,
      rpm: 0,
      tpm: 0,
      request_count: 4,
      exact: 0,
      estimated: 0,
      pending: 0,
      failed: 0,
    })

    assert.equal(summary.tracked, 0)
    assert.equal(summary.waiting, 0)
    assert.equal(summary.coveragePercent, null)
  })
})
