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
import { useMemo, type KeyboardEventHandler } from 'react'

import { Combobox } from '@/components/ui/combobox'

import { LogsFilterInput } from './logs-filter-toolbar'

type UsageFilterOption = {
  value: string
  label: string
}

interface UsageFilterComboboxProps {
  options: UsageFilterOption[]
  value: string
  placeholder: string
  masked?: boolean
  onValueChange: (value: string) => void
  onKeyDown?: KeyboardEventHandler<HTMLInputElement>
}

export function UsageFilterCombobox(props: UsageFilterComboboxProps) {
  const options = useMemo(() => {
    if (
      !props.value ||
      props.options.some((option) => option.value === props.value)
    ) {
      return props.options
    }
    return [{ value: props.value, label: props.value }, ...props.options]
  }, [props.options, props.value])

  if (props.masked) {
    return (
      <LogsFilterInput
        type='password'
        placeholder={props.placeholder}
        value={props.value}
        onChange={(event) => props.onValueChange(event.target.value)}
        onKeyDown={props.onKeyDown}
      />
    )
  }

  return (
    <Combobox
      options={options}
      value={props.value}
      onValueChange={(value) => props.onValueChange(value ?? '')}
      placeholder={props.placeholder}
      searchPlaceholder={props.placeholder}
      emptyText='No results found.'
      allowCustomValue
      className='h-8 min-w-0 text-sm leading-5'
    />
  )
}
