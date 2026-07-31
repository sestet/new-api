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
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { AuditLogsTable } from './components/audit-logs-table'
import type { AuditLogSection } from './section-registry'

const route = getRouteApi('/_authenticated/audit-logs/$section')

const SECTIONS: Array<{ value: AuditLogSection; label: string }> = [
  { value: 'login', label: 'Login Activity' },
  { value: 'operation', label: 'Admin Operations' },
  { value: 'system', label: 'System Events' },
]

export function AuditLogs() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const params = route.useParams()

  const handleSectionChange = useCallback(
    (section: string) => {
      void navigate({
        to: '/audit-logs/$section',
        params: { section: section as AuditLogSection },
      })
    },
    [navigate]
  )

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Audit Logs')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-3'>
          <Tabs value={params.section} onValueChange={handleSectionChange}>
            <TabsList className='max-w-full overflow-x-auto'>
              {SECTIONS.map((section) => (
                <TabsTrigger key={section.value} value={section.value}>
                  {t(section.label)}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <div className='min-h-0 flex-1'>
            <AuditLogsTable section={params.section as AuditLogSection} />
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
