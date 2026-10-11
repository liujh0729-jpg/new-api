import dayjs from '@/lib/dayjs'
import { MATERIAL_TIME_FILTER } from './constants'
import type { Material } from './types'

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const size = bytes / Math.pow(1024, i)
  return `${size.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function getMaterialTypeIcon(type: string): string {
  switch (type) {
    case 'image':
      return 'Image'
    case 'video':
      return 'Video'
    case 'audio':
      return 'Audio'
    default:
      return 'File'
  }
}

function getPreviewUserId(): string {
  if (typeof window === 'undefined') return ''
  try {
    return window.localStorage.getItem('uid')?.trim() || ''
  } catch {
    return ''
  }
}

function getMaterialProxyUrl(id: number): string {
  const userId = getPreviewUserId()
  const query = userId ? `?user_id=${encodeURIComponent(userId)}` : ''
  return `/pg/material/file/${encodeURIComponent(id)}${query}`
}

export function getMaterialPreviewUrl(
  material: Pick<Material, 'id' | 'url' | 'storage_type'> | number
): string {
  if (typeof material === 'number') {
    return getMaterialProxyUrl(material)
  }
  if (material.storage_type === 'local' && material.url) {
    return material.url
  }
  return getMaterialProxyUrl(material.id)
}

export function getMaterialTimeRange(value?: string): {
  created_after?: number
  created_before?: number
} {
  if (!value) return {}

  const now = dayjs()
  const end = now.endOf('day').unix()

  switch (value) {
    case MATERIAL_TIME_FILTER.TODAY:
      return {
        created_after: now.startOf('day').unix(),
        created_before: end,
      }
    case MATERIAL_TIME_FILTER.LAST_7_DAYS:
      return {
        created_after: now.subtract(6, 'day').startOf('day').unix(),
        created_before: end,
      }
    case MATERIAL_TIME_FILTER.LAST_30_DAYS:
      return {
        created_after: now.subtract(29, 'day').startOf('day').unix(),
        created_before: end,
      }
    case MATERIAL_TIME_FILTER.LAST_90_DAYS:
      return {
        created_after: now.subtract(89, 'day').startOf('day').unix(),
        created_before: end,
      }
    default:
      return {}
  }
}
