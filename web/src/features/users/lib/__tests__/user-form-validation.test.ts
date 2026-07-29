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

import { userFormSchema } from '../user-form-schema.ts'

describe('user form quota validation', () => {
  test('accepts a negative wallet balance created by subscription debt', () => {
    const result = userFormSchema.safeParse({
      username: 'subscription-user',
      quota_dollars: -0.001526,
    })

    assert.equal(result.success, true)
  })

  test('continues to accept zero and positive wallet balances', () => {
    assert.equal(
      userFormSchema.safeParse({ username: 'zero', quota_dollars: 0 }).success,
      true
    )
    assert.equal(
      userFormSchema.safeParse({ username: 'positive', quota_dollars: 1.25 })
        .success,
      true
    )
  })

  test('rejects non-finite wallet balances', () => {
    for (const quota of [
      Number.NaN,
      Number.POSITIVE_INFINITY,
      Number.NEGATIVE_INFINITY,
    ]) {
      assert.equal(
        userFormSchema.safeParse({ username: 'invalid', quota_dollars: quota })
          .success,
        false
      )
    }
  })
})
