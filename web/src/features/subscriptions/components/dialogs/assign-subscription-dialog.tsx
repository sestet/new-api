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
import { useMutation, useQuery } from '@tanstack/react-query'
import { Check, LoaderCircle } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getUsers, searchUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'
import { useDebounce } from '@/hooks/use-debounce'
import { cn } from '@/lib/utils'

import { createUserSubscription, getAdminPlans } from '../../api'
import { useSubscriptions } from '../subscriptions-provider'

export function AssignSubscriptionDialog() {
  const { t } = useTranslation()
  const { open, setOpen, triggerRefresh, refreshTrigger, complianceConfirmed } =
    useSubscriptions()
  const isOpen = open === 'assign'
  const [search, setSearch] = useState('')
  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [selectedPlanId, setSelectedPlanId] = useState('')
  const debouncedSearch = useDebounce(search, 300)

  const { data: plans = [] } = useQuery({
    queryKey: ['admin-subscription-plans', refreshTrigger],
    queryFn: async () => {
      const result = await getAdminPlans()
      return result.data || []
    },
    enabled: isOpen,
  })
  const { data: users = [], isFetching } = useQuery({
    queryKey: ['subscription-user-search', debouncedSearch],
    queryFn: async () => {
      const result = debouncedSearch.trim()
        ? await searchUsers({
            keyword: debouncedSearch,
            p: 1,
            page_size: 10,
          })
        : await getUsers({ p: 1, page_size: 10 })
      return result.data?.items || []
    },
    enabled: isOpen,
    placeholderData: (previous) => previous,
  })
  const assignMutation = useMutation({
    mutationFn: async () => {
      if (!selectedUser || !selectedPlanId) {
        throw new Error(t('Please select a user and subscription plan'))
      }
      return createUserSubscription(selectedUser.id, {
        plan_id: Number(selectedPlanId),
      })
    },
    onSuccess: (result) => {
      if (!result.success) {
        toast.error(result.message || t('Operation failed'))
        return
      }
      toast.success(result.data?.message || t('Added successfully'))
      triggerRefresh()
      setOpen(null)
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })
  const resetAssignMutation = assignMutation.reset

  useEffect(() => {
    if (!isOpen) return
    setSearch('')
    setSelectedUser(null)
    setSelectedPlanId('')
    resetAssignMutation()
  }, [isOpen, resetAssignMutation])

  return (
    <Dialog open={isOpen} onOpenChange={(next) => !next && setOpen(null)}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Assign Subscription')}</DialogTitle>
          <DialogDescription>
            {t('Select a user and plan to create a subscription directly.')}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='space-y-1.5'>
            <label
              htmlFor='subscription-user-search'
              className='text-sm font-medium'
            >
              {t('User')}
            </label>
            <Command shouldFilter={false} className='rounded-lg border'>
              <CommandInput
                id='subscription-user-search'
                value={search}
                onValueChange={setSearch}
                placeholder={t('Search by username, email or user ID...')}
              />
              <CommandList className='h-52 max-h-52'>
                <CommandEmpty>
                  {isFetching ? t('Loading...') : t('No Users Found')}
                </CommandEmpty>
                <CommandGroup>
                  {users.map((user) => (
                    <CommandItem
                      key={user.id}
                      value={`${user.id}-${user.username}`}
                      onSelect={() => setSelectedUser(user)}
                      className='gap-3 px-3 py-2.5'
                    >
                      <Check
                        className={cn(
                          'size-4',
                          selectedUser?.id === user.id
                            ? 'opacity-100'
                            : 'opacity-0'
                        )}
                      />
                      <span className='min-w-0 flex-1'>
                        <span className='block truncate font-medium'>
                          {user.username}
                        </span>
                        <span className='text-muted-foreground block truncate text-xs'>
                          {user.display_name || user.email || '-'} · ID{' '}
                          {user.id}
                        </span>
                      </span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </div>

          <div className='space-y-1.5'>
            <div id='subscription-plan-label' className='text-sm font-medium'>
              {t('Plan')}
            </div>
            <Select
              value={selectedPlanId}
              onValueChange={(value) =>
                value !== null && setSelectedPlanId(value)
              }
              items={plans.map((record) => ({
                value: String(record.plan.id),
                label: record.plan.title,
              }))}
            >
              <SelectTrigger
                className='w-full'
                aria-labelledby='subscription-plan-label'
              >
                <SelectValue placeholder={t('Select subscription plan')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {plans.map((record) => (
                    <SelectItem
                      key={record.plan.id}
                      value={String(record.plan.id)}
                    >
                      {record.plan.title}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => setOpen(null)}>
            {t('Cancel')}
          </Button>
          <Button
            disabled={
              !complianceConfirmed ||
              !selectedUser ||
              !selectedPlanId ||
              assignMutation.isPending
            }
            onClick={() => assignMutation.mutate()}
          >
            {assignMutation.isPending ? (
              <LoaderCircle className='animate-spin' />
            ) : null}
            {t('Assign Subscription')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
