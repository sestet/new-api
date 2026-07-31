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
import { api } from '@/lib/api'

import { buildQueryParams } from './lib/query'
import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
  GetUsageAnalyticsResponse,
  GetUsageFilterOptionsResponse,
  GetMidjourneyLogsParams,
  GetTaskLogsParams,
  UserInfo,
} from './types'

// ============================================================================
// Generic API Helpers
// ============================================================================

function buildApiPath(endpoint: string, isAdmin: boolean): string {
  return isAdmin ? endpoint : `${endpoint}/self`
}

async function fetchLogs<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogsResponse> {
  const paramRecord = params as unknown as Record<string, unknown>
  const queryParams = buildQueryParams({
    p: paramRecord.p || 1,
    page_size: paramRecord.page_size || 20,
    ...params,
  })
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

async function fetchLogStats<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogStatsResponse> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}/stat?${queryParams}`)
  return res.data
}

// ============================================================================
// Common Log APIs
// ============================================================================

export const getAllUsageLogs = (params: GetLogsParams = {}) =>
  fetchLogs('/api/log/usage', params, true)

export const getUserUsageLogs = (
  params: Omit<GetLogsParams, 'username' | 'channel'> = {}
) => fetchLogs('/api/log/usage', params, false)

export async function getUsageAnalytics(
  params: GetLogStatsParams,
  isAdmin: boolean
): Promise<GetUsageAnalyticsResponse> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const path = isAdmin
    ? '/api/log/usage/analytics'
    : '/api/log/usage/self/analytics'
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

export async function getUsageFilterOptions(
  params: Pick<GetLogStatsParams, 'start_timestamp' | 'end_timestamp'>,
  isAdmin: boolean
): Promise<GetUsageFilterOptionsResponse> {
  const queryParams = buildQueryParams(params)
  const path = isAdmin
    ? '/api/log/usage/options'
    : '/api/log/usage/self/options'
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

export async function getAuditLogs(
  params: GetLogsParams & {
    category: 'login' | 'operation' | 'system'
  }
): Promise<GetLogsResponse> {
  const queryParams = buildQueryParams({
    p: params.p || 1,
    page_size: params.page_size || 20,
    ...params,
  })
  const res = await api.get(`/api/log/audit?${queryParams}`)
  return res.data
}

export const getLogStats = (params: GetLogStatsParams = {}) =>
  fetchLogStats('/api/log', params, true)

export const getUserLogStats = (
  params: Omit<GetLogStatsParams, 'username' | 'channel'> = {}
) => fetchLogStats('/api/log', params, false)

export async function getUserInfo(
  userId: number
): Promise<{ success: boolean; message?: string; data?: UserInfo }> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data
}

// ============================================================================
// MjProxy (Drawing) Logs API
// ============================================================================

export const getAllMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, true)

export const getUserMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, false)

// ============================================================================
// Task Logs API
// ============================================================================

export const getAllTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, true)

export const getUserTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, false)
