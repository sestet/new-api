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
import type { UsageAnalyticsSummary } from '../types'

export function getUsageAnalyticsSummary(summary: UsageAnalyticsSummary) {
  const trackedCount =
    summary.exact + summary.estimated + summary.pending + summary.failed
  const requestTotal = summary.request_count + summary.error_count

  return {
    requestTotal,
    trackedCount,
    exactCoverage:
      trackedCount > 0 ? (summary.exact / trackedCount) * 100 : null,
    successRate:
      requestTotal > 0 ? (summary.request_count / requestTotal) * 100 : null,
  }
}
