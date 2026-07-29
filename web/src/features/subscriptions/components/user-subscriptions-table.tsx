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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { getAdminPlans, getAdminUserSubscriptions } from '../api'
import { useSubscriptions } from './subscriptions-provider'
import { useUserSubscriptionsColumns } from './user-subscriptions-columns'

const route = getRouteApi('/_authenticated/subscriptions/')

export function UserSubscriptionsTable() {
  const { t } = useTranslation()
  const columns = useUserSubscriptionsColumns()
  const { refreshTrigger } = useSubscriptions()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'plan_id', searchKey: 'plan', type: 'array' },
    ],
  })
  const status =
    (
      columnFilters.find((filter) => filter.id === 'status')?.value as
        | string[]
        | undefined
    )?.[0] || ''
  const planId = Number(
    (
      columnFilters.find((filter) => filter.id === 'plan_id')?.value as
        | string[]
        | undefined
    )?.[0] || 0
  )

  const { data: plans = [] } = useQuery({
    queryKey: ['admin-subscription-plans', refreshTrigger],
    queryFn: async () => {
      const result = await getAdminPlans()
      return result.data || []
    },
    placeholderData: (previous) => previous,
  })
  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'admin-user-subscriptions',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      status,
      planId,
      refreshTrigger,
    ],
    queryFn: async () => {
      const result = await getAdminUserSubscriptions({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter,
        status,
        plan_id: planId || undefined,
      })
      if (!result.success) {
        toast.error(result.message || t('Loading failed'))
        return { items: [], total: 0 }
      }
      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previous) => previous,
  })

  const { table } = useDataTable({
    data: data?.items || [],
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualFiltering: true,
    manualPagination: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  const planOptions = useMemo(
    () =>
      plans.map((record) => ({
        label: record.plan.title,
        value: String(record.plan.id),
      })),
    [plans]
  )

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No subscribed users')}
      emptyDescription={t(
        'Assign a subscription to a user or adjust the current filters.'
      )}
      skeletonKeyPrefix='user-subscriptions-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by username, email or user ID...'),
        searchDebounceMs: 300,
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            singleSelect: true,
            options: [
              { label: t('Active'), value: 'active' },
              { label: t('Expired'), value: 'expired' },
              { label: t('Cancelled'), value: 'cancelled' },
            ],
          },
          {
            columnId: 'plan_id',
            title: t('Plan'),
            singleSelect: true,
            options: planOptions,
          },
        ],
      }}
    />
  )
}
