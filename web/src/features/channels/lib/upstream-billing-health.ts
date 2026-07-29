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
import type { UpstreamBillingAccount } from '../types'

type HealthStatus = UpstreamBillingAccount['health_status']

type HealthPresentation = {
  status: HealthStatus
  labelKey: 'Healthy' | 'Error' | 'Not tested'
  variant: 'outline' | 'destructive' | 'secondary'
  className?: string
}

export function getUpstreamBillingHealthPresentation(
  status?: string
): HealthPresentation {
  if (status === 'healthy') {
    return {
      status,
      labelKey: 'Healthy',
      variant: 'outline',
      className:
        'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
    }
  }
  if (status === 'error') {
    return { status, labelKey: 'Error', variant: 'destructive' }
  }
  return { status: 'unknown', labelKey: 'Not tested', variant: 'secondary' }
}
