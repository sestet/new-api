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

import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'

describe('upstream billing recheck settings', () => {
  test('persists automatic recheck controls in channel settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'billing-channel',
      key: 'sk-test',
      models: 'gpt-4o',
      upstream_billing_enabled: true,
      upstream_billing_recheck_enabled: false,
      upstream_billing_recheck_window_hours: 72,
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.upstream_billing.recheck_enabled, false)
    assert.equal(settings.upstream_billing.recheck_window_hours, 72)
  })

  test('loads existing recheck settings and defaults old channels to enabled', () => {
    const channel = channelSchema.parse({
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'billing-channel',
      created_time: 1,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      settings: JSON.stringify({
        upstream_billing: {
          enabled: true,
          recheck_enabled: false,
          recheck_window_hours: 168,
        },
      }),
    })

    const configured = transformChannelToFormDefaults(channel)
    assert.equal(configured.upstream_billing_recheck_enabled, false)
    assert.equal(configured.upstream_billing_recheck_window_hours, 168)

    const legacy = transformChannelToFormDefaults(
      channelSchema.parse({
        ...channel,
        settings: JSON.stringify({ upstream_billing: { enabled: true } }),
      })
    )
    assert.equal(legacy.upstream_billing_recheck_enabled, true)
    assert.equal(legacy.upstream_billing_recheck_window_hours, 24)
  })

  test('accepts a Sub2API refresh token without an access token', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'sub2-channel',
      key: 'sk-test',
      models: 'gpt-5.4',
      upstream_billing_enabled: true,
      upstream_billing_provider: 'sub2api',
      upstream_billing_refresh_token: 'refresh-token',
    })

    assert.equal(result.success, true)
  })

  test('auto mode accepts and persists an rt_ token as Sub2API refresh token', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'auto-sub2-channel',
      key: 'sk-test',
      models: 'gpt-5.6-luna',
      upstream_billing_enabled: true,
      upstream_billing_provider: 'auto' as const,
      upstream_billing_access_token: 'rt_refresh-token',
    }

    assert.equal(channelFormSchema.safeParse(formData).success, true)
    const payload = transformFormDataToCreatePayload(formData)
    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.upstream_billing.access_token, '')
    assert.equal(settings.upstream_billing.refresh_token, 'rt_refresh-token')
  })

  test('persists rotated Sub2API credentials and expiry metadata', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'sub2-channel',
      key: 'sk-test',
      models: 'gpt-5.4',
      upstream_billing_enabled: true,
      upstream_billing_provider: 'sub2api',
      upstream_billing_access_token: 'generated-access',
      upstream_billing_refresh_token: 'rotated-refresh',
      upstream_billing_access_token_issued_at: 1_700_000_000,
      upstream_billing_access_token_expires_at: 1_700_086_400,
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.upstream_billing.refresh_token, 'rotated-refresh')
    assert.equal(
      settings.upstream_billing.access_token_expires_at,
      1_700_086_400
    )
  })

  test('binds a shared account without copying account credentials into the channel', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'shared-billing-channel',
      key: 'sk-channel-key',
      models: 'claude-sonnet-4-5',
      upstream_billing_enabled: true,
      upstream_billing_credential_id: 42,
      upstream_billing_provider: 'sub2api' as const,
      upstream_billing_access_token: 'must-not-be-saved',
      upstream_billing_refresh_token: 'must-not-be-saved',
    }

    assert.equal(channelFormSchema.safeParse(formData).success, true)
    const payload = transformFormDataToCreatePayload(formData)
    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.upstream_billing.credential_id, 42)
    assert.equal(settings.upstream_billing.access_token, undefined)
    assert.equal(settings.upstream_billing.refresh_token, undefined)
    assert.equal(payload.channel.key, 'sk-channel-key')
  })

  test('enables exact billing whenever an upstream account is selected', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'shared-billing-channel',
      key: 'sk-channel-key',
      models: 'gpt-5.4',
      upstream_billing_enabled: false,
      upstream_billing_credential_id: 42,
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.upstream_billing.enabled, true)
    assert.equal(settings.upstream_billing.credential_id, 42)
  })

  test('unbinds the account and disables exact billing in channel settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'unbound-billing-channel',
      key: 'sk-channel-key',
      models: 'gpt-5.4',
      upstream_billing_enabled: false,
      upstream_billing_credential_id: 0,
    })

    const settings = JSON.parse(payload.channel.settings || '{}')
    assert.equal(settings.upstream_billing.enabled, false)
    assert.equal(settings.upstream_billing.credential_id, undefined)
  })
})
