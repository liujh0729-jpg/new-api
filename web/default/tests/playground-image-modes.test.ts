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
import { describe, expect, test } from 'bun:test'
import { getPlaygroundModelEndpointType } from '../src/features/playground/api'
import { ENDPOINT_TEMPLATES } from '../src/features/models/constants'
import {
  DEFAULT_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
  isImagePlaygroundMode,
  isReferenceImagePlaygroundMode,
  usesImageEditsEndpoint,
} from '../src/features/playground/constants'
import { createPlaygroundConversation } from '../src/features/playground/lib/storage'

describe('Playground image modes', () => {
  test('uses distinct model endpoint types', () => {
    expect(getPlaygroundModelEndpointType('image')).toBe('image-generation')
    expect(getPlaygroundModelEndpointType('image_to_image')).toBe(
      'image-to-image'
    )
    expect(getPlaygroundModelEndpointType('image_edit')).toBe('image-edit')
  })

  test('only reference modes require image attachments', () => {
    expect(isImagePlaygroundMode('image')).toBe(true)
    expect(isImagePlaygroundMode('image_to_image')).toBe(true)
    expect(isImagePlaygroundMode('image_edit')).toBe(true)
    expect(isReferenceImagePlaygroundMode('image')).toBe(false)
    expect(isReferenceImagePlaygroundMode('image_to_image')).toBe(true)
    expect(isReferenceImagePlaygroundMode('image_edit')).toBe(true)
  })

  test('routes every reference-image request through the OpenAI edits transport', () => {
    expect(usesImageEditsEndpoint('image')).toBe(false)
    expect(usesImageEditsEndpoint('image_to_image')).toBe(true)
    expect(usesImageEditsEndpoint('image_edit')).toBe(true)
    expect(usesImageEditsEndpoint('image', 1)).toBe(true)
    expect(ENDPOINT_TEMPLATES['image-to-image']?.path).toBe(
      '/v1/images/edits'
    )
    expect(ENDPOINT_TEMPLATES['image-edit']?.path).toBe('/v1/images/edits')
  })

  test('keeps image-to-image and editing conversations distinct', () => {
    const imageToImage = createPlaygroundConversation(
      { ...DEFAULT_CONFIG, mode: 'image_to_image' },
      DEFAULT_PARAMETER_ENABLED
    )
    const imageEdit = createPlaygroundConversation(
      { ...DEFAULT_CONFIG, mode: 'image_edit' },
      DEFAULT_PARAMETER_ENABLED
    )

    expect(imageToImage.title).toBe('Image-to-image conversation')
    expect(imageEdit.title).toBe('Image editing conversation')
  })
})
