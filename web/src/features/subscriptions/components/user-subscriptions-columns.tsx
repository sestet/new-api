/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Progress } from '@/components/ui/progress'
import { formatQuota } from '@/lib/format'

import { formatResetPeriod, formatTimestamp } from '../lib'
import { getSubscriptionUsageBreakdown } from '../lib/subscription-usage'
import type { AdminUserSubscription } from '../types'
import { UserSubscriptionRowActions } from './user-subscription-row-actions'

export function useUserSubscriptionsColumns(): ColumnDef<AdminUserSubscription>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<AdminUserSubscription>[] => [
      {
        accessorKey: 'user_id',
        id: 'user_id',
        header: t('User'),
        meta: { mobileTitle: true },
        cell: ({ row }) => {
          const subscription = row.original
          return (
            <div className='min-w-0'>
              <div className='flex items-center gap-1.5'>
                <span className='truncate font-medium'>
                  {subscription.username}
                </span>
                <TableId value={subscription.user_id} />
              </div>
              <div className='text-muted-foreground max-w-52 truncate text-xs'>
                {subscription.display_name || subscription.email || '-'}
              </div>
            </div>
          )
        },
        size: 190,
      },
      {
        accessorKey: 'plan_id',
        id: 'plan_id',
        header: t('Plan'),
        cell: ({ row }) => (
          <div className='min-w-0'>
            <div className='max-w-44 truncate font-medium'>
              {row.original.plan_title || `#${row.original.plan_id}`}
            </div>
            <div className='text-muted-foreground text-xs'>
              ID: {row.original.plan_id}
            </div>
          </div>
        ),
        size: 160,
      },
      {
        accessorKey: 'status',
        id: 'status',
        header: t('Status'),
        meta: { mobileBadge: true },
        cell: ({ row }) => {
          if (row.original.status === 'active') {
            return (
              <StatusBadge
                label={t('Active')}
                variant='success'
                copyable={false}
              />
            )
          }
          if (row.original.status === 'cancelled') {
            return (
              <StatusBadge
                label={t('Cancelled')}
                variant='neutral'
                copyable={false}
              />
            )
          }
          return (
            <StatusBadge
              label={t('Expired')}
              variant='warning'
              copyable={false}
            />
          )
        },
        size: 90,
      },
      {
        id: 'quota',
        header: t('Subscription usage'),
        cell: ({ row }) => {
          const usage = getSubscriptionUsageBreakdown(row.original)
          if (usage.total === 0) {
            return (
              <div className='grid grid-cols-2 gap-x-4 gap-y-1 text-xs'>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Request usage')}:
                  </span>{' '}
                  {formatQuota(usage.requestUsage)}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Debt offset')}:
                  </span>{' '}
                  {formatQuota(usage.debtOffset)}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Available quota')}:
                  </span>{' '}
                  {t('Unlimited')}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Period quota')}:
                  </span>{' '}
                  {t('Unlimited')}
                </div>
              </div>
            )
          }
          return (
            <div className='w-64 max-w-full space-y-1.5'>
              <div className='grid grid-cols-2 gap-x-4 gap-y-1 text-xs'>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Request usage')}:
                  </span>{' '}
                  {formatQuota(usage.requestUsage)}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Debt offset')}:
                  </span>{' '}
                  {formatQuota(usage.debtOffset)}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Available quota')}:
                  </span>{' '}
                  {formatQuota(usage.available)}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Period quota')}:
                  </span>{' '}
                  {formatQuota(usage.total)}
                </div>
              </div>
              <Progress
                value={usage.percent}
                aria-label={t('Subscription usage')}
              />
            </div>
          )
        },
        size: 285,
      },
      {
        id: 'reset',
        header: t('Quota Reset'),
        cell: ({ row }) => {
          const subscription = row.original
          const resetPeriod = formatResetPeriod(
            {
              quota_reset_period: subscription.quota_reset_period,
              quota_reset_custom_seconds:
                subscription.quota_reset_custom_seconds,
            },
            t
          )
          return (
            <div className='text-xs leading-5 tabular-nums'>
              <div className='font-medium'>{resetPeriod}</div>
              <div>
                <span className='text-muted-foreground'>
                  {t('Last reset')}:
                </span>{' '}
                {formatTimestamp(subscription.last_reset_time)}
              </div>
              <div>
                <span className='text-muted-foreground'>
                  {t('Next reset')}:
                </span>{' '}
                {formatTimestamp(subscription.next_reset_time)}
              </div>
            </div>
          )
        },
        size: 190,
      },
      {
        id: 'validity',
        header: t('Validity'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <div className='text-xs leading-5 tabular-nums'>
            <div>
              <span className='text-muted-foreground'>{t('Start')}:</span>{' '}
              {formatTimestamp(row.original.start_time)}
            </div>
            <div>
              <span className='text-muted-foreground'>{t('End')}:</span>{' '}
              {formatTimestamp(row.original.end_time)}
            </div>
          </div>
        ),
        size: 175,
      },
      {
        accessorKey: 'source',
        id: 'source',
        header: t('Source'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <span className='text-muted-foreground text-sm'>
            {row.original.source || '-'}
          </span>
        ),
        size: 90,
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <UserSubscriptionRowActions subscription={row.original} />
        ),
        meta: { pinned: 'right' as const },
        size: 64,
      },
    ],
    [t]
  )
}
