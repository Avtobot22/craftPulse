import { ApiError } from '@/shared/api'
import type { FieldError } from '@/shared/api'
import {
  fileMedia,
  instanceKeyLimit,
  isSchema,
  maxFileBytes,
  reportFormats,
  versionLimit,
} from './import-draft'
import type { ImportDraft, ImportField } from './import-draft'

export const fieldLabels = {
  project_id: 'Проект',
  schema_id: 'Вид данных',
  schema_version: 'Версия схемы',
  source_provider: 'Провайдер источника',
  source_instance_key: 'Ключ экземпляра источника',
  report_format: 'Формат отчёта',
  commit_sha: 'Ревизия (commit SHA)',
  snapshot: 'Снимок',
  file: 'Файл',
} satisfies Record<ImportField, string>

export interface ImportIssue {
  field?: ImportField
  message: string
}

const sizeError = 'Пакет превышает лимит 25 MiB'
const typeError = 'Выберите файл поддержанного формата'
const shaPattern = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/iu

export function fileIssue(
  file: File,
  draft: ImportDraft,
): ImportIssue | undefined {
  if (file.size > maxFileBytes) return { field: 'file', message: sizeError }
  if (!fileMedia(file, draft)) return { field: 'file', message: typeError }
  if (file.size === 0)
    return { field: 'file', message: 'Выберите непустой файл' }
  return undefined
}

export function validateImport(
  draft: ImportDraft,
  file: File | undefined,
  projectAvailable: boolean,
) {
  const issues: ImportIssue[] = []
  if (!projectAvailable)
    issues.push({ field: 'project_id', message: 'Выберите проект' })
  if (!isSchema(draft.schema_id))
    issues.push({ field: 'schema_id', message: 'Выберите вид данных' })
  if (
    !draft.schema_version.trim() ||
    draft.schema_version.length > versionLimit
  )
    issues.push({
      field: 'schema_version',
      message: 'Укажите версию схемы длиной до 16 символов',
    })
  if (!draft.source_provider)
    issues.push({
      field: 'source_provider',
      message: 'Выберите провайдер источника',
    })
  if (
    !draft.source_instance_key.trim() ||
    draft.source_instance_key.length > instanceKeyLimit
  )
    issues.push({
      field: 'source_instance_key',
      message: 'Укажите экземпляр источника длиной до 512 символов',
    })
  const formats = reportFormats(draft.schema_id)
  if (formats.length) {
    if (!formats.some((format) => format === draft.report_format))
      issues.push({ field: 'report_format', message: 'Выберите формат отчёта' })
    if (!shaPattern.test(draft.commit_sha.trim()))
      issues.push({
        field: 'commit_sha',
        message: 'Укажите 40 или 64 шестнадцатеричных символа',
      })
  }
  const issue = file
    ? fileIssue(file, draft)
    : ({ field: 'file', message: 'Выберите файл' } as const)
  if (issue) issues.push(issue)
  return issues
}

const fieldMessages: Record<FieldError['code'], string> = {
  required: 'Заполните обязательное поле',
  invalid_format: 'Проверьте формат',
  out_of_range: 'Значение вне допустимого диапазона',
  too_long: 'Значение слишком длинное',
  unknown_field: 'Поле не поддерживается',
  not_allowed_value: 'Значение не поддерживается',
  duplicate: 'Значение уже используется',
  outside_scope: 'Значение вне доступной области',
  unknown_reference: 'Связанный объект недоступен',
  unknown: 'Проверьте значение',
}

function isField(value: string): value is ImportField {
  return Object.hasOwn(fieldLabels, value)
}

export function submissionIssues(
  error: unknown,
  draft: ImportDraft,
): ImportIssue[] {
  const status = error instanceof ApiError ? error.status : undefined
  switch (status) {
    case 413:
      return [{ field: 'file', message: sizeError }]
    case 415:
      return [
        { field: 'file', message: 'Сервер не поддерживает тип этого файла' },
      ]
    case 422: {
      const fields =
        error instanceof ApiError ? (error.problem?.errors ?? []) : []
      const issues = fields.map<ImportIssue>(({ field: pointer, code }) => {
        const field = pointer.startsWith('/')
          ? (pointer.split('/')[1] ?? '')
          : ''
        const hidden =
          (field === 'commit_sha' || field === 'report_format') &&
          !reportFormats(draft.schema_id).length
        const message = fieldMessages[code]
        if (isField(field))
          return hidden
            ? { message: fieldLabels[field] + ' · ' + message }
            : { field, message }
        return { message }
      })
      return issues.length
        ? issues
        : [{ message: 'Проверьте параметры и содержимое файла' }]
    }
    case 403:
    case 404:
      return [
        { field: 'project_id', message: 'Нет права импорта в этот проект' },
      ]
    case 503:
      return [
        { message: 'Сервис временно недоступен. Повторите отправку позже' },
      ]
    case 400:
      return [{ message: 'Сервер отклонил пакет. Проверьте параметры и файл' }]
    case undefined:
    default:
      return [{ message: 'Исход передачи пока не известен' }]
  }
}
