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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Progress } from '@/components/ui/progress'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatQuota, formatTimestampRelative } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ApiKey } from '../types'

type ApiKeyRateLimitsCellProps = {
  apiKey: ApiKey
  now: number
  locale?: string
  compact?: boolean
}

function progressColor(percentage: number): string {
  if (percentage >= 100) {
    return '[&_[data-slot=progress-indicator]]:bg-rose-500'
  }
  if (percentage >= 80) {
    return '[&_[data-slot=progress-indicator]]:bg-amber-500'
  }
  return '[&_[data-slot=progress-indicator]]:bg-emerald-500'
}

export function ApiKeyRateLimitsCell(props: ApiKeyRateLimitsCellProps) {
  const { t } = useTranslation()
  const windows = [
    {
      label: t('5h'),
      limit: props.apiKey.rate_limit_5h,
      usage: props.apiKey.usage_5h,
      resetAt: props.apiKey.reset_5h_at,
    },
    {
      label: t('1d'),
      limit: props.apiKey.rate_limit_1d,
      usage: props.apiKey.usage_1d,
      resetAt: props.apiKey.reset_1d_at,
    },
    {
      label: t('7d'),
      limit: props.apiKey.rate_limit_7d,
      usage: props.apiKey.usage_7d,
      resetAt: props.apiKey.reset_7d_at,
    },
  ].filter((window) => window.limit > 0)

  if (windows.length === 0) {
    return (
      <StatusBadge
        label={t('Unlimited')}
        variant='neutral'
        copyable={false}
        className='-ml-1.5'
      />
    )
  }

  return (
    <div
      className={cn('min-w-0 space-y-1.5', props.compact ? 'w-full' : 'w-52')}
    >
      {windows.map((window) => {
        const isExpired =
          window.resetAt > 0 && window.resetAt * 1000 <= props.now
        const effectiveUsage = isExpired ? 0 : window.usage
        const effectiveResetAt = isExpired ? 0 : window.resetAt
        const percentage = Math.min((effectiveUsage / window.limit) * 100, 100)
        const resetText = effectiveResetAt
          ? formatTimestampRelative(effectiveResetAt, 'seconds', props.locale)
          : t('Starts on first use')

        return (
          <Tooltip key={window.label}>
            <TooltipTrigger render={<div className='min-w-0 space-y-1' />}>
              <div className='flex min-w-0 items-center gap-2 text-[11px]'>
                <span className='text-muted-foreground w-5 shrink-0'>
                  {window.label}
                </span>
                <span className='min-w-0 flex-1 truncate text-right font-medium tabular-nums'>
                  {formatQuota(effectiveUsage)} / {formatQuota(window.limit)}
                </span>
              </div>
              <Progress
                value={percentage}
                className={cn('h-1', progressColor(percentage))}
              />
            </TooltipTrigger>
            <TooltipContent>
              <div className='space-y-1 text-xs'>
                <div>
                  {t('Used:')} {formatQuota(effectiveUsage)}
                </div>
                <div>
                  {t('Limit:')} {formatQuota(window.limit)}
                </div>
                <div>
                  {t('Resets:')} {resetText}
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        )
      })}
    </div>
  )
}
