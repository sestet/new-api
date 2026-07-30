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
import { test } from 'node:test'

import { apiKeySchema } from '../types.ts'

test('API key response preserves all time-based quota windows', () => {
  const apiKey = apiKeySchema.parse({
    id: 1,
    name: 'limited-key',
    key: 'abcd**********wxyz',
    status: 1,
    remain_quota: 900,
    used_quota: 100,
    unlimited_quota: false,
    expired_time: -1,
    created_time: 1000,
    accessed_time: 1100,
    group: 'default',
    model_limits_enabled: false,
    model_limits: '',
    allow_ips: '',
    rate_limit_5h: 100,
    rate_limit_1d: 200,
    rate_limit_7d: 300,
    usage_5h: 10,
    usage_1d: 20,
    usage_7d: 30,
    window_5h_start: 1000,
    window_1d_start: 1000,
    window_7d_start: 1000,
    reset_5h_at: 19_000,
    reset_1d_at: 87_400,
    reset_7d_at: 605_800,
  })

  assert.equal(apiKey.rate_limit_5h, 100)
  assert.equal(apiKey.usage_1d, 20)
  assert.equal(apiKey.reset_7d_at, 605_800)
})
