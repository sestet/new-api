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
import type { LogOtherData } from '../types'

export interface AdminBillingBreakdown {
  upstreamCostQuota: number
  groupRatio: number
  chargedQuota: number
}

export function getAdminBillingBreakdown(
  other: LogOtherData | null,
  isAdmin: boolean
): AdminBillingBreakdown | null {
  if (!isAdmin || other?.upstream_billing_status !== 'exact') return null

  const billing = other.admin_info?.upstream_billing
  if (!billing) return null

  const upstreamCostQuota = billing.upstream_cost_quota
  const chargedQuota = billing.charged_quota
  const groupRatio = Number.parseFloat(billing.group_ratio ?? '')
  if (
    upstreamCostQuota == null ||
    chargedQuota == null ||
    !Number.isFinite(upstreamCostQuota) ||
    !Number.isFinite(chargedQuota) ||
    !Number.isFinite(groupRatio) ||
    upstreamCostQuota < 0 ||
    chargedQuota < 0 ||
    groupRatio <= 0 ||
    groupRatio === 1
  ) {
    return null
  }

  return { upstreamCostQuota, groupRatio, chargedQuota }
}
