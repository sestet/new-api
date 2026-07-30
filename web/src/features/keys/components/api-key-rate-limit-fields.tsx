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
import { RotateCcw } from 'lucide-react'
import { useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { resetApiKeyRateLimitUsage } from '../api'
import type { ApiKeyFormValues } from '../lib'
import { useApiKeys } from './api-keys-provider'

const RATE_LIMIT_FIELDS = [
  { name: 'rate_limit_5h_dollars', labelKey: '5-hour limit' },
  { name: 'rate_limit_1d_dollars', labelKey: 'Daily limit' },
  { name: 'rate_limit_7d_dollars', labelKey: '7-day limit' },
] as const

type ApiKeyRateLimitFieldsProps = {
  form: UseFormReturn<ApiKeyFormValues>
  apiKeyId?: number
  currencyLabel: string
  tokensOnly: boolean
}

export function ApiKeyRateLimitFields(props: ApiKeyRateLimitFieldsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useApiKeys()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [isResetting, setIsResetting] = useState(false)

  const handleReset = async () => {
    if (!props.apiKeyId) return
    setIsResetting(true)
    try {
      const result = await resetApiKeyRateLimitUsage(props.apiKeyId)
      if (result.success) {
        toast.success(t('Time-based quota usage reset'))
        triggerRefresh()
        setConfirmOpen(false)
      } else {
        toast.error(result.message || t('Failed to reset quota usage'))
      }
    } catch {
      toast.error(t('Failed to reset quota usage'))
    } finally {
      setIsResetting(false)
    }
  }

  return (
    <div className='border-border/70 space-y-3 border-t pt-4'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0 space-y-0.5'>
          <div className='text-sm font-medium'>{t('Time-based limits')}</div>
          <p className='text-muted-foreground text-xs'>
            {t('Limits reset independently. Enter 0 for unlimited.')}
          </p>
        </div>
        {props.apiKeyId && (
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='shrink-0'
            onClick={() => setConfirmOpen(true)}
          >
            <RotateCcw className='size-3.5' />
            {t('Reset usage')}
          </Button>
        )}
      </div>

      <div className='grid gap-3 sm:grid-cols-3'>
        {RATE_LIMIT_FIELDS.map((config) => (
          <FormField
            key={config.name}
            control={props.form.control}
            name={config.name}
            render={({ field }) => (
              <FormItem className='min-w-0'>
                <FormLabel>{t(config.labelKey)}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type='number'
                    min={0}
                    step={props.tokensOnly ? 1 : 0.01}
                    onChange={(event) =>
                      field.onChange(Number.parseFloat(event.target.value) || 0)
                    }
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ))}
      </div>
      <FormDescription>
        {props.tokensOnly
          ? t('Usage is counted in quota units after final billing.')
          : t('Usage is counted in {{currency}} after final billing.', {
              currency: props.currencyLabel,
            })}
      </FormDescription>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Reset time-based quota usage?')}
        desc={t(
          'All active 5-hour, daily, and 7-day usage windows for this API key will be cleared.'
        )}
        confirmText={t('Reset usage')}
        handleConfirm={handleReset}
        isLoading={isResetting}
      />
    </div>
  )
}
