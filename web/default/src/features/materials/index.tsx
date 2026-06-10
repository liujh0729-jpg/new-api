import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { MaterialsDialogs } from './components/materials-dialogs'
import { MaterialsPreviewDialog } from './components/materials-preview-dialog'
import { MaterialsPrimaryButtons } from './components/materials-primary-buttons'
import { MaterialsProvider } from './components/materials-provider'
import { MaterialsTable } from './components/materials-table'

export function Materials() {
  const { t } = useTranslation()
  return (
    <MaterialsProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Material Library')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage your media materials for AI generation reference')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <MaterialsPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <MaterialsTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <MaterialsDialogs />
      <MaterialsPreviewDialog />
    </MaterialsProvider>
  )
}
