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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Settings2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatQuota } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelOps } from './api'
import { ChannelsDialogs } from './components/channels-dialogs'
import { ChannelsPrimaryButtons } from './components/channels-primary-buttons'
import { ChannelsProvider } from './components/channels-provider'
import { ChannelsTable } from './components/channels-table'
import { UpstreamBillingAccountsTab } from './components/upstream-billing-accounts-tab'

export function Channels() {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState('channels')
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const channelOpsQuery = useQuery({
    queryKey: ['channel-ops'],
    queryFn: getChannelOps,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
  const retryTimes = channelOpsQuery.data?.data?.retry_times
  const retryLabel =
    typeof retryTimes === 'number' ? `${t('Max Retries')}: ${retryTimes}` : null
  let retryBadge = null
  if (retryLabel) {
    retryBadge = isRoot ? (
      <Tooltip>
        <TooltipTrigger
          render={
            <Badge
              variant='outline'
              className='shrink-0 cursor-pointer'
              aria-label={t('Retry Settings')}
              render={
                <Link
                  to='/system-settings/models/$section'
                  params={{ section: 'routing-reliability' }}
                />
              }
            />
          }
        >
          <span>{retryLabel}</span>
          <Settings2 data-icon='inline-end' />
        </TooltipTrigger>
        <TooltipContent>
          <p>{t('Retry Settings')}</p>
        </TooltipContent>
      </Tooltip>
    ) : (
      <Badge variant='outline' className='shrink-0'>
        {retryLabel}
      </Badge>
    )
  }
  const billingStats = channelOpsQuery.data?.data?.upstream_billing_stats
  const billingCoverage = billingStats
    ? `${(billingStats.coverage * 100).toFixed(1)}%`
    : null
  const billingBadge =
    billingStats && billingStats.total > 0 ? (
      <Tooltip>
        <TooltipTrigger
          render={<Badge variant='outline' className='shrink-0 cursor-help' />}
        >
          {t('Exact billing')}: {billingStats.exact}/{billingStats.total} (
          {billingCoverage})
        </TooltipTrigger>
        <TooltipContent className='space-y-1'>
          <p>
            {t('Exact amount')}: {formatQuota(billingStats.exact_quota)}
          </p>
          <p>
            {t('Estimated amount')}: {formatQuota(billingStats.estimated_quota)}
          </p>
          <p>
            {t('Pending amount')}: {formatQuota(billingStats.pending_quota)}
          </p>
          <p>
            {t('Failed requests')}: {billingStats.failed}
          </p>
        </TooltipContent>
      </Tooltip>
    ) : null

  return (
    <ChannelsProvider>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          <span className='flex min-w-0 items-center gap-2'>
            <span className='truncate'>{t('Channels')}</span>
            {retryBadge}
            {billingBadge}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {activeTab === 'channels' && <ChannelsPrimaryButtons />}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs
            value={activeTab}
            onValueChange={setActiveTab}
            className='min-h-0 flex-1'
          >
            <TabsList variant='line'>
              <TabsTrigger value='channels'>{t('Channels')}</TabsTrigger>
              <TabsTrigger value='upstream-accounts'>
                {t('Upstream Accounts')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='channels' className='min-h-0'>
              <ChannelsTable />
            </TabsContent>
            <TabsContent value='upstream-accounts' className='min-h-0'>
              <UpstreamBillingAccountsTab />
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ChannelsDialogs />
    </ChannelsProvider>
  )
}
