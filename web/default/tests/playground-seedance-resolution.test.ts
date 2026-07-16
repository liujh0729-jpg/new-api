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
import {
  getSeedanceVideoProcessingChainOptionsForModel,
  getVideoResolutionOptionsForModel,
  normalizeVideoResolutionForModel,
} from '../src/features/playground/constants'

const cases = [
  {
    model: 'AP Seedance-2.0 VIP',
    options: ['480p', '720p', '1080p', '4k'],
    chains: [
      ['480p', '480p'],
      ['480p', '720p'],
      ['720p', '1080p'],
      ['1080p', '4K'],
    ],
    invalidResolution: '1440p',
    expectedDefault: '720p',
  },
  {
    model: 'AP Seedance-2.0 标准版',
    options: ['480p', '720p', '1080p', '4k'],
    chains: [
      ['480p', '480p'],
      ['480p', '720p'],
      ['720p', '1080p'],
      ['720p', '4K'],
    ],
    invalidResolution: '1440p',
    expectedDefault: '720p',
  },
  {
    model: 'AP Seedance-2.0 轻量版',
    options: ['480p', '720p', '1080p'],
    chains: [
      ['480p', '480p'],
      ['480p', '720p'],
      ['720p', '1080p'],
    ],
    invalidResolution: '4k',
    expectedDefault: '720p',
  },
  {
    model: 'AP Seedance-2.0 高性价比版',
    options: ['1080p', '4k'],
    chains: [
      ['720p', '1080p'],
      ['720p', '4K'],
    ],
    invalidResolution: '720p',
    expectedDefault: '1080p',
  },
] as const

describe('Playground AP Seedance resolution options', () => {
  for (const item of cases) {
    test(`${item.model} exposes its supported resolutions`, () => {
      expect(getVideoResolutionOptionsForModel(item.model)).toEqual(
        item.options
      )
    })

    test(`${item.model} normalizes an unsupported resolution`, () => {
      expect(
        normalizeVideoResolutionForModel(item.model, item.invalidResolution)
      ).toBe(item.expectedDefault)
    })

    test(`${item.model} exposes its processing chain labels`, () => {
      expect(
        getSeedanceVideoProcessingChainOptionsForModel(item.model).map(
          (option) => [option.sourceResolution, option.outputResolution]
        )
      ).toEqual(item.chains)
    })
  }
})
