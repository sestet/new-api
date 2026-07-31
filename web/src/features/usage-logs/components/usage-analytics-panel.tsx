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
import { getRouteApi } from '@tanstack/react-router'
import { VChart } from '@visactor/react-vchart'
import dayjs from 'dayjs'
import {
  BadgeCheck,
  CircleAlert,
  Coins,
  Database,
  Waypoints,
} from 'lucide-react'
import { useMemo, useState, type ComponentType } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTheme } from '@/context/theme-provider'
import { formatLogQuota, formatTokens } from '@/lib/format'
import { VCHART_OPTION } from '@/lib/vchart'

import { getUsageAnalytics } from '../api'
import { getUsageAnalyticsSummary } from '../lib/usage-analytics'
import { buildApiParams } from '../lib/utils'
import type { UsageAnalytics, UsageAnalyticsDimension } from '../types'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

type TrendMetric = 'request_count' | 'token_count' | 'quota'
type DistributionKind = 'models' | 'groups'

const EMPTY_ANALYTICS: UsageAnalytics = {
  summary: {
    request_count: 0,
    error_count: 0,
    refund_count: 0,
    token_count: 0,
    quota: 0,
    exact: 0,
    estimated: 0,
    pending: 0,
    failed: 0,
  },
  trend: [],
  models: [],
  groups: [],
}

function MetricCard(props: {
  label: string
  value: string
  detail?: string
  icon: ComponentType<{ className?: string }>
  tone: string
}) {
  const Icon = props.icon
  return (
    <div className='bg-card flex min-w-0 items-center gap-3 rounded-lg border px-3 py-3'>
      <div
        className={`flex size-8 shrink-0 items-center justify-center rounded-md ${props.tone}`}
      >
        <Icon className='size-4' />
      </div>
      <div className='min-w-0'>
        <p className='text-muted-foreground truncate text-xs'>{props.label}</p>
        <div className='flex min-w-0 items-baseline gap-1.5'>
          <p className='truncate font-mono text-base font-semibold tabular-nums'>
            {props.value}
          </p>
          {props.detail && (
            <span className='text-muted-foreground truncate text-[11px]'>
              {props.detail}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

function AnalyticsSkeleton() {
  return (
    <div className='space-y-3'>
      <div className='grid grid-cols-2 gap-2 lg:grid-cols-5'>
        {Array.from({ length: 5 }, (_, index) => (
          <Skeleton key={index} className='h-[66px] rounded-lg' />
        ))}
      </div>
      <div className='grid gap-3 lg:grid-cols-2'>
        <Skeleton className='h-72 rounded-lg' />
        <Skeleton className='h-72 rounded-lg' />
      </div>
    </div>
  )
}

export function UsageAnalyticsPanel() {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const { sensitiveVisible } = useUsageLogsContext()
  const searchParams = route.useSearch()
  const [trendMetric, setTrendMetric] = useState<TrendMetric>('request_count')
  const [distributionKind, setDistributionKind] =
    useState<DistributionKind>('models')

  const queryParams = useMemo(
    () =>
      buildApiParams({
        page: 1,
        pageSize: 1,
        searchParams,
        columnFilters: [],
        isAdmin,
      }),
    [isAdmin, searchParams]
  )
  const analyticsQuery = useQuery({
    queryKey: ['usage-analytics', isAdmin, queryParams],
    queryFn: () => getUsageAnalytics(queryParams, isAdmin),
    select: (response) =>
      response.success ? (response.data ?? EMPTY_ANALYTICS) : EMPTY_ANALYTICS,
    placeholderData: (previousData) => previousData,
  })

  if (analyticsQuery.isLoading) return <AnalyticsSkeleton />

  const analytics = analyticsQuery.data ?? EMPTY_ANALYTICS
  const summary = getUsageAnalyticsSummary(analytics.summary)

  const trendMetricConfig: Record<
    TrendMetric,
    { label: string; formatter: (value: number) => string }
  > = {
    request_count: {
      label: t('Requests'),
      formatter: (value) => value.toLocaleString(),
    },
    token_count: {
      label: t('Tokens'),
      formatter: formatTokens,
    },
    quota: {
      label: t('Cost'),
      formatter: (value) => (sensitiveVisible ? formatLogQuota(value) : '••••'),
    },
  }

  const trendValues = analytics.trend.map((item) => ({
    ...item,
    label: dayjs
      .unix(item.timestamp)
      .format(analytics.trend.length > 36 ? 'MM-DD' : 'HH:mm'),
  }))
  const trendSpec =
    trendValues.length > 0
      ? {
          type: 'line' as const,
          data: [{ id: 'usage-trend', values: trendValues }],
          xField: 'label',
          yField: trendMetric,
          point: { visible: trendValues.length <= 48 },
          line: { style: { lineWidth: 2 } },
          axes: [
            {
              orient: 'bottom',
              label: {
                autoHide: true,
                autoLimit: true,
                style: { fontSize: 10 },
              },
              tick: { visible: false },
            },
            {
              orient: 'left',
              label: {
                formatMethod: (value: number | string) =>
                  trendMetricConfig[trendMetric].formatter(Number(value)),
                style: { fontSize: 10 },
              },
              grid: { visible: true, style: { lineDash: [3, 3] } },
            },
          ],
          tooltip: {
            mark: {
              content: [
                {
                  key: trendMetricConfig[trendMetric].label,
                  value: (datum: Record<string, unknown>) =>
                    trendMetricConfig[trendMetric].formatter(
                      Number(datum[trendMetric]) || 0
                    ),
                },
              ],
            },
          },
        }
      : null

  const distribution: UsageAnalyticsDimension[] =
    distributionKind === 'models' ? analytics.models : analytics.groups
  const distributionValues = distribution.map((item) => ({
    ...item,
    name:
      distributionKind === 'groups' && !sensitiveVisible
        ? '••••'
        : item.name || t('Unknown'),
  }))
  const distributionSpec =
    distributionValues.length > 0
      ? {
          type: 'bar' as const,
          data: [{ id: 'usage-distribution', values: distributionValues }],
          direction: 'horizontal' as const,
          xField: 'request_count',
          yField: 'name',
          bar: { style: { cornerRadius: 3 } },
          axes: [
            {
              orient: 'bottom',
              label: {
                formatMethod: (value: number | string) =>
                  Number(value).toLocaleString(),
                style: { fontSize: 10 },
              },
              grid: { visible: true, style: { lineDash: [3, 3] } },
            },
            {
              orient: 'left',
              label: { autoLimit: true, style: { fontSize: 10 } },
              tick: { visible: false },
            },
          ],
          tooltip: {
            mark: {
              content: [
                {
                  key: t('Requests'),
                  value: (datum: Record<string, unknown>) =>
                    Number(datum.request_count || 0).toLocaleString(),
                },
                {
                  key: t('Tokens'),
                  value: (datum: Record<string, unknown>) =>
                    formatTokens(Number(datum.token_count) || 0),
                },
              ],
            },
          },
        }
      : null

  return (
    <div className='space-y-3'>
      <div className='grid grid-cols-2 gap-2 lg:grid-cols-5'>
        <MetricCard
          label={t('API Requests')}
          value={summary.requestTotal.toLocaleString()}
          icon={Waypoints}
          tone='bg-sky-500/10 text-sky-600 dark:text-sky-400'
        />
        <MetricCard
          label={t('Tokens Used')}
          value={formatTokens(analytics.summary.token_count)}
          icon={Database}
          tone='bg-violet-500/10 text-violet-600 dark:text-violet-400'
        />
        <MetricCard
          label={t('Total cost')}
          value={
            sensitiveVisible ? formatLogQuota(analytics.summary.quota) : '••••'
          }
          icon={Coins}
          tone='bg-amber-500/10 text-amber-600 dark:text-amber-400'
        />
        <MetricCard
          label={t('Success rate')}
          value={
            summary.successRate == null
              ? '-'
              : `${summary.successRate.toFixed(1)}%`
          }
          detail={`${analytics.summary.error_count} ${t('errors')}`}
          icon={CircleAlert}
          tone='bg-rose-500/10 text-rose-600 dark:text-rose-400'
        />
        <MetricCard
          label={t('Exact coverage')}
          value={
            summary.exactCoverage == null
              ? '-'
              : `${summary.exactCoverage.toFixed(1)}%`
          }
          detail={`${analytics.summary.exact}/${summary.trackedCount}`}
          icon={BadgeCheck}
          tone='bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
        />
      </div>

      <div className='grid gap-3 lg:grid-cols-2'>
        <section className='bg-card overflow-hidden rounded-lg border'>
          <header className='flex items-center justify-between gap-3 border-b px-3 py-2.5'>
            <h2 className='text-sm font-semibold'>{t('Request Trend')}</h2>
            <Tabs
              value={trendMetric}
              onValueChange={(value) => setTrendMetric(value as TrendMetric)}
            >
              <TabsList>
                {Object.entries(trendMetricConfig).map(([value, config]) => (
                  <TabsTrigger key={value} value={value} className='text-xs'>
                    {config.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </header>
          <div className='h-64 p-2'>
            {trendSpec ? (
              <VChart
                key={`${trendMetric}-${resolvedTheme}`}
                spec={{
                  ...trendSpec,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
              />
            ) : (
              <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
                {t('No usage data')}
              </div>
            )}
          </div>
        </section>

        <section className='bg-card overflow-hidden rounded-lg border'>
          <header className='flex items-center justify-between gap-3 border-b px-3 py-2.5'>
            <h2 className='text-sm font-semibold'>{t('Usage Distribution')}</h2>
            <Tabs
              value={distributionKind}
              onValueChange={(value) =>
                setDistributionKind(value as DistributionKind)
              }
            >
              <TabsList>
                <TabsTrigger value='models' className='text-xs'>
                  {t('Models')}
                </TabsTrigger>
                <TabsTrigger value='groups' className='text-xs'>
                  {t('Groups')}
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </header>
          <div className='h-64 p-2'>
            {distributionSpec ? (
              <VChart
                key={`${distributionKind}-${resolvedTheme}-${sensitiveVisible}`}
                spec={{
                  ...distributionSpec,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
              />
            ) : (
              <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
                {t('No usage data')}
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
