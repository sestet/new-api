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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { gptImagePlaygroundLayoutClasses } from '../layout.ts'

describe('Image Playground layout', () => {
  test('lets the task surface scroll vertically without moving the input bar', () => {
    const taskSurfaceClasses =
      gptImagePlaygroundLayoutClasses.taskSurface.split(' ')

    assert.ok(taskSurfaceClasses.includes('overflow-y-auto'))
    assert.ok(taskSurfaceClasses.includes('overscroll-contain'))
    assert.ok(!taskSurfaceClasses.includes('overflow-hidden'))
  })

  test('keeps the controls close to the page header without duplicate margins', () => {
    const taskContentClasses =
      gptImagePlaygroundLayoutClasses.taskContent.split(' ')
    const searchControlClasses =
      gptImagePlaygroundLayoutClasses.searchControls.split(' ')

    assert.ok(taskContentClasses.includes('py-3'))
    assert.ok(taskContentClasses.includes('gap-4'))
    assert.ok(!searchControlClasses.some((className) => className.startsWith('mt-')))
    assert.ok(!searchControlClasses.some((className) => className.startsWith('mb-')))
  })
})
