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
import { useIsFetching, useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTablePage,
  TruncatedCell,
  useDataTable,
} from '@/components/data-table'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { getAuditLogs } from '@/features/usage-logs/api'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from '@/features/usage-logs/components/logs-filter-toolbar'
import { DEFAULT_LOGS_DATA } from '@/features/usage-logs/constants'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import {
  parseLogOther,
  renderAuditContent,
} from '@/features/usage-logs/lib/format'
import {
  getDefaultTimeRange,
  getLogTypeConfig,
} from '@/features/usage-logs/lib/utils'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { formatTimestampToDate } from '@/lib/format'

import type { AuditLogSection } from '../section-registry'

const route = getRouteApi('/_authenticated/audit-logs/$section')

type AuditFilterDraft = {
  sourceKey: string
  username: string
  start: Date
  end: Date
}

export function AuditLogsTable(props: { section: AuditLogSection }) {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const searchParams = route.useSearch()
  const navigate = route.useNavigate()
  const fetching = useIsFetching({ queryKey: ['audit-logs'] })
  const { pagination, onPaginationChange, ensurePageInRange } =
    useTableUrlState({
      search: searchParams,
      navigate,
      pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 50 },
      globalFilter: { enabled: false },
      columnFilters: [],
    })

  const searchState = useMemo<AuditFilterDraft>(() => {
    const defaults = getDefaultTimeRange()
    const start = searchParams.startTime
      ? new Date(searchParams.startTime)
      : defaults.start
    const end = searchParams.endTime
      ? new Date(searchParams.endTime)
      : defaults.end
    const username = searchParams.username || ''
    return {
      sourceKey: `${start.getTime()}\u001f${end.getTime()}\u001f${username}`,
      username,
      start,
      end,
    }
  }, [searchParams.endTime, searchParams.startTime, searchParams.username])
  const [draft, setDraft] = useState(searchState)
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState

  const logsQuery = useQuery({
    queryKey: [
      'audit-logs',
      props.section,
      pagination.pageIndex,
      pagination.pageSize,
      searchParams,
    ],
    queryFn: async () => {
      const result = await getAuditLogs({
        category: props.section,
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        username: searchParams.username || undefined,
        start_timestamp: Math.floor(
          (searchParams.startTime ?? getDefaultTimeRange().start.getTime()) /
            1000
        ),
        end_timestamp: Math.floor(
          (searchParams.endTime ?? getDefaultTimeRange().end.getTime()) / 1000
        ),
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to load logs'))
        return DEFAULT_LOGS_DATA
      }
      return result.data || DEFAULT_LOGS_DATA
    },
    placeholderData: (previousData) => previousData,
  })

  const columns = useMemo<ColumnDef<UsageLog>[]>(
    () => [
      {
        accessorKey: 'created_at',
        header: t('Time'),
        cell: ({ row }) => (
          <span className='font-mono text-xs tabular-nums'>
            {formatTimestampToDate(row.original.created_at)}
          </span>
        ),
        enableHiding: false,
        size: 180,
      },
      {
        accessorKey: 'type',
        header: t('Type'),
        cell: ({ row }) => {
          const config = getLogTypeConfig(row.original.type)
          return (
            <StatusBadge
              label={t(config.label)}
              variant={config.color as StatusBadgeProps['variant']}
              size='sm'
              copyable={false}
            />
          )
        },
        size: 110,
      },
      {
        accessorKey: 'username',
        header: t('User'),
        cell: ({ row }) => (
          <TruncatedCell>{row.original.username || '-'}</TruncatedCell>
        ),
        size: 150,
      },
      {
        accessorKey: 'ip',
        header: t('IP Address'),
        cell: ({ row }) => (
          <span className='font-mono text-xs'>{row.original.ip || '-'}</span>
        ),
        size: 150,
      },
      {
        accessorKey: 'content',
        header: t('Details'),
        cell: ({ row }) => {
          const localized = renderAuditContent(
            parseLogOther(row.original.other),
            t
          )
          const content = localized || row.original.content || '-'
          return (
            <TruncatedCell tooltipContent={content}>{content}</TruncatedCell>
          )
        },
        size: 420,
      },
    ],
    [t]
  )
  const data = (logsQuery.data?.items ?? []) as UsageLog[]
  const { table } = useDataTable({
    data,
    columns,
    pagination,
    enableRowSelection: false,
    onPaginationChange,
    manualPagination: true,
    totalCount: logsQuery.data?.total || 0,
    ensurePageInRange,
    columnVisibilityStorageKey: 'audit-logs:column-visibility',
  })

  const handleSearch = useCallback(() => {
    void navigate({
      to: '/audit-logs/$section',
      params: { section: props.section },
      search: {
        page: 1,
        pageSize: pagination.pageSize,
        username: activeDraft.username || undefined,
        startTime: activeDraft.start.getTime(),
        endTime: activeDraft.end.getTime(),
      },
    })
  }, [activeDraft, navigate, pagination.pageSize, props.section])

  const handleReset = useCallback(() => {
    const defaults = getDefaultTimeRange()
    const next = {
      sourceKey: `${defaults.start.getTime()}\u001f${defaults.end.getTime()}\u001f`,
      username: '',
      start: defaults.start,
      end: defaults.end,
    }
    setDraft(next)
    void navigate({
      to: '/audit-logs/$section',
      params: { section: props.section },
      search: {
        page: 1,
        pageSize: pagination.pageSize,
        startTime: defaults.start.getTime(),
        endTime: defaults.end.getTime(),
      },
    })
  }, [navigate, pagination.pageSize, props.section])

  const dateFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={activeDraft.start}
        end={activeDraft.end}
        onChange={({ start, end }) => {
          if (!start || !end) return
          setDraft((current) => ({ ...current, start, end }))
        }}
      />
    </LogsFilterField>
  )
  const usernameFilter = (
    <LogsFilterField>
      <LogsFilterInput
        value={activeDraft.username}
        placeholder={t('Username')}
        onChange={(event) =>
          setDraft((current) => ({
            ...current,
            username: event.target.value,
          }))
        }
        onKeyDown={(event) => {
          if (event.key === 'Enter') handleSearch()
        }}
      />
    </LogsFilterField>
  )

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={logsQuery.isLoading}
      isFetching={logsQuery.isFetching}
      emptyTitle={t('No Logs Found')}
      emptyDescription={t('No audit logs match the selected filters.')}
      skeletonKeyPrefix='audit-log-skeleton'
      toolbar={
        <LogsFilterToolbar
          table={table}
          primaryFilters={
            <>
              {dateFilter}
              {usernameFilter}
            </>
          }
          mobilePinnedFilters={dateFilter}
          mobileFilters={usernameFilter}
          mobileFilterCount={activeDraft.username ? 1 : 0}
          hasActiveFilters={Boolean(activeDraft.username)}
          searchLoading={fetching > 0}
          onReset={handleReset}
          onSearch={handleSearch}
        />
      }
    />
  )
}
