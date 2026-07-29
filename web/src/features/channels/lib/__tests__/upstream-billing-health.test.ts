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

import { getUpstreamBillingHealthPresentation } from '../upstream-billing-health'

describe('upstream billing account health presentation', () => {
  test('shows successful checks as healthy', () => {
    const presentation = getUpstreamBillingHealthPresentation('healthy')

    assert.equal(presentation.status, 'healthy')
    assert.equal(presentation.labelKey, 'Healthy')
    assert.equal(presentation.variant, 'outline')
  })

  test('shows failed checks as a destructive error', () => {
    const presentation = getUpstreamBillingHealthPresentation('error')

    assert.equal(presentation.status, 'error')
    assert.equal(presentation.labelKey, 'Error')
    assert.equal(presentation.variant, 'destructive')
  })

  test('treats missing and unrecognized states as not tested', () => {
    for (const status of [undefined, '', 'legacy']) {
      const presentation = getUpstreamBillingHealthPresentation(status)
      assert.equal(presentation.status, 'unknown')
      assert.equal(presentation.labelKey, 'Not tested')
      assert.equal(presentation.variant, 'secondary')
    }
  })
})
