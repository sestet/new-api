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
import { useQuery } from '@tanstack/react-query'
import { Loader2, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatNumber, formatTimestamp, formatTokens } from '@/lib/format'

import { getUpstreamBillingAccountUsageStats } from '../../api'
import type {
  UpstreamBillingAccount,
  UpstreamBillingUsageBucket,
} from '../../types'

type UpstreamBillingAccountStatsDialogProps = {
  account: UpstreamBillingAccount | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

const STATS_PERIODS = [7, 30] as const

function formatUsageCost(value: string): string {
  return formatBillingCurrencyFromUSD(Number(value), {
    abbreviate: false,
    digitsLarge: 6,
    digitsSmall: 8,
  })
}

function UsageTable(props: {
  rows: UpstreamBillingUsageBucket[]
  firstColumnLabel: string
  emptyLabel: string
}) {
  const { t } = useTranslation()

  return (
    <div className='max-h-72 overflow-auto rounded-md border'>
      <Table>
        <TableHeader className='bg-popover sticky top-0 z-10'>
          <TableRow>
            <TableHead>{props.firstColumnLabel}</TableHead>
            <TableHead className='text-right'>{t('Requests')}</TableHead>
            <TableHead className='text-right'>{t('Total tokens')}</TableHead>
            <TableHead className='text-right'>{t('Exact coverage')}</TableHead>
            <TableHead className='text-right'>{t('Upstream cost')}</TableHead>
            <TableHead className='text-right'>{t('Member charge')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={6}
                className='text-muted-foreground h-24 text-center'
              >
                {props.emptyLabel}
              </TableCell>
            </TableRow>
          )}
          {props.rows.map((row) => (
            <TableRow key={row.key}>
              <TableCell className='font-medium'>{row.key}</TableCell>
              <TableCell className='text-right'>
                {formatNumber(row.requests)}
              </TableCell>
              <TableCell className='text-right'>
                {formatTokens(row.total_tokens)}
              </TableCell>
              <TableCell className='text-right'>
                {row.requests > 0
                  ? `${formatNumber((row.exact / row.requests) * 100)}%`
                  : '-'}
              </TableCell>
              <TableCell className='text-right'>
                {formatUsageCost(row.upstream_cost_usd)}
              </TableCell>
              <TableCell className='text-right'>
                {formatUsageCost(row.member_charge_usd)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

export function UpstreamBillingAccountStatsDialog(
  props: UpstreamBillingAccountStatsDialogProps
) {
  const { t } = useTranslation()
  const [days, setDays] = useState<(typeof STATS_PERIODS)[number]>(30)
  const statsQuery = useQuery({
    queryKey: [
      'channels',
      'upstream-billing-account-stats',
      props.account?.id,
      days,
    ],
    queryFn: ({ signal }) =>
      getUpstreamBillingAccountUsageStats(props.account?.id ?? 0, days, signal),
    enabled: props.open && props.account !== null,
    retry: false,
  })
  const stats = statsQuery.data?.data
  const dailyRows = stats?.daily.filter((row) => row.requests > 0) || []

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-5xl'>
        <DialogHeader className='pr-10'>
          <DialogTitle>{t('Usage statistics')}</DialogTitle>
          <DialogDescription>{props.account?.name}</DialogDescription>
        </DialogHeader>

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <ToggleGroup
            value={[String(days)]}
            onValueChange={(values) => {
              const value = Number(values.find((item) => item !== String(days)))
              if (value === 7 || value === 30) setDays(value)
            }}
            variant='outline'
            size='sm'
            aria-label={t('Usage period')}
          >
            <ToggleGroupItem value='7'>{t('Last 7 days')}</ToggleGroupItem>
            <ToggleGroupItem value='30'>{t('Last 30 days')}</ToggleGroupItem>
          </ToggleGroup>
          {stats && stats.last_checked_at > 0 && (
            <span className='text-muted-foreground text-xs tabular-nums'>
              {t('Last checked')}: {formatTimestamp(stats.last_checked_at)}
            </span>
          )}
        </div>

        {statsQuery.isLoading && (
          <div className='flex min-h-72 items-center justify-center'>
            <Loader2 className='text-muted-foreground size-5 animate-spin' />
          </div>
        )}

        {statsQuery.isError && (
          <div className='flex min-h-72 flex-col items-center justify-center gap-3 text-center'>
            <p className='text-sm font-medium'>
              {t('Failed to load usage statistics')}
            </p>
            <p className='text-muted-foreground max-w-md text-xs'>
              {statsQuery.error instanceof Error
                ? statsQuery.error.message
                : t('Failed to load')}
            </p>
            <Button
              type='button'
              variant='outline'
              onClick={() => statsQuery.refetch()}
            >
              <RefreshCw data-icon='inline-start' />
              {t('Retry')}
            </Button>
          </div>
        )}

        {stats && !statsQuery.isError && (
          <div className='space-y-5'>
            <div className='grid overflow-hidden rounded-md border sm:grid-cols-2 lg:grid-cols-5'>
              {[
                [t('Requests'), formatNumber(stats.requests)],
                [t('Total tokens'), formatTokens(stats.total_tokens)],
                [t('Exact coverage'), `${formatNumber(stats.coverage * 100)}%`],
                [t('Upstream cost'), formatUsageCost(stats.upstream_cost_usd)],
                [t('Member charge'), formatUsageCost(stats.member_charge_usd)],
              ].map(([label, value]) => (
                <div
                  key={label}
                  className='border-b p-3 last:border-b-0 sm:border-r lg:border-b-0 lg:last:border-r-0 sm:[&:nth-child(2)]:border-r-0 lg:[&:nth-child(2)]:border-r'
                >
                  <p className='text-muted-foreground text-xs'>{label}</p>
                  <p className='mt-1 text-base font-semibold tabular-nums'>
                    {value}
                  </p>
                </div>
              ))}
            </div>

            <div className='flex flex-wrap gap-2'>
              <Badge variant='secondary'>
                {t('Exact')}: {formatNumber(stats.exact)}
              </Badge>
              <Badge variant='secondary'>
                {t('Estimated')}: {formatNumber(stats.estimated)}
              </Badge>
              <Badge variant='secondary'>
                {t('Pending')}: {formatNumber(stats.pending)}
              </Badge>
              <Badge variant={stats.failed > 0 ? 'destructive' : 'secondary'}>
                {t('Failed')}: {formatNumber(stats.failed)}
              </Badge>
            </div>

            {stats.requests === 0 ? (
              <div className='text-muted-foreground flex min-h-40 items-center justify-center rounded-md border text-sm'>
                {t('No usage records')}
              </div>
            ) : (
              <>
                <section className='space-y-2'>
                  <h3 className='text-sm font-semibold'>{t('Daily usage')}</h3>
                  <UsageTable
                    rows={dailyRows}
                    firstColumnLabel={t('Date')}
                    emptyLabel={t('No usage records')}
                  />
                </section>
                <section className='space-y-2'>
                  <h3 className='text-sm font-semibold'>
                    {t('Model distribution')}
                  </h3>
                  <UsageTable
                    rows={stats.models}
                    firstColumnLabel={t('Model')}
                    emptyLabel={t('No usage records')}
                  />
                </section>
              </>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
