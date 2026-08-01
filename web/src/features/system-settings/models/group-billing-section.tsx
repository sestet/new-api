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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useMemo } from 'react'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
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
import { Switch } from '@/components/ui/switch'
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

import { updateGroupRatioOptions } from '../api'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import {
  groupBillingSchema,
  parseGroupBillingDefaults,
  serializeGroupBillingValues,
  type GroupBillingFormInput,
  type GroupBillingFormValues,
} from './group-billing-config'

type GroupBillingSectionProps = {
  defaultValues: {
    GroupRatio: string
    GroupGroupRatio: string
    GroupSpecialUsableGroup: string
    UserUsableGroups: string
    AutoGroups: string
    MaxTokenAutoGroups: number
    DefaultUseAutoGroup: boolean
  }
}

export function GroupBillingSection(props: GroupBillingSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const defaults = useMemo(
    () => parseGroupBillingDefaults(props.defaultValues),
    [props.defaultValues]
  )
  const form = useForm<GroupBillingFormInput, unknown, GroupBillingFormValues>({
    resolver: zodResolver(groupBillingSchema),
    defaultValues: defaults,
  })
  useResetForm(form, defaults)

  const groupFields = useFieldArray({ control: form.control, name: 'groups' })
  const overrideFields = useFieldArray({
    control: form.control,
    name: 'overrides',
  })
  const specialUsableRuleFields = useFieldArray({
    control: form.control,
    name: 'specialUsableRules',
  })
  const autoGroupFields = useFieldArray({
    control: form.control,
    name: 'autoGroups',
  })
  const watchedGroups = useWatch({ control: form.control, name: 'groups' })
  const watchedAutoGroups = useWatch({
    control: form.control,
    name: 'autoGroups',
  })
  const groupItems = useMemo(() => {
    const seen = new Set<string>()
    return (watchedGroups ?? []).flatMap((group) => {
      const name = group.name?.trim()
      if (!name || seen.has(name)) return []
      seen.add(name)
      return [{ value: name, label: name }]
    })
  }, [watchedGroups])
  const selectedAutoGroups = useMemo(
    () => new Set((watchedAutoGroups ?? []).map((item) => item.group)),
    [watchedAutoGroups]
  )
  const nextAutoGroup = groupItems.find(
    (item) => !selectedAutoGroups.has(item.value)
  )

  const saveMutation = useMutation({ mutationFn: updateGroupRatioOptions })
  const onSubmit = async (values: GroupBillingFormValues) => {
    try {
      const result = await saveMutation.mutateAsync(
        serializeGroupBillingValues(values)
      )
      if (!result.success) {
        toast.error(result.message || t('Failed to update setting'))
        return
      }
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      form.reset(values)
      toast.success(t('Setting updated successfully'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to update setting')
      )
    }
  }

  return (
    <SettingsSection title={t('Group Billing')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={() => form.reset(defaults)}
            isSaving={saveMutation.isPending}
            isSaveDisabled={!form.formState.isDirty}
            isResetDisabled={!form.formState.isDirty}
          />

          <div className='flex min-w-0 flex-col gap-3'>
            <div className='flex items-center justify-between gap-3'>
              <h4 className='text-sm font-medium'>{t('Base group ratios')}</h4>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={() =>
                  groupFields.append({
                    name: '',
                    description: '',
                    ratio: 1,
                    globallyUsable: true,
                  })
                }
              >
                <Plus data-icon='inline-start' />
                {t('Add group')}
              </Button>
            </div>
            <div className='overflow-hidden rounded-md border'>
              <Table className='min-w-[680px]'>
                <TableHeader>
                  <TableRow>
                    <TableHead className='w-[28%]'>{t('Group name')}</TableHead>
                    <TableHead>{t('Description')}</TableHead>
                    <TableHead className='w-36'>{t('Base ratio')}</TableHead>
                    <TableHead className='w-32 text-center'>
                      {t('User selectable')}
                    </TableHead>
                    <TableHead className='w-14' />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {groupFields.fields.map((group, index) => {
                    const isDefault = group.name === 'default'
                    return (
                      <TableRow key={group.id}>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`groups.${index}.name`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Input {...field} disabled={isDefault} />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`groups.${index}.description`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Input {...field} />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`groups.${index}.ratio`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Input
                                    type='number'
                                    min='0'
                                    step='any'
                                    value={String(field.value ?? '')}
                                    onChange={field.onChange}
                                    onBlur={field.onBlur}
                                    name={field.name}
                                    ref={field.ref}
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`groups.${index}.globallyUsable`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <div className='flex justify-center'>
                                    <Checkbox
                                      checked={field.value}
                                      disabled={isDefault}
                                      aria-label={t('User selectable')}
                                      onCheckedChange={(checked) =>
                                        field.onChange(checked === true)
                                      }
                                    />
                                  </div>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <Button
                            type='button'
                            size='icon-sm'
                            variant='ghost'
                            aria-label={t('Delete group')}
                            disabled={isDefault}
                            onClick={() => groupFields.remove(index)}
                          >
                            <Trash2 />
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          </div>

          <div className='flex min-w-0 flex-col gap-3'>
            <div className='flex items-center justify-between gap-3'>
              <h4 className='text-sm font-medium'>
                {t('Auto assignment order')}
              </h4>
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={!nextAutoGroup}
                onClick={() => {
                  if (nextAutoGroup) {
                    autoGroupFields.append({ group: nextAutoGroup.value })
                  }
                }}
              >
                <Plus data-icon='inline-start' />
                {t('Add group')}
              </Button>
            </div>
            <div className='overflow-hidden rounded-md border'>
              <Table className='min-w-[520px]'>
                <TableHeader>
                  <TableRow>
                    <TableHead className='w-24'>{t('Priority')}</TableHead>
                    <TableHead>{t('Group')}</TableHead>
                    <TableHead className='w-28' />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {autoGroupFields.fields.map((autoGroup, index) => (
                    <TableRow key={autoGroup.id}>
                      <TableCell className='text-muted-foreground tabular-nums'>
                        {index + 1}
                      </TableCell>
                      <TableCell>
                        <FormField
                          control={form.control}
                          name={`autoGroups.${index}.group`}
                          render={({ field }) => {
                            const rowItems = groupItems.filter(
                              (item) =>
                                item.value === field.value ||
                                !selectedAutoGroups.has(item.value)
                            )
                            return (
                              <FormItem>
                                <FormControl>
                                  <Select
                                    items={rowItems}
                                    value={field.value}
                                    onValueChange={(value) =>
                                      value !== null && field.onChange(value)
                                    }
                                  >
                                    <SelectTrigger className='w-full'>
                                      <SelectValue
                                        placeholder={t('Select a group')}
                                      />
                                    </SelectTrigger>
                                    <SelectContent alignItemWithTrigger={false}>
                                      <SelectGroup>
                                        {rowItems.map((item) => (
                                          <SelectItem
                                            key={item.value}
                                            value={item.value}
                                          >
                                            {item.label}
                                          </SelectItem>
                                        ))}
                                      </SelectGroup>
                                    </SelectContent>
                                  </Select>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )
                          }}
                        />
                      </TableCell>
                      <TableCell>
                        <TooltipProvider>
                          <div className='flex justify-end gap-0.5'>
                            <Tooltip>
                              <TooltipTrigger
                                render={
                                  <Button
                                    type='button'
                                    size='icon-sm'
                                    variant='ghost'
                                    aria-label={t('Move up')}
                                    disabled={index === 0}
                                    onClick={() =>
                                      autoGroupFields.move(index, index - 1)
                                    }
                                  >
                                    <ArrowUp />
                                  </Button>
                                }
                              />
                              <TooltipContent>{t('Move up')}</TooltipContent>
                            </Tooltip>
                            <Tooltip>
                              <TooltipTrigger
                                render={
                                  <Button
                                    type='button'
                                    size='icon-sm'
                                    variant='ghost'
                                    aria-label={t('Move down')}
                                    disabled={
                                      index ===
                                      autoGroupFields.fields.length - 1
                                    }
                                    onClick={() =>
                                      autoGroupFields.move(index, index + 1)
                                    }
                                  >
                                    <ArrowDown />
                                  </Button>
                                }
                              />
                              <TooltipContent>{t('Move down')}</TooltipContent>
                            </Tooltip>
                            <Button
                              type='button'
                              size='icon-sm'
                              variant='ghost'
                              aria-label={t('Delete group')}
                              disabled={autoGroupFields.fields.length === 1}
                              onClick={() => autoGroupFields.remove(index)}
                            >
                              <Trash2 />
                            </Button>
                          </div>
                        </TooltipProvider>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <FormField
              control={form.control}
              name='maxTokenAutoGroups'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Maximum Auto groups per API key')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min='1'
                      step='1'
                      value={String(field.value ?? '')}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                      name={field.name}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='defaultUseAutoGroup'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Default to auto groups')}</FormLabel>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      aria-label={t('Default to auto groups')}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>

          <div className='flex min-w-0 flex-col gap-3'>
            <div className='flex items-center justify-between gap-3'>
              <h4 className='text-sm font-medium'>
                {t('Group ratio overrides')}
              </h4>
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={groupItems.length === 0}
                onClick={() => {
                  const firstGroup = groupItems[0]?.value ?? ''
                  overrideFields.append({
                    userGroup: firstGroup,
                    usingGroup: firstGroup,
                    ratio: 1,
                  })
                }}
              >
                <Plus data-icon='inline-start' />
                {t('Add override')}
              </Button>
            </div>
            <div className='overflow-hidden rounded-md border'>
              <Table className='min-w-[680px]'>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('User group')}</TableHead>
                    <TableHead>{t('Using group')}</TableHead>
                    <TableHead className='w-36'>
                      {t('Override ratio')}
                    </TableHead>
                    <TableHead className='w-14' />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {overrideFields.fields.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className='text-muted-foreground h-20 text-center'
                      >
                        {t('No overrides configured')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    overrideFields.fields.map((override, index) => (
                      <TableRow key={override.id}>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`overrides.${index}.userGroup`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Select
                                    items={groupItems}
                                    value={field.value}
                                    onValueChange={(value) =>
                                      value !== null && field.onChange(value)
                                    }
                                  >
                                    <SelectTrigger className='w-full'>
                                      <SelectValue
                                        placeholder={t('Select a group')}
                                      />
                                    </SelectTrigger>
                                    <SelectContent alignItemWithTrigger={false}>
                                      <SelectGroup>
                                        {groupItems.map((item) => (
                                          <SelectItem
                                            key={item.value}
                                            value={item.value}
                                          >
                                            {item.label}
                                          </SelectItem>
                                        ))}
                                      </SelectGroup>
                                    </SelectContent>
                                  </Select>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`overrides.${index}.usingGroup`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Select
                                    items={groupItems}
                                    value={field.value}
                                    onValueChange={(value) =>
                                      value !== null && field.onChange(value)
                                    }
                                  >
                                    <SelectTrigger className='w-full'>
                                      <SelectValue
                                        placeholder={t('Select a group')}
                                      />
                                    </SelectTrigger>
                                    <SelectContent alignItemWithTrigger={false}>
                                      <SelectGroup>
                                        {groupItems.map((item) => (
                                          <SelectItem
                                            key={item.value}
                                            value={item.value}
                                          >
                                            {item.label}
                                          </SelectItem>
                                        ))}
                                      </SelectGroup>
                                    </SelectContent>
                                  </Select>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`overrides.${index}.ratio`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Input
                                    type='number'
                                    min='0'
                                    step='any'
                                    value={String(field.value ?? '')}
                                    onChange={field.onChange}
                                    onBlur={field.onBlur}
                                    name={field.name}
                                    ref={field.ref}
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <Button
                            type='button'
                            size='icon-sm'
                            variant='ghost'
                            aria-label={t('Delete override')}
                            onClick={() => overrideFields.remove(index)}
                          >
                            <Trash2 />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </div>

          <div className='flex min-w-0 flex-col gap-3'>
            <div className='flex items-center justify-between gap-3'>
              <h4 className='text-sm font-medium'>
                {t('Special usable group rules')}
              </h4>
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={groupItems.length === 0}
                onClick={() => {
                  const firstGroup = groupItems[0]?.value ?? ''
                  specialUsableRuleFields.append({
                    userGroup: firstGroup,
                    targetGroup: firstGroup,
                    visible: true,
                  })
                }}
              >
                <Plus data-icon='inline-start' />
                {t('Add rule')}
              </Button>
            </div>
            <div className='overflow-hidden rounded-md border'>
              <Table className='min-w-[680px]'>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('User group')}</TableHead>
                    <TableHead>{t('Target group')}</TableHead>
                    <TableHead className='w-44'>{t('Visibility')}</TableHead>
                    <TableHead className='w-14' />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {specialUsableRuleFields.fields.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className='text-muted-foreground h-20 text-center'
                      >
                        {t('No special usable group rules configured')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    specialUsableRuleFields.fields.map((rule, index) => (
                      <TableRow key={rule.id}>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`specialUsableRules.${index}.userGroup`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Select
                                    items={groupItems}
                                    value={field.value}
                                    onValueChange={(value) =>
                                      value !== null && field.onChange(value)
                                    }
                                  >
                                    <SelectTrigger className='w-full'>
                                      <SelectValue
                                        placeholder={t('Select a group')}
                                      />
                                    </SelectTrigger>
                                    <SelectContent alignItemWithTrigger={false}>
                                      <SelectGroup>
                                        {groupItems.map((item) => (
                                          <SelectItem
                                            key={item.value}
                                            value={item.value}
                                          >
                                            {item.label}
                                          </SelectItem>
                                        ))}
                                      </SelectGroup>
                                    </SelectContent>
                                  </Select>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`specialUsableRules.${index}.targetGroup`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Select
                                    items={groupItems}
                                    value={field.value}
                                    onValueChange={(value) =>
                                      value !== null && field.onChange(value)
                                    }
                                  >
                                    <SelectTrigger className='w-full'>
                                      <SelectValue
                                        placeholder={t('Select a group')}
                                      />
                                    </SelectTrigger>
                                    <SelectContent alignItemWithTrigger={false}>
                                      <SelectGroup>
                                        {groupItems.map((item) => (
                                          <SelectItem
                                            key={item.value}
                                            value={item.value}
                                          >
                                            {item.label}
                                          </SelectItem>
                                        ))}
                                      </SelectGroup>
                                    </SelectContent>
                                  </Select>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <FormField
                            control={form.control}
                            name={`specialUsableRules.${index}.visible`}
                            render={({ field }) => (
                              <FormItem>
                                <FormControl>
                                  <Select
                                    value={field.value ? 'visible' : 'hidden'}
                                    onValueChange={(value) =>
                                      value !== null &&
                                      field.onChange(value === 'visible')
                                    }
                                  >
                                    <SelectTrigger className='w-full'>
                                      <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent alignItemWithTrigger={false}>
                                      <SelectGroup>
                                        <SelectItem value='visible'>
                                          {t('Extra visible')}
                                        </SelectItem>
                                        <SelectItem value='hidden'>
                                          {t('Hidden')}
                                        </SelectItem>
                                      </SelectGroup>
                                    </SelectContent>
                                  </Select>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TableCell>
                        <TableCell>
                          <Button
                            type='button'
                            size='icon-sm'
                            variant='ghost'
                            aria-label={t('Delete special rule')}
                            onClick={() =>
                              specialUsableRuleFields.remove(index)
                            }
                          >
                            <Trash2 />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
