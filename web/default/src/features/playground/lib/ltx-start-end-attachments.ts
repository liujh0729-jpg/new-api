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
import type {
  PromptInputFile,
  PromptInputSubmittedFile,
} from '@/components/ai-elements/prompt-input'

type RoleFile = Pick<PromptInputFile, 'url' | 'mediaType' | 'filename' | 'role'>

export interface LTXStartEndAttachmentState<T extends RoleFile> {
  firstFrame?: T
  extras: T[]
  isValid: boolean
}

export function getLTXStartEndAttachmentState<T extends RoleFile>(
  files: T[]
): LTXStartEndAttachmentState<T> {
  const images = files.filter((file) => getAttachmentKind(file) === 'image')
  const explicitFirst = images.find((file) => file.role === 'first_frame')
  const firstFrame = explicitFirst || images.at(0)
  const extras = files.filter((file) => file !== firstFrame)

  return {
    firstFrame,
    extras,
    isValid: !!firstFrame && extras.length === 0,
  }
}

export function assignLTXStartEndAttachmentRoles(
  files: PromptInputSubmittedFile[]
): PromptInputSubmittedFile[] {
  const state = getLTXStartEndAttachmentState(files)
  return files.map((file) => {
    if (file === state.firstFrame) return { ...file, role: 'first_frame' }
    return file
  })
}

function getAttachmentKind(file: RoleFile): 'image' | 'video' | 'audio' | '' {
  const mediaType = file.mediaType?.toLowerCase() || ''
  if (mediaType.startsWith('image/')) return 'image'
  if (mediaType.startsWith('video/')) return 'video'
  if (mediaType.startsWith('audio/')) return 'audio'
  const dataUrlType = file.url.match(/^data:(image|video|audio)\//i)?.[1]
  if (dataUrlType) {
    return dataUrlType.toLowerCase() as 'image' | 'video' | 'audio'
  }

  const urlPath = file.url.split(/[?#]/, 1)[0]
  if (/\.(avif|gif|jpe?g|png|webp)$/i.test(urlPath)) return 'image'
  if (/\.(m4v|mkv|mov|mp4|webm)$/i.test(urlPath)) return 'video'
  if (/\.(aac|flac|m4a|mp3|ogg|wav)$/i.test(urlPath)) return 'audio'
  return ''
}
