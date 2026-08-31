/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
    10|but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Download, FileSpreadsheet, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  downloadTaskPricingCSVTemplate,
  exportTaskPricingCSV,
  importTaskPricingCSV,
  previewTaskPricingCSV,
  type TaskPricingCSVPlan,
} from '../api'

type TaskPricingCsvImportProps = {
  onImported?: () => void
}

export function TaskPricingCsvImport(props: TaskPricingCsvImportProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [plan, setPlan] = useState<TaskPricingCSVPlan | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const templateMutation = useMutation({
    mutationFn: downloadTaskPricingCSVTemplate,
    onSuccess: () => {
      toast.success(t('Template downloaded'))
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to download template'))
    },
  })

  const exportMutation = useMutation({
    mutationFn: exportTaskPricingCSV,
    onSuccess: () => {
      toast.success(t('Current task pricing exported'))
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to export task pricing'))
    },
  })

  const previewMutation = useMutation({
    mutationFn: previewTaskPricingCSV,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to preview CSV import'))
        setPlan(null)
        return
      }
      setPlan(response.data)
      toast.success(t('CSV preview ready'))
    },
    onError: (error: Error) => {
      setPlan(null)
      toast.error(error.message || t('Failed to preview CSV import'))
    },
  })

  const importMutation = useMutation({
    mutationFn: importTaskPricingCSV,
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to import CSV'))
        return
      }
      toast.success(t('Task pricing CSV imported'))
      setConfirmOpen(false)
      setPlan(null)
      setSelectedFile(null)
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      props.onImported?.()
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to import CSV'))
    },
  })

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    setSelectedFile(file)
    setPlan(null)
    previewMutation.mutate(file)
  }

  const summary = plan?.summary

  return (
    <div className='space-y-4 rounded-lg border p-4'>
      <div className='space-y-1'>
        <h3 className='text-sm font-medium'>
          {t('Task pricing CSV import / export')}
        </h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Download a retail-price template or current config, then import a CSV to update per-second task pricing and fixed Seedance discount groups.'
          )}
        </p>
      </div>

      <div className='flex flex-wrap gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={templateMutation.isPending}
          onClick={() => templateMutation.mutate()}
        >
          <Download className='mr-2 h-4 w-4' />
          {t('Download empty template')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={exportMutation.isPending}
          onClick={() => exportMutation.mutate()}
        >
          <FileSpreadsheet className='mr-2 h-4 w-4' />
          {t('Export current config')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={previewMutation.isPending}
          onClick={() => fileInputRef.current?.click()}
        >
          <Upload className='mr-2 h-4 w-4' />
          {t('Choose CSV to preview')}
        </Button>
        <input
          ref={fileInputRef}
          type='file'
          accept='.csv,text/csv'
          className='hidden'
          onChange={handleFileChange}
        />
      </div>

      {selectedFile ? (
        <p className='text-muted-foreground text-sm'>
          {t('Selected file')}: {selectedFile.name}
        </p>
      ) : null}

      {summary ? (
        <div className='bg-muted/40 space-y-2 rounded-md p-3 text-sm'>
          <div>
            {t('Models')}: {summary.models.length} (
            {summary.models.join(', ') || t('None')})
          </div>
          <div>
            {t('Resolution tiers')}: {summary.resolution_tiers}
          </div>
          <div>
            {t('CSV rows')}: {summary.source_rows}
          </div>
          {summary.rmb_per_usd ? (
            <div>
              {t('RMB per USD')}: {summary.rmb_per_usd}
            </div>
          ) : null}
          {summary.exempt_resolutions?.length ? (
            <div>
              {t('Native-price tiers')}:{' '}
              {summary.exempt_resolutions.join(', ')}
            </div>
          ) : null}
          <div className='pt-1'>
            <Button
              type='button'
              size='sm'
              disabled={importMutation.isPending}
              onClick={() => setConfirmOpen(true)}
            >
              {t('Apply import')}
            </Button>
          </div>
        </div>
      ) : null}

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Apply task pricing CSV import?')}
        desc={t(
          'This will overwrite task pricing and billing mode for models in the CSV, and sync fixed Seedance group ratios.'
        )}
        confirmText={t('Import')}
        isLoading={importMutation.isPending}
        handleConfirm={() => {
          if (!selectedFile) return
          importMutation.mutate(selectedFile)
        }}
      />
    </div>
  )
}
