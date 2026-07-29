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

import {
  getUpstreamBillingProviderLabel,
  getUpstreamBillingStatusPresentation,
} from '../upstream-billing-status.ts'

describe('upstream billing status presentation', () => {
  test('distinguishes exact, estimated, pending, and failed charges', () => {
    assert.deepEqual(getUpstreamBillingStatusPresentation('exact'), {
      labelKey: 'Exact',
      variant: 'success',
    })
    assert.deepEqual(getUpstreamBillingStatusPresentation('estimated'), {
      labelKey: 'Estimated',
      variant: 'warning',
    })
    assert.deepEqual(getUpstreamBillingStatusPresentation('pending'), {
      labelKey: 'Pending',
      variant: 'info',
    })
    assert.deepEqual(getUpstreamBillingStatusPresentation('failed'), {
      labelKey: 'Failed',
      variant: 'danger',
    })
  })

  test('omits the indicator for logs without precise billing enabled', () => {
    assert.equal(getUpstreamBillingStatusPresentation(undefined), null)
    assert.equal(getUpstreamBillingStatusPresentation('unknown'), null)
  })

  test('formats known providers while preserving future provider names', () => {
    assert.equal(getUpstreamBillingProviderLabel('new_api'), 'New API')
    assert.equal(getUpstreamBillingProviderLabel('sub2api'), 'Sub2API')
    assert.equal(getUpstreamBillingProviderLabel('custom'), 'custom')
    assert.equal(getUpstreamBillingProviderLabel(undefined), '')
  })
})
