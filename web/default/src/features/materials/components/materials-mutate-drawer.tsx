import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Label } from '@/components/ui/label'
import { updateMaterial } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import { useMaterials } from './materials-provider'

export function MaterialsMutateDrawer() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh } = useMaterials()
  const [name, setName] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const isUpdate = open === 'update'

  useEffect(() => {
    if (isUpdate && currentRow) {
      setName(currentRow.name)
    } else {
      setName('')
    }
  }, [isUpdate, currentRow])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!currentRow) return

    setIsSubmitting(true)
    try {
      const result = await updateMaterial(currentRow.id, name)
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.MATERIAL_UPDATED))
        setOpen(null)
        triggerRefresh()
      }
    } catch {
      toast.error(t('Failed to update material'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Sheet
      open={isUpdate}
      onOpenChange={(v) => {
        if (!v) setOpen(null)
      }}
    >
      <SheetContent className='flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-[400px]'>
        <SheetHeader className='border-b px-4 py-3 text-start sm:px-6 sm:py-4'>
          <SheetTitle>{t('Rename Material')}</SheetTitle>
          <SheetDescription>
            {t('Update the display name for this material.')}
          </SheetDescription>
        </SheetHeader>
        <form
          id='material-rename-form'
          onSubmit={handleSubmit}
          className='flex-1 space-y-4 overflow-y-auto px-3 py-3 pb-4 sm:space-y-6 sm:px-4'
        >
          <div className='space-y-2'>
            <Label htmlFor='material-name'>{t('Name')}</Label>
            <Input
              id='material-name'
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('Enter a name')}
            />
          </div>
        </form>
        <SheetFooter className='grid grid-cols-2 gap-2 border-t px-4 py-3 sm:flex sm:px-6 sm:py-4'>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button type='submit' form='material-rename-form' disabled={isSubmitting || !name.trim()}>
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
