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
import { useQuery, useQueryClient, useIsFetching } from '@tanstack/react-query'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { useState, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { getUsageFilterOptions } from '../api'
import {
  LOG_TYPE_ALL_VALUE,
  USAGE_LOG_TYPE_FILTERS,
  UPSTREAM_BILLING_STATUS_FILTERS,
} from '../constants'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/utils'
import type { CommonLogFilters, UpstreamBillingStatusFilter } from '../types'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from './logs-filter-toolbar'
import { UsageFilterCombobox } from './usage-filter-combobox'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

type LogTypeValue = (typeof USAGE_LOG_TYPE_FILTERS)[number]['value']
type BillingStatusValue =
  (typeof UPSTREAM_BILLING_STATUS_FILTERS)[number]['value']
const logTypeValueSet = new Set<string>(
  USAGE_LOG_TYPE_FILTERS.map((type) => type.value)
)
const billingStatusValueSet = new Set<string>(
  UPSTREAM_BILLING_STATUS_FILTERS.map((status) => status.value)
)

type CommonLogDraft = {
  sourceKey: string
  filters: CommonLogFilters
  logType: LogTypeValue
}

const EMPTY_FILTER_OPTIONS = {
  models: [],
  groups: [],
  tokens: [],
  users: [],
  channels: [],
}

function isLogTypeValue(value: string): value is LogTypeValue {
  return logTypeValueSet.has(value)
}

function getLogTypeValue(value: unknown): LogTypeValue {
  return Array.isArray(value) &&
    value.length === 1 &&
    typeof value[0] === 'string' &&
    isLogTypeValue(value[0])
    ? value[0]
    : LOG_TYPE_ALL_VALUE
}

function getBillingStatusValue(value: unknown): BillingStatusValue {
  return typeof value === 'string' && billingStatusValueSet.has(value)
    ? (value as BillingStatusValue)
    : 'all'
}

function buildSearchSourceKey(values: {
  startTime?: unknown
  endTime?: unknown
  channel?: unknown
  model?: unknown
  token?: unknown
  group?: unknown
  username?: unknown
  requestId?: unknown
  upstreamRequestId?: unknown
  billingStatus?: unknown
  type?: unknown
}) {
  return [
    values.startTime,
    values.endTime,
    values.channel,
    values.model,
    values.token,
    values.group,
    values.username,
    values.requestId,
    values.upstreamRequestId,
    values.billingStatus,
    Array.isArray(values.type) ? values.type.join(',') : values.type,
  ]
    .map((value) => String(value ?? ''))
    .join('\u001f')
}

interface CommonLogsFilterBarProps<TData> {
  table: Table<TData>
}

export function CommonLogsFilterBar<TData>(
  props: CommonLogsFilterBarProps<TData>
) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const { sensitiveVisible, setSensitiveVisible } = useUsageLogsContext()
  const fetchingLogs = useIsFetching({ queryKey: ['logs'] })

  const searchState = useMemo<CommonLogDraft>(() => {
    const { start, end } = getDefaultTimeRange()
    const sourceValues = {
      startTime: searchParams.startTime,
      endTime: searchParams.endTime,
      channel: searchParams.channel,
      model: searchParams.model,
      token: searchParams.token,
      group: searchParams.group,
      username: searchParams.username,
      requestId: searchParams.requestId,
      upstreamRequestId: searchParams.upstreamRequestId,
      billingStatus: searchParams.billingStatus,
      type: searchParams.type,
    }
    const filters: CommonLogFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      channel: searchParams.channel || undefined,
      model: searchParams.model || undefined,
      token: searchParams.token || undefined,
      group: searchParams.group || undefined,
      username: searchParams.username || undefined,
      requestId: searchParams.requestId || undefined,
      upstreamRequestId: searchParams.upstreamRequestId || undefined,
      billingStatus: getBillingStatusValue(searchParams.billingStatus),
    }
    return {
      sourceKey: buildSearchSourceKey(sourceValues),
      filters,
      logType: getLogTypeValue(searchParams.type),
    }
  }, [
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.model,
    searchParams.token,
    searchParams.group,
    searchParams.username,
    searchParams.requestId,
    searchParams.upstreamRequestId,
    searchParams.billingStatus,
    searchParams.type,
  ])
  const [draft, setDraft] = useState<CommonLogDraft>(() => searchState)
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState
  const filters = activeDraft.filters
  const logType = activeDraft.logType
  const filterOptionsQuery = useQuery({
    queryKey: [
      'usage-log-filter-options',
      isAdmin,
      searchState.filters.startTime?.getTime(),
      searchState.filters.endTime?.getTime(),
    ],
    queryFn: () =>
      getUsageFilterOptions(
        {
          start_timestamp: searchState.filters.startTime
            ? Math.floor(searchState.filters.startTime.getTime() / 1000)
            : undefined,
          end_timestamp: searchState.filters.endTime
            ? Math.floor(searchState.filters.endTime.getTime() / 1000)
            : undefined,
        },
        isAdmin
      ),
    select: (response) =>
      response.success
        ? (response.data ?? EMPTY_FILTER_OPTIONS)
        : EMPTY_FILTER_OPTIONS,
    staleTime: 60_000,
  })
  const filterOptions = filterOptionsQuery.data ?? EMPTY_FILTER_OPTIONS
  const modelOptions = useMemo(
    () => filterOptions.models.map((value) => ({ value, label: value })),
    [filterOptions.models]
  )
  const groupOptions = useMemo(
    () => filterOptions.groups.map((value) => ({ value, label: value })),
    [filterOptions.groups]
  )
  const tokenOptions = useMemo(
    () => filterOptions.tokens.map((value) => ({ value, label: value })),
    [filterOptions.tokens]
  )
  const userOptions = useMemo(
    () => filterOptions.users.map((value) => ({ value, label: value })),
    [filterOptions.users]
  )
  const channelOptions = useMemo(
    () =>
      filterOptions.channels.map((channel) => ({
        value: String(channel.id),
        label: channel.name
          ? `${channel.name} (#${channel.id})`
          : `#${channel.id}`,
      })),
    [filterOptions.channels]
  )

  const handleChange = useCallback(
    (field: keyof CommonLogFilters, value: Date | string | undefined) => {
      setDraft((current) => {
        const base =
          current.sourceKey === searchState.sourceKey ? current : searchState
        return {
          sourceKey: searchState.sourceKey,
          filters: { ...base.filters, [field]: value },
          logType: base.logType,
        }
      })
    },
    [searchState]
  )

  const handleApply = useCallback(() => {
    const filterParams = buildSearchParams(filters, 'common')
    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        ...filterParams,
        type: [logType],
        page: 1,
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-analytics'] })
  }, [filters, logType, navigate, queryClient])

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: CommonLogFilters = { startTime: start, endTime: end }
    const resetSearch = {
      type: [LOG_TYPE_ALL_VALUE],
      startTime: start.getTime(),
      endTime: end.getTime(),
      billingStatus: 'all' as UpstreamBillingStatusFilter,
    }
    setDraft({
      sourceKey: buildSearchSourceKey(resetSearch),
      filters: resetFilters,
      logType: LOG_TYPE_ALL_VALUE,
    })

    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        page: 1,
        ...resetSearch,
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-analytics'] })
  }, [navigate, queryClient])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const hasExpandedFilters =
    !!filters.token ||
    !!filters.group ||
    !!filters.username ||
    !!filters.channel ||
    !!filters.requestId ||
    !!filters.upstreamRequestId

  const hasTypeFilter = logType !== LOG_TYPE_ALL_VALUE
  const hasAdditionalFilters =
    !!filters.model ||
    (filters.billingStatus != null && filters.billingStatus !== 'all') ||
    hasTypeFilter ||
    hasExpandedFilters

  const expandedFilterCount = [
    filters.token,
    filters.group,
    isAdmin ? filters.username : undefined,
    isAdmin ? filters.channel : undefined,
    filters.requestId,
    filters.upstreamRequestId,
  ].filter(Boolean).length
  const logTypeItems = useMemo(
    () =>
      USAGE_LOG_TYPE_FILTERS.map((type) => ({
        value: type.value,
        label: t(type.label),
      })),
    [t]
  )
  const logTypeLabel =
    logTypeItems.find((type) => type.value === logType)?.label ?? t('All Types')
  const billingStatusItems = useMemo(
    () =>
      UPSTREAM_BILLING_STATUS_FILTERS.map((status) => ({
        value: status.value,
        label: t(status.label),
      })),
    [t]
  )
  const billingStatus = getBillingStatusValue(filters.billingStatus)
  const billingStatusLabel =
    billingStatusItems.find((status) => status.value === billingStatus)
      ?.label ?? t('All billing statuses')

  const sensitiveToggle = (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={() => setSensitiveVisible(!sensitiveVisible)}
            aria-label={sensitiveVisible ? t('Hide') : t('Show')}
            className='text-muted-foreground hover:text-foreground size-7'
          />
        }
      >
        {sensitiveVisible ? <Eye /> : <EyeOff />}
      </TooltipTrigger>
      <TooltipContent>
        {sensitiveVisible ? t('Hide') : t('Show')}
      </TooltipContent>
    </Tooltip>
  )

  const dateRangeFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={filters.startTime}
        end={filters.endTime}
        onChange={({ start, end }) => {
          handleChange('startTime', start)
          handleChange('endTime', end)
        }}
      />
    </LogsFilterField>
  )
  const modelFilter = (
    <LogsFilterField>
      <UsageFilterCombobox
        placeholder={t('Model Name')}
        options={modelOptions}
        value={filters.model || ''}
        onValueChange={(value) => handleChange('model', value)}
      />
    </LogsFilterField>
  )
  const groupFilter = (
    <LogsFilterField>
      <UsageFilterCombobox
        placeholder={t('Group')}
        options={groupOptions}
        value={filters.group || ''}
        masked={!sensitiveVisible}
        onValueChange={(value) => handleChange('group', value)}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const billingStatusFilter = (
    <LogsFilterField>
      <Select
        items={billingStatusItems}
        value={billingStatus}
        onValueChange={(value) => {
          handleChange(
            'billingStatus',
            getBillingStatusValue(value) as UpstreamBillingStatusFilter
          )
        }}
      >
        <SelectTrigger>
          <SelectValue>{billingStatusLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {UPSTREAM_BILLING_STATUS_FILTERS.map((status) => (
              <SelectItem key={status.value} value={status.value}>
                {t(status.label)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )
  const typeFilter = (
    <LogsFilterField>
      <Select
        items={logTypeItems}
        value={logType}
        onValueChange={(value) => {
          const nextLogType =
            value !== null && isLogTypeValue(value) ? value : LOG_TYPE_ALL_VALUE
          setDraft((current) => {
            const base =
              current.sourceKey === searchState.sourceKey
                ? current
                : searchState
            return {
              sourceKey: searchState.sourceKey,
              filters: base.filters,
              logType: nextLogType,
            }
          })
        }}
      >
        <SelectTrigger>
          <SelectValue>{logTypeLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {USAGE_LOG_TYPE_FILTERS.map((type) => (
              <SelectItem key={type.value} value={type.value}>
                {t(type.label)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )
  const advancedFilters = (
    <>
      {groupFilter}
      <LogsFilterField>
        <UsageFilterCombobox
          placeholder={t('Token Name')}
          options={tokenOptions}
          value={filters.token || ''}
          masked={!sensitiveVisible}
          onValueChange={(value) => handleChange('token', value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      {isAdmin && (
        <LogsFilterField>
          <UsageFilterCombobox
            placeholder={t('Username')}
            options={userOptions}
            value={filters.username || ''}
            masked={!sensitiveVisible}
            onValueChange={(value) => handleChange('username', value)}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      {isAdmin && (
        <LogsFilterField>
          <UsageFilterCombobox
            placeholder={t('Channel ID')}
            options={channelOptions}
            value={filters.channel || ''}
            onValueChange={(value) => handleChange('channel', value)}
          />
        </LogsFilterField>
      )}
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Request ID')}
          value={filters.requestId || ''}
          onChange={(e) => handleChange('requestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Upstream Request ID')}
          value={filters.upstreamRequestId || ''}
          onChange={(e) => handleChange('upstreamRequestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
    </>
  )

  return (
    <LogsFilterToolbar
      table={props.table}
      actionStart={sensitiveToggle}
      primaryFilters={
        <>
          {dateRangeFilter}
          {modelFilter}
          {billingStatusFilter}
          {typeFilter}
        </>
      }
      advancedFilters={advancedFilters}
      mobilePinnedFilters={dateRangeFilter}
      mobileFilters={
        <>
          {modelFilter}
          {billingStatusFilter}
          {typeFilter}
          {advancedFilters}
        </>
      }
      mobileFilterCount={
        [filters.model, billingStatus !== 'all', hasTypeFilter].filter(Boolean)
          .length + expandedFilterCount
      }
      hasAdvancedActiveFilters={hasExpandedFilters}
      advancedFilterCount={expandedFilterCount}
      hasActiveFilters={hasAdditionalFilters}
      onSearch={handleApply}
      searchLoading={fetchingLogs > 0}
      onReset={handleReset}
    />
  )
}
