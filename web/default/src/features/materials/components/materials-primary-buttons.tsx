import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useRef } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { uploadMaterial } from '../api'
import { ACCEPTED_FILE_TYPES, SUCCESS_MESSAGES, ERROR_MESSAGES } from '../constants'
import { useMaterials } from './materials-provider'

export function MaterialsPrimaryButtons() {
  const { t } = useTranslation()
  const { triggerRefresh } = useMaterials()
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

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
      <Button size='sm' onClick={() => fileInputRef.current?.click()}>
        <Plus className='h-4 w-4' />
        {t('Upload Material')}
      </Button>
    </div>
  )
}
