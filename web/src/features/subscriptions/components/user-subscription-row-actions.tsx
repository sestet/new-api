/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Ban, CreditCard, RotateCcw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableRowActionMenu } from '@/components/data-table'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'

import {
  deleteUserSubscription,
  invalidateUserSubscription,
  resetUserSubscriptionsByPlan,
} from '../api'
import type { AdminUserSubscription } from '../types'
import { UserSubscriptionsDialog } from './dialogs/user-subscriptions-dialog'
import { useSubscriptions } from './subscriptions-provider'

type PendingAction = 'reset' | 'invalidate' | 'delete'

export function UserSubscriptionRowActions(props: {
  subscription: AdminUserSubscription
}) {
  const { t } = useTranslation()
  const { triggerRefresh } = useSubscriptions()
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [manageOpen, setManageOpen] = useState(false)
  const isActive = props.subscription.status === 'active'

  const handleConfirm = async () => {
    if (!pendingAction) return
    setSubmitting(true)
    try {
      if (pendingAction === 'reset') {
        const result = await resetUserSubscriptionsByPlan(
          props.subscription.user_id,
          {
            plan_id: props.subscription.plan_id,
            advance_reset_time: true,
          }
        )
        if (!result.success) {
          toast.error(result.message || t('Operation failed'))
          return
        }
        toast.success(
          t('Reset {{count}} active subscriptions', {
            count: result.data?.reset_count || 0,
          })
        )
      } else if (pendingAction === 'invalidate') {
        const result = await invalidateUserSubscription(props.subscription.id)
        if (!result.success) {
          toast.error(result.message || t('Operation failed'))
          return
        }
        toast.success(result.data?.message || t('Has been invalidated'))
      } else {
        const result = await deleteUserSubscription(props.subscription.id)
        if (!result.success) {
          toast.error(result.message || t('Operation failed'))
          return
        }
        toast.success(t('Deleted'))
      }
      triggerRefresh()
      setPendingAction(null)
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setSubmitting(false)
    }
  }

  let confirmTitle = t('Reset subscription quota')
  let confirmDescription = t(
    'Reset active {{plan}} subscriptions for this user?',
    { plan: props.subscription.plan_title }
  )
  if (pendingAction === 'invalidate') {
    confirmTitle = t('Confirm invalidate')
    confirmDescription = t(
      'After invalidating, this subscription will be immediately deactivated. Historical records are not affected. Continue?'
    )
  } else if (pendingAction === 'delete') {
    confirmTitle = t('Confirm delete')
    confirmDescription = t(
      'Deleting will permanently remove this subscription record (including benefit details). Continue?'
    )
  }

  return (
    <>
      <DataTableRowActionMenu ariaLabel={t('Actions')}>
        <DropdownMenuItem onClick={() => setManageOpen(true)}>
          {t('Manage Subscriptions')}
          <DropdownMenuShortcut>
            <CreditCard />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={!isActive}
          onClick={() => setPendingAction('reset')}
        >
          {t('Reset quota')}
          <DropdownMenuShortcut>
            <RotateCcw />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={!isActive}
          onClick={() => setPendingAction('invalidate')}
        >
          {t('Invalidate')}
          <DropdownMenuShortcut>
            <Ban />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant='destructive'
          onClick={() => setPendingAction('delete')}
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DataTableRowActionMenu>

      <UserSubscriptionsDialog
        open={manageOpen}
        onOpenChange={setManageOpen}
        user={{
          id: props.subscription.user_id,
          username: props.subscription.username,
        }}
        onSuccess={triggerRefresh}
      />

      {pendingAction ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setPendingAction(null)}
          title={confirmTitle}
          desc={confirmDescription}
          confirmText={
            pendingAction === 'reset' ? t('Reset quota') : t('Continue')
          }
          destructive={pendingAction === 'delete'}
          handleConfirm={handleConfirm}
          isLoading={submitting}
        />
      ) : null}
    </>
  )
}
