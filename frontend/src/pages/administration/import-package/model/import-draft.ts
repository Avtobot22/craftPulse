import type { components } from '@/shared/api'

type Upload = components['schemas']['ImportUpload']
export type ReportFormat = NonNullable<Upload['report_format']>

export const schemaLabels = {
  tasks: 'Задачи',
  task_events: 'События задач',
  relations: 'Связи',
  merge_requests: 'Запросы на слияние',
  ci_runs: 'Запуски CI',
  findings: 'Находки',
  junit: 'Результаты тестов (JUnit)',
  coverage: 'Покрытие',
  documents: 'Документы',
} satisfies Record<Upload['schema_id'], string>

export const reportLabels = {
  envelope: 'Записи craftPulse (JSON)',
  sarif: 'SARIF',
  gitlab_security: 'GitLab Security',
  gitlab_code_quality: 'GitLab Code Quality',
  junit_xml: 'JUnit XML',
  lcov: 'LCOV',
  cobertura: 'Cobertura',
  jacoco: 'JaCoCo',
} satisfies Record<ReportFormat, string>

const formatsBySchema: Readonly<Record<string, readonly ReportFormat[]>> = {
  findings: ['envelope', 'sarif', 'gitlab_security', 'gitlab_code_quality'],
  junit: ['junit_xml'],
  coverage: ['lcov', 'cobertura', 'jacoco'],
}

export function reportFormats(schema: string) {
  return Object.hasOwn(formatsBySchema, schema)
    ? (formatsBySchema[schema] ?? [])
    : []
}

export function isSchema(value: string | null): value is Upload['schema_id'] {
  return value !== null && Object.hasOwn(schemaLabels, value)
}

export const maxFileBytes = 25 * 1024 * 1024
export const versionLimit = 16
export const instanceKeyLimit = 512

export function initialDraft(search: URLSearchParams) {
  const version = search.get('schema_version') ?? ''
  const schema = search.get('schema_id') ?? ''
  const schemaId: string = isSchema(schema) ? schema : ''
  return {
    project_id: search.get('project_id') ?? '',
    schema_id: schemaId,
    schema_version: version.length <= versionLimit ? version : '',
    source_provider: '',
    source_instance_key: '',
    report_format: '',
    commit_sha: '',
    snapshot: false,
  }
}

export type ImportDraft = ReturnType<typeof initialDraft>
export type ImportField = keyof ImportDraft | 'file'

const json = { extensions: ['.json'], type: 'application/json' }
const csv = { extensions: ['.csv'], type: 'text/csv' }
const xml = {
  extensions: ['.xml'],
  type: 'application/xml',
  aliases: ['text/xml'],
}
const reportFiles = {
  envelope: json,
  sarif: {
    extensions: ['.sarif', '.json'],
    type: 'application/sarif+json',
    aliases: ['application/json'],
  },
  gitlab_security: json,
  gitlab_code_quality: json,
  junit_xml: xml,
  lcov: { extensions: ['.lcov', '.info', '.txt'], type: 'text/plain' },
  cobertura: xml,
  jacoco: xml,
} satisfies Record<ReportFormat, FileType>

interface FileType {
  extensions: string[]
  type: string
  aliases?: string[]
}

function fileTypes(draft: ImportDraft): FileType[] {
  const reports = reportFormats(draft.schema_id)
  if (!reports.length)
    return draft.schema_id === 'tasks' || !draft.schema_id
      ? [json, csv]
      : [json]
  const selected = reports.find((format) => format === draft.report_format)
  return (selected ? [selected] : reports).map((format) => reportFiles[format])
}

export function fileAccept(draft: ImportDraft) {
  return [
    ...new Set(
      fileTypes(draft).flatMap(({ extensions, type, aliases = [] }) => [
        ...extensions,
        type,
        ...aliases,
      ]),
    ),
  ].join(',')
}

export function fileMedia(file: File, draft: ImportDraft) {
  const name = file.name.toLowerCase()
  const types = fileTypes(draft)
  const byExtension = types.find(({ extensions }) =>
    extensions.some((ext) => name.endsWith(ext)),
  )
  if (byExtension) return byExtension.type
  // Extensionless exports can still have an explicit media type.
  if (name.includes('.')) return undefined
  return types.find(
    ({ type, aliases }) => type === file.type || aliases?.includes(file.type),
  )?.type
}

export function isTaskSnapshot(file: File | undefined, draft: ImportDraft) {
  return (
    draft.schema_id === 'tasks' &&
    file !== undefined &&
    fileMedia(file, draft) === csv.type
  )
}
