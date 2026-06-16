import dayjs from '@/lib/dayjs'
import { MATERIAL_TIME_FILTER } from './constants'

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

export function getMaterialPreviewUrl(id: number): string {
  return `/pg/material/file/${encodeURIComponent(id)}`
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
