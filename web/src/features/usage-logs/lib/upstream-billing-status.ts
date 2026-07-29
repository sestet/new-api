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
export interface UpstreamBillingStatusPresentation {
  labelKey: 'Exact' | 'Estimated' | 'Pending' | 'Failed'
  variant: 'success' | 'warning' | 'info' | 'danger'
}

const upstreamBillingStatusPresentations: Record<
  string,
  UpstreamBillingStatusPresentation
> = {
  exact: { labelKey: 'Exact', variant: 'success' },
  estimated: { labelKey: 'Estimated', variant: 'warning' },
  pending: { labelKey: 'Pending', variant: 'info' },
  failed: { labelKey: 'Failed', variant: 'danger' },
}

export function getUpstreamBillingStatusPresentation(
  status: string | undefined
): UpstreamBillingStatusPresentation | null {
  if (!status) return null
  return upstreamBillingStatusPresentations[status] ?? null
}

export function getUpstreamBillingProviderLabel(
  provider: string | undefined
): string {
  if (provider === 'new_api') return 'New API'
  if (provider === 'sub2api') return 'Sub2API'
  return provider ?? ''
}
