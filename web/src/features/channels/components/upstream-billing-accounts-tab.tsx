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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChartNoAxesColumnIncreasing,
  CircleAlert,
  CircleCheck,
  CircleDashed,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  TestTube2,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { formatTimestamp } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import {
  deleteUpstreamBillingAccount,
  getUpstreamBillingAccounts,
  reconcileUpstreamBillingAccount,
  testUpstreamBillingAccount,
  upstreamBillingAccountsQueryKey,
} from '../api'
import { getUpstreamBillingHealthPresentation } from '../lib/upstream-billing-health'
import type { UpstreamBillingAccount } from '../types'
import { UpstreamBillingAccountStatsDialog } from './dialogs/upstream-billing-account-stats-dialog'
import { UpstreamBillingAccountDrawer } from './dialogs/upstream-billing-accounts-dialog'

export function UpstreamBillingAccountsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingAccount, setEditingAccount] =
    useState<UpstreamBillingAccount | null>(null)
  const [testingAccountId, setTestingAccountId] = useState<number | null>(null)
  const [reconcilingAccountId, setReconcilingAccountId] = useState<
    number | null
  >(null)
  const [deletingAccount, setDeletingAccount] =
    useState<UpstreamBillingAccount | null>(null)
  const [statsAccount, setStatsAccount] =
    useState<UpstreamBillingAccount | null>(null)

  const accountsQuery = useQuery({
    queryKey: upstreamBillingAccountsQueryKey,
    queryFn: ({ signal }) => getUpstreamBillingAccounts(signal),
    retry: false,
  })
  const accounts = accountsQuery.data?.data || []

  const openCreateAccount = () => {
    setEditingAccount(null)
    setEditorOpen(true)
  }

  const openEditAccount = (account: UpstreamBillingAccount) => {
    setEditingAccount(account)
    setEditorOpen(true)
  }

  const handleEditorOpenChange = (open: boolean) => {
    setEditorOpen(open)
    if (!open) setEditingAccount(null)
  }

  const handleTestAccount = async (account: UpstreamBillingAccount) => {
    setTestingAccountId(account.id)
    try {
      const response = await testUpstreamBillingAccount(account.id)
      if (!response.success) {
        throw new Error(response.message || t('Upstream account test failed'))
      }
      toast.success(
        t('Upstream account connected: {{provider}}', {
          provider: response.data?.provider || account.provider,
        })
      )
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Upstream account test failed')
      )
    } finally {
      await queryClient.invalidateQueries({
        queryKey: upstreamBillingAccountsQueryKey,
      })
      setTestingAccountId(null)
    }
  }

  const handleDeleteAccount = async () => {
    if (!deletingAccount) return
    const response = await deleteUpstreamBillingAccount(deletingAccount.id)
    if (!response.success) {
      toast.error(response.message || t('Failed to delete upstream account'))
      return
    }
    toast.success(t('Upstream account deleted'))
    setDeletingAccount(null)
    await queryClient.invalidateQueries({
      queryKey: upstreamBillingAccountsQueryKey,
    })
  }

  const handleReconcileAccount = async (account: UpstreamBillingAccount) => {
    setReconcilingAccountId(account.id)
    try {
      const response = await reconcileUpstreamBillingAccount(account.id)
      if (!response.success) {
        toast.error(
          response.message ||
            t('Failed to start upstream billing reconciliation')
        )
        return
      }
      toast.success(t('Upstream billing reconciliation started'))
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to start upstream billing reconciliation')
      )
    } finally {
      setReconcilingAccountId(null)
      await queryClient.invalidateQueries({
        queryKey: upstreamBillingAccountsQueryKey,
      })
    }
  }

  if (accountsQuery.isLoading) {
    return (
      <div className='flex min-h-64 items-center justify-center'>
        <Loader2 className='text-muted-foreground size-5 animate-spin' />
      </div>
    )
  }

  if (accountsQuery.isError) {
    const message =
      accountsQuery.error instanceof Error
        ? accountsQuery.error.message
        : t('Failed to load')
    return (
      <div className='flex min-h-64 flex-col items-center justify-center gap-3'>
        <div className='flex flex-col gap-1 text-center'>
          <p className='text-sm font-medium'>{t('Failed to load')}</p>
          <p className='text-muted-foreground max-w-md text-xs'>{message}</p>
        </div>
        <Button
          type='button'
          variant='outline'
          onClick={() => accountsQuery.refetch()}
        >
          {t('Retry')}
        </Button>
      </div>
    )
  }

  return (
    <TooltipProvider delay={100}>
      <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-auto pb-1'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <h2 className='text-sm font-semibold'>
              {t('Manage upstream accounts')}
            </h2>
            <p className='text-muted-foreground mt-1 max-w-2xl text-xs'>
              {t(
                'Share one billing login across multiple channels while keeping each channel API key separate.'
              )}
            </p>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              type='button'
              disabled={!canEditSensitive}
              onClick={openCreateAccount}
            >
              <Plus data-icon='inline-start' />
              {t('Create upstream account')}
            </Button>
          </div>
        </div>

        <div className='overflow-x-auto rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Account name')}</TableHead>
                <TableHead>{t('Billing system')}</TableHead>
                <TableHead>{t('Billing API Base URL')}</TableHead>
                <TableHead>{t('Channels')}</TableHead>
                <TableHead>{t('Enabled')}</TableHead>
                <TableHead>{t('Connection status')}</TableHead>
                <TableHead className='min-w-64 text-right'>
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {accounts.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className='text-muted-foreground h-28 text-center'
                  >
                    {t('No upstream accounts configured')}
                  </TableCell>
                </TableRow>
              )}
              {accounts.map((account) => (
                <TableRow key={account.id}>
                  <TableCell className='font-medium'>{account.name}</TableCell>
                  <TableCell>
                    <Badge variant='outline'>{account.provider}</Badge>
                  </TableCell>
                  <TableCell className='max-w-80 truncate'>
                    {account.api_base_url}
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {account.channel_count}
                  </TableCell>
                  <TableCell>
                    <Badge variant={account.enabled ? 'default' : 'secondary'}>
                      {account.enabled ? t('Enabled') : t('Disabled')}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {(() => {
                      const health = getUpstreamBillingHealthPresentation(
                        account.health_status
                      )
                      const healthError =
                        account.health_error?.trim() || t('Unknown')
                      const statusBadge = (
                        <Badge
                          variant={health.variant}
                          className={health.className}
                        >
                          {health.status === 'healthy' && (
                            <CircleCheck data-icon='inline-start' />
                          )}
                          {health.status === 'error' && (
                            <CircleAlert data-icon='inline-start' />
                          )}
                          {health.status === 'unknown' && (
                            <CircleDashed data-icon='inline-start' />
                          )}
                          {t(health.labelKey)}
                        </Badge>
                      )

                      return (
                        <div className='flex min-w-36 flex-col items-start gap-1'>
                          {health.status === 'error' ? (
                            <Tooltip>
                              <TooltipTrigger
                                render={
                                  <button
                                    type='button'
                                    title={healthError}
                                    className='focus-visible:ring-ring inline-flex cursor-help rounded-sm focus-visible:ring-2 focus-visible:outline-none'
                                    aria-label={`${t('Error')}: ${healthError}`}
                                  />
                                }
                              >
                                {statusBadge}
                              </TooltipTrigger>
                              <TooltipContent
                                align='start'
                                className='block max-w-sm space-y-1.5'
                              >
                                <p className='font-medium'>{t('Error')}</p>
                                <p className='break-words whitespace-pre-wrap'>
                                  {healthError}
                                </p>
                                <p>
                                  {t('Last checked')}:{' '}
                                  {formatTimestamp(account.health_checked_at)}
                                </p>
                              </TooltipContent>
                            </Tooltip>
                          ) : (
                            statusBadge
                          )}
                          {health.status === 'error' && (
                            <span
                              className='text-destructive/80 max-w-56 truncate text-xs'
                              title={healthError}
                            >
                              {healthError}
                            </span>
                          )}
                          {account.health_checked_at > 0 && (
                            <span className='text-muted-foreground text-xs whitespace-nowrap tabular-nums'>
                              {formatTimestamp(account.health_checked_at)}
                            </span>
                          )}
                        </div>
                      )
                    })()}
                  </TableCell>
                  <TableCell>
                    <div className='flex justify-end gap-1'>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Usage statistics')}
                              onClick={() => setStatsAccount(account)}
                            />
                          }
                        >
                          <ChartNoAxesColumnIncreasing />
                        </TooltipTrigger>
                        <TooltipContent>{t('Usage statistics')}</TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              type='button'
                              variant='ghost'
                              size='sm'
                              disabled={
                                !canEditSensitive ||
                                reconcilingAccountId === account.id
                              }
                              onClick={() => handleReconcileAccount(account)}
                            />
                          }
                        >
                          {reconcilingAccountId === account.id ? (
                            <Loader2
                              data-icon='inline-start'
                              className='animate-spin'
                            />
                          ) : (
                            <RefreshCw data-icon='inline-start' />
                          )}
                          {t('Reconcile now')}
                        </TooltipTrigger>
                        <TooltipContent>
                          {t('Reconcile this upstream account now')}
                        </TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              type='button'
                              variant='ghost'
                              size='sm'
                              disabled={
                                !canEditSensitive ||
                                testingAccountId === account.id
                              }
                              onClick={() => handleTestAccount(account)}
                            />
                          }
                        >
                          {testingAccountId === account.id ? (
                            <Loader2
                              data-icon='inline-start'
                              className='animate-spin'
                            />
                          ) : (
                            <TestTube2 data-icon='inline-start' />
                          )}
                          {t('Test')}
                        </TooltipTrigger>
                        <TooltipContent>{t('Test account')}</TooltipContent>
                      </Tooltip>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        disabled={!canEditSensitive}
                        onClick={() => openEditAccount(account)}
                      >
                        <Pencil data-icon='inline-start' />
                        {t('Edit')}
                      </Button>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              type='button'
                              variant='ghost'
                              size='sm'
                              className='text-destructive'
                              disabled={
                                !canEditSensitive || account.channel_count > 0
                              }
                              onClick={() => setDeletingAccount(account)}
                            />
                          }
                        >
                          <Trash2 data-icon='inline-start' />
                          {t('Delete')}
                        </TooltipTrigger>
                        <TooltipContent>
                          {account.channel_count > 0
                            ? t('Unbind all channels before deleting')
                            : t('Delete')}
                        </TooltipContent>
                      </Tooltip>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </div>

      <UpstreamBillingAccountDrawer
        open={editorOpen}
        account={editingAccount}
        onOpenChange={handleEditorOpenChange}
      />
      <UpstreamBillingAccountStatsDialog
        open={statsAccount !== null}
        account={statsAccount}
        onOpenChange={(open) => !open && setStatsAccount(null)}
      />
      <ConfirmDialog
        open={deletingAccount !== null}
        onOpenChange={(open) => !open && setDeletingAccount(null)}
        title={t('Delete upstream account?')}
        desc={t(
          'This removes the stored billing credentials and cannot be undone.'
        )}
        destructive
        handleConfirm={handleDeleteAccount}
      />
    </TooltipProvider>
  )
}
