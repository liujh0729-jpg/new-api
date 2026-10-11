import React, { useState } from 'react'
import useDialogState from '@/hooks/use-dialog'
import { type Material, type MaterialsDialogType } from '../types'

type MaterialsContextType = {
  open: MaterialsDialogType | null
  setOpen: (str: MaterialsDialogType | null) => void
  currentRow: Material | null
  setCurrentRow: React.Dispatch<React.SetStateAction<Material | null>>
  refreshTrigger: number
  triggerRefresh: () => void
  previewMaterial: Material | null
  setPreviewMaterial: React.Dispatch<React.SetStateAction<Material | null>>
}

const MaterialsContext = React.createContext<MaterialsContextType | null>(null)

export function MaterialsProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<MaterialsDialogType>(null)
  const [currentRow, setCurrentRow] = useState<Material | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const [previewMaterial, setPreviewMaterial] = useState<Material | null>(null)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <MaterialsContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
        previewMaterial,
        setPreviewMaterial,
      }}
    >
      {children}
    </MaterialsContext>
  )
}

export const useMaterials = () => {
  const materialsContext = React.useContext(MaterialsContext)

  if (!materialsContext) {
    throw new Error(
      'useMaterials has to be used within <MaterialsProvider>'
    )
  }

  return materialsContext
}
