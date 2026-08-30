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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import {
  type PromptInputFile,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'
import { getMinimaxH3ModelSpec } from '../lib/minimax-h3'
import {
  getMinimaxH3PromptInputIssue,
  type MinimaxH3InputState,
  normalizeMinimaxH3InputFiles,
} from '../lib/minimax-h3-input'
import type { SeedanceReferenceRole } from '../types'

function copyWithRole(
  file: PromptInputFile,
  role: SeedanceReferenceRole
): {
  url: string
  mediaType?: string
  filename?: string
  sourceFile?: File
  role: SeedanceReferenceRole
} {
  return {
    url: file.url,
    mediaType: file.mediaType,
    filename: file.filename,
    sourceFile: file.sourceFile,
    role,
  }
}

export function MinimaxH3AttachmentPolicy(props: { model: string }) {
  const attachments = usePromptInputAttachments()
  const spec = getMinimaxH3ModelSpec(props.model)

  useEffect(() => {
    if (!spec) return

    const files = attachments.files
    const keptFiles = normalizeMinimaxH3InputFiles(props.model, files)

    const keptIds = new Set(keptFiles.map((file) => file.id))
    for (const file of files) {
      if (!keptIds.has(file.id)) attachments.remove(file.id)
    }

    if (spec.kind !== 'first-last-frame-to-video') return
    for (const normalizedFile of keptFiles) {
      const originalFile = files.find((file) => file.id === normalizedFile.id)
      if (!normalizedFile.role || originalFile?.role === normalizedFile.role) {
        continue
      }
      attachments.remove(normalizedFile.id)
      attachments.upsertRemote(
        normalizedFile.role,
        copyWithRole(normalizedFile, normalizedFile.role)
      )
    }
  }, [attachments, props.model, spec])

  return null
}

export function MinimaxH3ValidationMessage(props: MinimaxH3InputState) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()
  const issue = getMinimaxH3PromptInputIssue(props, attachments.files)
  if (!issue) return null

  return (
    <p aria-live='polite' className='text-destructive px-5 text-xs'>
      {t(issue.key, issue.options)}
    </p>
  )
}
