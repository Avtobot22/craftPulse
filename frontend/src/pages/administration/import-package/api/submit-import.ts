import { InvalidResponse, readRecord, readString } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { readTechnicalId } from '@/shared/config'
import { fileMedia, isTaskSnapshot, reportFormats } from '../model/import-draft'
import type { ImportDraft } from '../model/import-draft'

export async function submitImport(
  api: ApiClient,
  draft: ImportDraft,
  file: File,
  signal: AbortSignal,
) {
  const media = fileMedia(file, draft)
  if (!media) throw new Error('Неподдерживаемый тип файла')

  const form = new FormData()
  form.set('project_id', draft.project_id)
  form.set('schema_id', draft.schema_id)
  form.set('schema_version', draft.schema_version.trim())
  form.set('source_provider', draft.source_provider)
  form.set('source_instance_key', draft.source_instance_key)
  form.set('snapshot', String(draft.snapshot || isTaskSnapshot(file, draft)))
  if (reportFormats(draft.schema_id).length) {
    form.set('report_format', draft.report_format)
    form.set('commit_sha', draft.commit_sha.trim().toLowerCase())
  }
  // Use the contract's MIME type even when the platform supplies a generic one.
  form.set(
    'file',
    file.type === media ? file : file.slice(0, file.size, media),
    file.name,
  )
  const response = await api.createImport(
    form,
    (value) => {
      const id = readTechnicalId(readString(readRecord(value).receipt_id))
      if (!id) throw new InvalidResponse()
      return id
    },
    signal,
  )
  return response.data
}
