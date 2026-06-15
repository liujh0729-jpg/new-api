export const MATERIAL_TYPE = {
  IMAGE: 'image',
  VIDEO: 'video',
  AUDIO: 'audio',
} as const

export const ACCEPTED_FILE_TYPES = 'image/*,video/*,audio/*'
export const MAX_UPLOAD_SIZE_MB = 300
export const MAX_UPLOAD_SIZE_BYTES = MAX_UPLOAD_SIZE_MB * 1024 * 1024

export const SUCCESS_MESSAGES = {
  MATERIAL_UPLOADED: 'Material uploaded successfully',
  MATERIAL_UPDATED: 'Material updated successfully',
  MATERIAL_DELETED: 'Material deleted successfully',
} as const

export const ERROR_MESSAGES = {
  UNEXPECTED: 'An unexpected error occurred',
  LOAD_FAILED: 'Failed to load materials',
  SEARCH_FAILED: 'Failed to search materials',
  UPLOAD_FAILED: 'Failed to upload material',
  UPDATE_FAILED: 'Failed to update material',
  DELETE_FAILED: 'Failed to delete material',
} as const
