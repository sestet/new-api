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
import * as z from 'zod'

import type { UpdateGroupRatioOptionsRequest } from '../types'

const ratioSchema = z.coerce
  .number()
  .finite('Ratio must be a finite non-negative number')
  .min(0, 'Ratio must be a finite non-negative number')

export const groupBillingSchema = z
  .object({
    groups: z
      .array(
        z.object({
          name: z.string().trim().min(1, 'Group name is required'),
          description: z
            .string()
            .trim()
            .min(1, 'Group description is required'),
          ratio: ratioSchema,
          globallyUsable: z.boolean(),
        })
      )
      .min(1, 'At least one group is required'),
    overrides: z.array(
      z.object({
        userGroup: z.string().trim().min(1, 'Select a group'),
        usingGroup: z.string().trim().min(1, 'Select a group'),
        ratio: ratioSchema,
      })
    ),
    specialUsableRules: z.array(
      z.object({
        userGroup: z.string().trim().min(1, 'Select a group'),
        targetGroup: z.string().trim().min(1, 'Select a group'),
        visible: z.boolean(),
      })
    ),
    autoGroups: z
      .array(
        z.object({
          group: z.string().trim().min(1, 'Select a group'),
        })
      )
      .min(1, 'At least one group is required'),
    defaultUseAutoGroup: z.boolean(),
  })
  .superRefine((values, context) => {
    const groupNames = new Set<string>()
    values.groups.forEach((group, index) => {
      if (groupNames.has(group.name)) {
        context.addIssue({
          code: 'custom',
          path: ['groups', index, 'name'],
          message: 'Group names must be unique',
        })
      }
      groupNames.add(group.name)
    })
    if (!groupNames.has('default')) {
      context.addIssue({
        code: 'custom',
        path: ['groups'],
        message: 'Default group is required',
      })
    }
    const autoGroups = new Set<string>()
    values.autoGroups.forEach((autoGroup, index) => {
      if (!groupNames.has(autoGroup.group)) {
        context.addIssue({
          code: 'custom',
          path: ['autoGroups', index, 'group'],
          message: 'The selected group no longer exists',
        })
      }
      if (autoGroups.has(autoGroup.group)) {
        context.addIssue({
          code: 'custom',
          path: ['autoGroups', index, 'group'],
          message: 'Group names must be unique',
        })
      }
      autoGroups.add(autoGroup.group)
    })
    const pairs = new Set<string>()
    values.overrides.forEach((override, index) => {
      if (!groupNames.has(override.userGroup)) {
        context.addIssue({
          code: 'custom',
          path: ['overrides', index, 'userGroup'],
          message: 'The selected group no longer exists',
        })
      }
      if (!groupNames.has(override.usingGroup)) {
        context.addIssue({
          code: 'custom',
          path: ['overrides', index, 'usingGroup'],
          message: 'The selected group no longer exists',
        })
      }
      const pair = `${override.userGroup}\u0000${override.usingGroup}`
      if (pairs.has(pair)) {
        context.addIssue({
          code: 'custom',
          path: ['overrides', index, 'usingGroup'],
          message: 'Override pairs must be unique',
        })
      }
      pairs.add(pair)
    })

    const specialPairs = new Set<string>()
    values.specialUsableRules.forEach((rule, index) => {
      if (!groupNames.has(rule.userGroup)) {
        context.addIssue({
          code: 'custom',
          path: ['specialUsableRules', index, 'userGroup'],
          message: 'The selected group no longer exists',
        })
      }
      if (!groupNames.has(rule.targetGroup)) {
        context.addIssue({
          code: 'custom',
          path: ['specialUsableRules', index, 'targetGroup'],
          message: 'The selected group no longer exists',
        })
      }
      const pair = `${rule.userGroup}\u0000${rule.targetGroup}`
      if (specialPairs.has(pair)) {
        context.addIssue({
          code: 'custom',
          path: ['specialUsableRules', index, 'targetGroup'],
          message: 'Special usable group rules must be unique',
        })
      }
      specialPairs.add(pair)
    })
  })

export type GroupBillingFormValues = z.output<typeof groupBillingSchema>
export type GroupBillingFormInput = z.input<typeof groupBillingSchema>

function parseRecord(value: string): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(value)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    // Invalid legacy values fall back to a usable default configuration.
  }
  return {}
}

function parseStringArray(value: string): string[] {
  try {
    const parsed: unknown = JSON.parse(value)
    if (Array.isArray(parsed)) {
      return parsed.filter((item): item is string => typeof item === 'string')
    }
  } catch {
    // Invalid legacy values fall back to the default group.
  }
  return []
}

export function parseGroupBillingDefaults(defaults: {
  GroupRatio: string
  GroupGroupRatio: string
  GroupSpecialUsableGroup: string
  UserUsableGroups: string
  AutoGroups: string
  DefaultUseAutoGroup: boolean
}): GroupBillingFormInput {
  const rawRatios = parseRecord(defaults.GroupRatio)
  const rawDescriptions = parseRecord(defaults.UserUsableGroups)
  const names = Object.keys(rawRatios).filter((name) => {
    const ratio = rawRatios[name]
    return (
      name.trim() !== '' &&
      typeof ratio === 'number' &&
      Number.isFinite(ratio) &&
      ratio >= 0
    )
  })
  if (!names.includes('default')) {
    names.push('default')
    rawRatios.default = 1
  }
  if (typeof rawDescriptions.default !== 'string') {
    rawDescriptions.default = 'default'
  }
  names.sort((left, right) => {
    if (left === 'default') return -1
    if (right === 'default') return 1
    return left.localeCompare(right)
  })

  const nameSet = new Set(names)
  const overrides: GroupBillingFormInput['overrides'] = []
  const rawOverrides = parseRecord(defaults.GroupGroupRatio)
  Object.entries(rawOverrides).forEach(([userGroup, value]) => {
    if (!nameSet.has(userGroup) || !value || typeof value !== 'object') return
    Object.entries(value as Record<string, unknown>).forEach(
      ([usingGroup, ratio]) => {
        if (
          nameSet.has(usingGroup) &&
          typeof ratio === 'number' &&
          Number.isFinite(ratio) &&
          ratio >= 0
        ) {
          overrides.push({ userGroup, usingGroup, ratio })
        }
      }
    )
  })

  const specialUsableRules: GroupBillingFormInput['specialUsableRules'] = []
  const rawSpecialRules = parseRecord(defaults.GroupSpecialUsableGroup)
  Object.entries(rawSpecialRules).forEach(([userGroup, value]) => {
    if (!nameSet.has(userGroup) || !value || typeof value !== 'object') return
    Object.keys(value as Record<string, unknown>).forEach((rawTargetGroup) => {
      const visible = !rawTargetGroup.startsWith('-:')
      const targetGroup =
        rawTargetGroup.startsWith('+:') || rawTargetGroup.startsWith('-:')
          ? rawTargetGroup.slice(2)
          : rawTargetGroup
      if (nameSet.has(targetGroup)) {
        specialUsableRules.push({ userGroup, targetGroup, visible })
      }
    })
  })

  const specialDescriptions = new Map<string, string>()
  Object.values(rawSpecialRules).forEach((value) => {
    if (!value || typeof value !== 'object') return
    Object.entries(value as Record<string, unknown>).forEach(
      ([rawTargetGroup, description]) => {
        if (
          rawTargetGroup.startsWith('-:') ||
          typeof description !== 'string'
        ) {
          return
        }
        const targetGroup = rawTargetGroup.startsWith('+:')
          ? rawTargetGroup.slice(2)
          : rawTargetGroup
        if (
          description.trim() !== '' &&
          !specialDescriptions.has(targetGroup)
        ) {
          specialDescriptions.set(targetGroup, description)
        }
      }
    )
  })

  const autoGroupNames: string[] = []
  const seenAutoGroups = new Set<string>()
  parseStringArray(defaults.AutoGroups).forEach((group) => {
    if (!nameSet.has(group) || seenAutoGroups.has(group)) return
    seenAutoGroups.add(group)
    autoGroupNames.push(group)
  })
  if (autoGroupNames.length === 0) {
    autoGroupNames.push('default')
  }

  return {
    groups: names.map((name) => ({
      name,
      description:
        typeof rawDescriptions[name] === 'string'
          ? (rawDescriptions[name] as string)
          : (specialDescriptions.get(name) ?? name),
      ratio: rawRatios[name] as number,
      globallyUsable: Object.hasOwn(rawDescriptions, name),
    })),
    overrides,
    specialUsableRules,
    autoGroups: autoGroupNames.map((group) => ({ group })),
    defaultUseAutoGroup: defaults.DefaultUseAutoGroup,
  }
}

export function serializeGroupBillingValues(
  values: GroupBillingFormValues
): UpdateGroupRatioOptionsRequest {
  const groupRatios: Record<string, number> = {}
  const groupDescriptions: Record<string, string> = {}
  const usableGroups: Record<string, string> = {}
  values.groups.forEach((group) => {
    groupRatios[group.name] = group.ratio
    groupDescriptions[group.name] = group.description
    if (group.globallyUsable) {
      usableGroups[group.name] = group.description
    }
  })

  const overrides: Record<string, Record<string, number>> = {}
  values.overrides.forEach((override) => {
    overrides[override.userGroup] ??= {}
    overrides[override.userGroup][override.usingGroup] = override.ratio
  })

  const specialUsableGroups: Record<string, Record<string, string>> = {}
  values.specialUsableRules.forEach((rule) => {
    specialUsableGroups[rule.userGroup] ??= {}
    const key = `${rule.visible ? '+:' : '-:'}${rule.targetGroup}`
    specialUsableGroups[rule.userGroup][key] = rule.visible
      ? groupDescriptions[rule.targetGroup]
      : ''
  })

  return {
    group_ratio: JSON.stringify(groupRatios),
    group_group_ratio: JSON.stringify(overrides),
    group_special_usable_group: JSON.stringify(specialUsableGroups),
    user_usable_groups: JSON.stringify(usableGroups),
    auto_groups: JSON.stringify(
      values.autoGroups.map((autoGroup) => autoGroup.group)
    ),
    default_use_auto_group: values.defaultUseAutoGroup,
  }
}
