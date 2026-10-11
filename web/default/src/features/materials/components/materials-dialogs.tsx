import { MaterialsDeleteDialog } from './materials-delete-dialog'
import { MaterialsMutateDrawer } from './materials-mutate-drawer'

export function MaterialsDialogs() {
  return (
    <>
      <MaterialsMutateDrawer />
      <MaterialsDeleteDialog />
    </>
  )
}
