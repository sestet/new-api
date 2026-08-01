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

import { buildUsageFilterOptionsParams } from '../usage-filter-options.ts'

describe('buildUsageFilterOptionsParams', () => {
  test('uses the currently selected date range for filter option requests', () => {
    const todayStart = new Date('2026-07-31T00:00:00.000Z')
    const todayEnd = new Date('2026-07-31T23:59:59.000Z')
    const weekStart = new Date('2026-07-25T00:00:00.000Z')
    const thirtyDayStart = new Date('2026-07-02T00:00:00.000Z')

    assert.deepEqual(buildUsageFilterOptionsParams(todayStart, todayEnd), {
      start_timestamp: 1785456000,
      end_timestamp: 1785542399,
    })
    assert.deepEqual(buildUsageFilterOptionsParams(weekStart, todayEnd), {
      start_timestamp: 1784937600,
      end_timestamp: 1785542399,
    })
    assert.deepEqual(buildUsageFilterOptionsParams(thirtyDayStart, todayEnd), {
      start_timestamp: 1782950400,
      end_timestamp: 1785542399,
    })
  })
})
