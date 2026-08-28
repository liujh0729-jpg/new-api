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
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { VirtualCharacterAssetType } from '../types'
import { MaterialMedia } from './material-media'

export type CharacterImagePreview = {
  name: string
  url: string
  assetType?: VirtualCharacterAssetType
}

export function CharacterImagePreviewDialog(props: {
  preview: CharacterImagePreview | null
  onClose: () => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={props.preview != null}
      onOpenChange={(open) => !open && props.onClose()}
    >
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{props.preview?.name ?? t('Preview')}</DialogTitle>
          <DialogDescription>{t('Preview')}</DialogDescription>
        </DialogHeader>
        {props.preview ? (
          <MaterialMedia
            url={props.preview.url}
            name={props.preview.name}
            assetType={props.preview.assetType ?? 'Image'}
            controls
            className='bg-muted max-h-[75vh] w-full rounded-md object-contain'
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
