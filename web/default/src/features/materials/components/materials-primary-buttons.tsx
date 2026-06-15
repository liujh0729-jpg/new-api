import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { uploadMaterial } from '../api'
import {
  ACCEPTED_FILE_TYPES,
  SUCCESS_MESSAGES,
  ERROR_MESSAGES,
  MAX_UPLOAD_SIZE_BYTES,
  MAX_UPLOAD_SIZE_MB,
} from '../constants'
import { useMaterials } from './materials-provider'

export function MaterialsPrimaryButtons() {
  const { t } = useTranslation()
  const { triggerRefresh } = useMaterials()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [isUploading, setIsUploading] = useState(false)

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    if (!isAcceptedMaterialFile(file)) {
      toast.error(t('Only image, video, and audio files are supported'))
      e.target.value = ''
      return
    }
    if (file.size > MAX_UPLOAD_SIZE_BYTES) {
      toast.error(
        t('File size exceeds {{size}} MB limit', {
          size: MAX_UPLOAD_SIZE_MB,
        })
      )
      e.target.value = ''
      return
    }

    setIsUploading(true)
    try {
      const result = await uploadMaterial(file)
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.MATERIAL_UPLOADED))
        triggerRefresh()
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UPLOAD_FAILED))
    } finally {
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
      setIsUploading(false)
    }
  }

  return (
    <div className='flex gap-2'>
      <input
        ref={fileInputRef}
        type='file'
        accept={ACCEPTED_FILE_TYPES}
        onChange={handleFileChange}
        className='hidden'
      />
      <Button
        size='sm'
        onClick={() => fileInputRef.current?.click()}
        disabled={isUploading}
      >
        <Plus className='h-4 w-4' />
        {isUploading ? t('Uploading...') : t('Upload Material')}
      </Button>
    </div>
  )
}

function isAcceptedMaterialFile(file: File): boolean {
  const mimeType = file.type.toLowerCase()
  if (
    mimeType.startsWith('image/') ||
    mimeType.startsWith('video/') ||
    mimeType.startsWith('audio/')
  ) {
    return true
  }

  return /\.(png|jpe?g|webp|gif|bmp|heic|heif|mp4|mov|m4v|webm|mkv|avi|mpe?g|mpg|3gp|mp3|wav|m4a|aac|ogg|oga|flac|opus)$/i.test(
    file.name
  )
}
