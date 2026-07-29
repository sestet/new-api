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
export interface SubscriptionUsageInput {
  amount_total?: number
  amount_used?: number
  debt_offset?: number
}

export interface SubscriptionUsageBreakdown {
  total: number
  used: number
  debtOffset: number
  requestUsage: number
  available: number
  percent: number
}

export function getSubscriptionUsageBreakdown(
  subscription: SubscriptionUsageInput
): SubscriptionUsageBreakdown {
  const rawTotal = Number(subscription.amount_total || 0)
  const rawUsed = Number(subscription.amount_used || 0)
  const rawDebtOffset = Number(subscription.debt_offset || 0)
  const total = Number.isFinite(rawTotal) ? Math.max(0, rawTotal) : 0
  const used = Number.isFinite(rawUsed) ? Math.max(0, rawUsed) : 0
  const normalizedDebtOffset = Number.isFinite(rawDebtOffset)
    ? Math.max(0, rawDebtOffset)
    : 0
  const debtOffset = Math.min(used, normalizedDebtOffset)
  const requestUsage = Math.max(0, used - debtOffset)
  const available = total > 0 ? Math.max(0, total - used) : 0
  const percent = total > 0 ? Math.min(100, (used / total) * 100) : 0

  return {
    total,
    used,
    debtOffset,
    requestUsage,
    available,
    percent,
  }
}
