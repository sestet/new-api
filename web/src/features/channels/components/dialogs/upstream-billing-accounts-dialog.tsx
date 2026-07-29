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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  createUpstreamBillingAccount,
  updateUpstreamBillingAccount,
  upstreamBillingAccountsQueryKey,
} from '../../api'
import type { UpstreamBillingAccount } from '../../types'

const accountSchema = z
  .object({
    name: z.string().trim().min(1, 'Account name is required').max(128),
    provider: z.enum(['new_api', 'sub2api']),
    api_base_url: z.url('Billing API Base URL must be a valid HTTP URL'),
    access_token: z.string(),
    access_token_configured: z.boolean(),
    refresh_token: z.string(),
    refresh_token_configured: z.boolean(),
    user_id: z.string(),
    enabled: z.boolean(),
  })
  .superRefine((data, ctx) => {
    let parsedURL: URL | undefined
    try {
      parsedURL = new URL(data.api_base_url)
    } catch {
      return
    }
    if (!['http:', 'https:'].includes(parsedURL.protocol)) {
      ctx.addIssue({
        code: 'custom',
        path: ['api_base_url'],
        message: 'Billing API Base URL must be a valid HTTP URL',
      })
    }
    if (
      data.provider === 'new_api' &&
      !data.access_token.trim() &&
      !data.access_token_configured
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['access_token'],
        message: 'Upstream billing access token is required',
      })
    }
    if (
      data.provider === 'sub2api' &&
      !data.refresh_token.trim() &&
      !data.refresh_token_configured
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['refresh_token'],
        message: 'Sub2API refresh token is required',
      })
    }
    if (data.user_id.trim() && !/^[1-9]\d*$/.test(data.user_id.trim())) {
      ctx.addIssue({
        code: 'custom',
        path: ['user_id'],
        message: 'Upstream account user ID must be a positive integer',
      })
    }
  })

type AccountFormValues = z.infer<typeof accountSchema>

const defaultValues: AccountFormValues = {
  name: '',
  provider: 'new_api',
  api_base_url: '',
  access_token: '',
  access_token_configured: false,
  refresh_token: '',
  refresh_token_configured: false,
  user_id: '',
  enabled: true,
}

const billingSystemOptions = [
  { label: 'new-api', value: 'new_api' },
  { label: 'sub2api', value: 'sub2api' },
]

type UpstreamBillingAccountDrawerProps = {
  open: boolean
  account: UpstreamBillingAccount | null
  onOpenChange: (open: boolean) => void
}

export function UpstreamBillingAccountDrawer(
  props: UpstreamBillingAccountDrawerProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const [saving, setSaving] = useState(false)
  const form = useForm<AccountFormValues>({
    resolver: zodResolver(accountSchema),
    defaultValues,
  })
  const provider = form.watch('provider')
  const isEditing = props.account !== null

  useEffect(() => {
    if (!props.open) {
      form.reset(defaultValues)
      return
    }
    if (!props.account) {
      form.reset(defaultValues)
      return
    }
    form.reset({
      name: props.account.name,
      provider: props.account.provider,
      api_base_url: props.account.api_base_url,
      access_token: '',
      access_token_configured: props.account.access_token_configured,
      refresh_token: '',
      refresh_token_configured: props.account.refresh_token_configured,
      user_id: props.account.user_id > 0 ? String(props.account.user_id) : '',
      enabled: props.account.enabled,
    })
  }, [form, props.account, props.open])

  const handleSave = async (values: AccountFormValues) => {
    setSaving(true)
    try {
      const payload = {
        name: values.name.trim(),
        provider: values.provider,
        api_base_url: values.api_base_url.trim().replace(/\/+$/, ''),
        access_token: values.access_token.trim() || undefined,
        refresh_token: values.refresh_token.trim() || undefined,
        user_id: Number(values.user_id.trim()) || undefined,
        enabled: values.enabled,
      }
      const response = props.account
        ? await updateUpstreamBillingAccount(props.account.id, payload)
        : await createUpstreamBillingAccount(payload)
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to save upstream account')
        )
      }
      toast.success(t('Upstream account saved'))
      await queryClient.invalidateQueries({
        queryKey: upstreamBillingAccountsQueryKey,
      })
      props.onOpenChange(false)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to save upstream account')
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-2xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isEditing
              ? t('Edit upstream account')
              : t('Create upstream account')}
          </SheetTitle>
          <SheetDescription>
            {t('These credentials are shared by every bound channel.')}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='upstream-billing-account-form'
            className={sideDrawerFormClassName()}
            onSubmit={form.handleSubmit(handleSave)}
          >
            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Account name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Example: Main upstream account')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='provider'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Billing system')}</FormLabel>
                    <Select
                      items={billingSystemOptions}
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value='new_api'>new-api</SelectItem>
                          <SelectItem value='sub2api'>sub2api</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='api_base_url'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Billing API Base URL')}</FormLabel>
                  <FormControl>
                    <Input placeholder='https://api.example.com' {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Use the exact account or billing API address; no domain guessing is performed.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {provider === 'new_api' ? (
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='access_token'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Account Access Token')}</FormLabel>
                      <FormControl>
                        <Input
                          type='password'
                          autoComplete='new-password'
                          placeholder={
                            form.getValues('access_token_configured')
                              ? '••••••••'
                              : t('Dashboard account access token')
                          }
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='user_id'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Upstream Account User ID')}</FormLabel>
                      <FormControl>
                        <Input type='number' min={1} step={1} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            ) : (
              <FormField
                control={form.control}
                name='refresh_token'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Account Refresh Token')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        autoComplete='new-password'
                        placeholder={
                          form.getValues('refresh_token_configured')
                            ? '••••••••'
                            : t('Sub2API refresh token')
                        }
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'The token is refreshed early and rotated once for all bound channels.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className={sideDrawerSwitchItemClassName()}>
                  <div>
                    <FormLabel>{t('Enable upstream account')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Disabled accounts cannot provide exact billing to bound channels.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </form>
        </Form>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='upstream-billing-account-form'
            disabled={!canEditSensitive || saving}
          >
            {saving && (
              <Loader2 data-icon='inline-start' className='animate-spin' />
            )}
            {t('Save')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
