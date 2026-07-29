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

import { getAdminBillingBreakdown } from '../admin-billing-breakdown.ts'

const doubledBill = {
  upstream_billing_status: 'exact',
  admin_info: {
    upstream_billing: {
      upstream_cost_quota: 1_193_175,
      charged_quota: 2_386_350,
      group_ratio: '2',
    },
  },
}

describe('admin billing breakdown', () => {
  test('shows the upstream cost and multiplier to administrators', () => {
    assert.deepEqual(getAdminBillingBreakdown(doubledBill, true), {
      upstreamCostQuota: 1_193_175,
      groupRatio: 2,
      chargedQuota: 2_386_350,
    })
  })

  test('does not expose the group multiplier to regular users', () => {
    assert.equal(getAdminBillingBreakdown(doubledBill, false), null)
  })

  test('keeps ordinary 1x billing rows compact', () => {
    const standardBill = structuredClone(doubledBill)
    standardBill.admin_info.upstream_billing.group_ratio = '1'

    assert.equal(getAdminBillingBreakdown(standardBill, true), null)
  })

  test('ignores incomplete or invalid billing snapshots', () => {
    assert.equal(
      getAdminBillingBreakdown(
        {
          admin_info: {
            upstream_billing: {
              upstream_cost_quota: 100,
              charged_quota: 200,
              group_ratio: 'invalid',
            },
          },
        },
        true
      ),
      null
    )
  })
})
