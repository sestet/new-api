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
import test from 'node:test'

import {
  groupBillingSchema,
  parseGroupBillingDefaults,
  serializeGroupBillingValues,
} from '../group-billing-config.ts'

test('missing default group is restored with ratio one', () => {
  const values = parseGroupBillingDefaults({
    GroupRatio: '{"premium":2}',
    GroupGroupRatio: '{}',
    GroupSpecialUsableGroup: '{}',
    UserUsableGroups: '{"premium":"Premium"}',
  })

  assert.deepEqual(values.groups, [
    {
      name: 'default',
      description: 'default',
      ratio: 1,
      globallyUsable: true,
    },
    {
      name: 'premium',
      description: 'Premium',
      ratio: 2,
      globallyUsable: true,
    },
  ])
})

test('duplicate group names and override pairs are rejected', () => {
  const result = groupBillingSchema.safeParse({
    groups: [
      {
        name: 'default',
        description: 'Default',
        ratio: 1,
        globallyUsable: true,
      },
      {
        name: 'default',
        description: 'Duplicate',
        ratio: 2,
        globallyUsable: true,
      },
    ],
    overrides: [
      { userGroup: 'default', usingGroup: 'default', ratio: 2 },
      { userGroup: 'default', usingGroup: 'default', ratio: 3 },
    ],
    specialUsableRules: [],
  })

  assert.equal(result.success, false)
  assert.ok(
    result.error.issues.some(
      (issue) => issue.message === 'Group names must be unique'
    )
  )
  assert.ok(
    result.error.issues.some(
      (issue) => issue.message === 'Override pairs must be unique'
    )
  )
})

test('override rows serialize as nested user and using group map', () => {
  const request = serializeGroupBillingValues({
    groups: [
      {
        name: 'default',
        description: 'Default',
        ratio: 1,
        globallyUsable: true,
      },
      {
        name: 'premium',
        description: 'Premium',
        ratio: 1.5,
        globallyUsable: false,
      },
    ],
    overrides: [{ userGroup: 'default', usingGroup: 'premium', ratio: 2 }],
    specialUsableRules: [
      { userGroup: 'default', targetGroup: 'premium', visible: true },
      { userGroup: 'premium', targetGroup: 'default', visible: false },
    ],
  })

  assert.deepEqual(JSON.parse(request.group_ratio), {
    default: 1,
    premium: 1.5,
  })
  assert.deepEqual(JSON.parse(request.group_group_ratio), {
    default: { premium: 2 },
  })
  assert.deepEqual(JSON.parse(request.group_special_usable_group), {
    default: { '+:premium': 'Premium' },
    premium: { '-:default': '' },
  })
  assert.deepEqual(JSON.parse(request.user_usable_groups), {
    default: 'Default',
  })
})

test('legacy special usable rules parse prefixes and reject duplicate pairs', () => {
  const values = parseGroupBillingDefaults({
    GroupRatio: '{"default":1,"premium":2}',
    GroupGroupRatio: '{}',
    GroupSpecialUsableGroup: '{"default":{"premium":"Premium","-:default":""}}',
    UserUsableGroups: '{"default":"Default","premium":"Premium"}',
  })

  assert.deepEqual(values.specialUsableRules, [
    { userGroup: 'default', targetGroup: 'premium', visible: true },
    { userGroup: 'default', targetGroup: 'default', visible: false },
  ])

  const result = groupBillingSchema.safeParse({
    groups: [
      {
        name: 'default',
        description: 'Default',
        ratio: 1,
        globallyUsable: true,
      },
      {
        name: 'premium',
        description: 'Premium',
        ratio: 2,
        globallyUsable: false,
      },
    ],
    overrides: [],
    specialUsableRules: [
      { userGroup: 'default', targetGroup: 'premium', visible: true },
      { userGroup: 'default', targetGroup: 'premium', visible: false },
    ],
  })

  assert.equal(result.success, false)
  assert.ok(
    result.error.issues.some(
      (issue) => issue.message === 'Special usable group rules must be unique'
    )
  )
})
