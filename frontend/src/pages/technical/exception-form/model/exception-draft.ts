import {
  describeExceptionScope,
  formatExceptionDate,
} from '@/entities/exception'
import type {
  ExceptionDraft,
  ExceptionScope,
  ExceptionVersion,
  FieldError,
} from '@/shared/api'
import { contractLabel, technicalLabels } from '@/shared/config'

export const decisionTypes = [
  'false_positive',
  'accepted_risk',
  'not_applicable',
] as const
export const maxReasonLength = 4000

export interface ScopeOption {
  label: string
  value: ExceptionScope
}
export interface ExceptionFields {
  type: string
  reason: string
  scopeIndex: number
  start: string
  end: string
}

export const fieldNames = {
  decision_type: 'Тип решения',
  reason: 'Причина',
  scope: 'Область действия',
  effective_from: 'Начало действия',
  expires_at: 'Срок пересмотра',
} as const

type Field = keyof typeof fieldNames
export type FormIssues = Partial<Record<Field | 'general', string>>

const validationMessages: Record<FieldError['code'], string> = {
  required: 'Заполните поле.',
  invalid_format: 'Проверьте формат.',
  out_of_range: 'Значение вне допустимого диапазона.',
  too_long: 'Значение слишком длинное.',
  unknown_field: 'Неизвестное поле.',
  not_allowed_value: 'Недопустимое значение.',
  duplicate: 'Значение уже добавлено.',
  outside_scope: 'Область выходит за ваши права.',
  unknown_reference: 'Объект не найден.',
  unknown: 'Проверьте значение.',
}

export function responseIssues(
  fields: ReadonlyMap<string, readonly FieldError[]>,
): FormIssues {
  const issues: FormIssues = {}
  for (const [pointer, entries] of fields) {
    const field = pointer.split('/')[1]
    const target =
      field && Object.hasOwn(fieldNames, field) ? (field as Field) : 'general'
    const entry = entries[0]
    if (entry)
      issues[target] = [issues[target], validationMessages[entry.code]]
        .filter(Boolean)
        .join(' ')
  }
  return Object.keys(issues).length
    ? issues
    : { general: 'Проверьте введённые данные.' }
}

export function initialFields(version?: ExceptionVersion): ExceptionFields {
  const now = new Date()
  const localTime = new Date(now.getTime() - now.getTimezoneOffset() * 60_000)
  return {
    type: version?.decision_type ?? '',
    reason: version?.reason ?? '',
    scopeIndex: 0,
    start: localTime.toISOString().slice(0, 16),
    end: '',
  }
}

export function scopeOptions(
  anchor: ExceptionScope,
  previous?: ExceptionScope,
): ScopeOption[] {
  const {
    area_id,
    finding_id,
    component_id,
    rule_id,
    path_prefix,
    package_purl,
  } = anchor
  const empty = {
    area_id,
    finding_id: null,
    component_id: null,
    rule_id: null,
    path_prefix: null,
    package_purl: null,
  }
  const options: ScopeOption[] = []
  if (previous)
    options.push({ label: 'Область исходной версии', value: previous })
  if (finding_id)
    options.push({
      label: 'Находка',
      value: { ...empty, finding_id, component_id },
    })
  if (rule_id) {
    options.push({ label: 'Правило', value: { ...empty, rule_id } })
    if (path_prefix)
      options.push({
        label: 'Путь',
        value: { ...empty, rule_id, component_id, path_prefix },
      })
    if (component_id)
      options.push({
        label: 'Компонент',
        value: { ...empty, rule_id, component_id },
      })
    if (package_purl)
      options.push({
        label: 'Пакет',
        value: { ...empty, rule_id, component_id, package_purl },
      })
  }
  return options
}

export function validateDraft(
  fields: ExceptionFields,
  scope: ExceptionScope | undefined,
): { draft: ExceptionDraft } | { issues: FormIssues } {
  const issues: FormIssues = {}
  const type = decisionTypes.find((decision) => decision === fields.type)
  const reason = fields.reason.trim()
  const start = new Date(fields.start)
  const end = new Date(fields.end)
  if (!type) issues.decision_type = 'Выберите тип решения.'
  if (!reason) issues.reason = 'Укажите причину.'
  else if (Array.from(reason).length > maxReasonLength)
    issues.reason = 'Сократите причину до 4 000 символов.'
  if (!scope) issues.scope = 'Выберите область действия.'
  if (!Number.isFinite(start.getTime()))
    issues.effective_from = 'Укажите начало действия.'
  if (!Number.isFinite(end.getTime()))
    issues.expires_at = 'Укажите срок пересмотра.'
  else if (end <= start)
    issues.expires_at = 'Срок пересмотра должен быть позже начала действия.'
  if (Object.keys(issues).length || !type || !scope) return { issues }
  return {
    draft: {
      decision_type: type,
      reason,
      scope,
      effective_from: start.toISOString(),
      expires_at: end.toISOString(),
    },
  }
}

function previewValues(value: ExceptionDraft | ExceptionVersion) {
  return {
    decision_type: contractLabel(
      technicalLabels.exceptionDecisionType,
      value.decision_type,
    ),
    reason: value.reason,
    scope: describeExceptionScope(value.scope),
    effective_from: formatExceptionDate(value.effective_from),
    expires_at: formatExceptionDate(value.expires_at),
  }
}

export function previewDraft(
  draft: ExceptionDraft,
  previous?: ExceptionVersion,
) {
  const after = previewValues(draft)
  const before = previous && previewValues(previous)
  return Object.entries(fieldNames).flatMap(([key, label]) => {
    const field = key as Field
    return after[field] === before?.[field]
      ? []
      : [
          {
            label,
            before: before?.[field] ?? 'Не задано',
            after: after[field],
          },
        ]
  })
}
